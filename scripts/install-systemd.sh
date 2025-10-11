#!/usr/bin/env bash
set -euo pipefail

# ========================================
# Install Systemd Services and Slices
# ========================================

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Check if running as root
if [ "$EUID" -ne 0 ]; then
    log_error "Please run as root (sudo)"
    exit 1
fi

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SYSTEMD_DIR="${PROJECT_ROOT}/scripts/systemd"
TARGET_DIR="/etc/systemd/system"

# Detect CPU count
CPU_COUNT=$(nproc)
log_info "Detected $CPU_COUNT CPU cores"

if [ "$CPU_COUNT" -lt 16 ]; then
    log_warn "System has fewer than 16 cores. Adjusting CPU affinity..."
    log_warn "Control plane: cores 0-$((CPU_COUNT/2-1))"
    log_warn "Runner plane: cores $((CPU_COUNT/2))-$((CPU_COUNT-1))"
fi

# ========================================
# 1. Install Slice Files
# ========================================

log_info "Installing systemd slices..."

cp "${SYSTEMD_DIR}/orchestrator-control.slice" "${TARGET_DIR}/"
cp "${SYSTEMD_DIR}/orchestrator-runner.slice" "${TARGET_DIR}/"
cp "${SYSTEMD_DIR}/orchestrator-fanout.slice" "${TARGET_DIR}/"

# Adjust CPU affinity if fewer cores
if [ "$CPU_COUNT" -lt 16 ]; then
    HALF=$((CPU_COUNT/2))

    # Control plane: first half
    sed -i "s/CPUAffinity=0 1 2 3 4 5 6 7/CPUAffinity=$(seq -s ' ' 0 $((HALF-1)))/" \
        "${TARGET_DIR}/orchestrator-control.slice"

    # Runner plane: second half
    sed -i "s/CPUAffinity=8 9 10 11 12 13 14 15/CPUAffinity=$(seq -s ' ' $HALF $((CPU_COUNT-1)))/" \
        "${TARGET_DIR}/orchestrator-runner.slice"

    # Fanout: share with control plane
    sed -i "s/CPUAffinity=16 17 18 19/CPUAffinity=$(seq -s ' ' 0 3)/" \
        "${TARGET_DIR}/orchestrator-fanout.slice"
fi

log_info "Slices installed and configured"

# ========================================
# 2. Install Service Files
# ========================================

log_info "Installing service files..."

SERVICES=(
    "orchestrator-orchestrator.service"
    "orchestrator-runner.service"
    "orchestrator-fanout.service"
)

for service in "${SERVICES[@]}"; do
    if [ -f "${SYSTEMD_DIR}/${service}" ]; then
        cp "${SYSTEMD_DIR}/${service}" "${TARGET_DIR}/"
        log_info "Installed ${service}"
    else
        log_warn "${service} not found, skipping"
    fi
done

# ========================================
# 3. Create User and Directories
# ========================================

log_info "Setting up orchestrator user..."

if ! id -u orchestrator &>/dev/null; then
    useradd -r -s /bin/false -d /opt/orchestrator orchestrator
    log_info "Created orchestrator user"
else
    log_info "orchestrator user already exists"
fi

# Create directories
mkdir -p /opt/orchestrator/{bin,data,logs,etc}
chown -R orchestrator:orchestrator /opt/orchestrator

log_info "Directories created"

# ========================================
# 4. Reload Systemd
# ========================================

log_info "Reloading systemd..."
systemctl daemon-reload

# ========================================
# 5. Enable Services (don't start yet)
# ========================================

log_info "Enabling services..."

for service in "${SERVICES[@]}"; do
    systemctl enable "${service}" || log_warn "Could not enable ${service}"
done

log_info "Services enabled"

# ========================================
# 6. Display Status
# ========================================

echo ""
echo "========================================="
echo "  Systemd Installation Complete"
echo "========================================="
echo ""
echo "✓ Slices installed:"
echo "  - orchestrator-control.slice (CPU cores: 0-7)"
echo "  - orchestrator-runner.slice (CPU cores: 8-15)"
echo "  - orchestrator-fanout.slice (CPU cores: 16-19)"
echo ""
echo "✓ Services installed:"
for service in "${SERVICES[@]}"; do
    echo "  - ${service}"
done
echo ""
echo "Next steps:"
echo "  1. Copy binaries to /opt/orchestrator/bin/"
echo "  2. Create config files in /opt/orchestrator/etc/"
echo "  3. Start services: systemctl start orchestrator-orchestrator.service"
echo "  4. Check status: systemctl status orchestrator-orchestrator.service"
echo "  5. View logs: journalctl -u orchestrator-orchestrator -f"
echo ""
echo "Resource monitoring:"
echo "  systemctl show orchestrator-control.slice"
echo "  systemd-cgtop"
echo ""
