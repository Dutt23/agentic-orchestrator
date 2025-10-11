#!/usr/bin/env bash
set -euo pipefail

# ========================================
# Linux OS & Kernel Optimization Script
# For Orchestrator Platform
# ========================================
# Run with: sudo ./scripts/setup-linux.sh

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

# Detect OS
if [ -f /etc/os-release ]; then
    . /etc/os-release
    OS=$ID
    log_info "Detected OS: $OS"
else
    log_error "Cannot detect OS"
    exit 1
fi

# ========================================
# 1. Network Stack Tuning
# ========================================

log_info "Configuring network stack optimizations..."

cat > /etc/sysctl.d/99-orchestrator.conf <<EOF
# ========================================
# Orchestrator Platform - Network Tuning
# ========================================

# Connection queue sizes
net.core.somaxconn = 4096
net.core.netdev_max_backlog = 32768
net.ipv4.tcp_max_syn_backlog = 8192

# TCP tuning
net.ipv4.tcp_fin_timeout = 30
net.ipv4.tcp_tw_reuse = 1
net.ipv4.ip_local_port_range = 20000 60999
net.ipv4.tcp_mtu_probing = 1

# TCP Fast Open (TFO)
net.ipv4.tcp_fastopen = 3

# TCP keepalive
net.ipv4.tcp_keepalive_time = 300
net.ipv4.tcp_keepalive_intvl = 30
net.ipv4.tcp_keepalive_probes = 3

# Buffer sizes for high-throughput
net.core.rmem_max = 134217728
net.core.wmem_max = 134217728
net.ipv4.tcp_rmem = 4096 87380 67108864
net.ipv4.tcp_wmem = 4096 65536 67108864

# Congestion control (BBR recommended for modern kernels)
net.ipv4.tcp_congestion_control = bbr
net.core.default_qdisc = fq

# Connection tracking (if needed)
net.netfilter.nf_conntrack_max = 1048576
net.netfilter.nf_conntrack_tcp_timeout_established = 7200
net.netfilter.nf_conntrack_tcp_timeout_time_wait = 30
EOF

# Apply sysctl settings
sysctl -p /etc/sysctl.d/99-orchestrator.conf

log_info "Network tuning applied"

# ========================================
# 2. File Descriptor Limits
# ========================================

log_info "Configuring file descriptor limits..."

cat >> /etc/security/limits.conf <<EOF

# Orchestrator Platform - File Descriptor Limits
* soft nofile 262144
* hard nofile 262144
* soft nproc 65536
* hard nproc 65536
EOF

# Also set in systemd (if available)
if [ -d /etc/systemd/system.conf.d ]; then
    mkdir -p /etc/systemd/system.conf.d
    cat > /etc/systemd/system.conf.d/limits.conf <<EOF
[Manager]
DefaultLimitNOFILE=262144
DefaultLimitNPROC=65536
EOF
    systemctl daemon-reload
    log_info "Systemd limits configured"
fi

log_info "File descriptor limits set"

# ========================================
# 3. CPU & Scheduler Tuning
# ========================================

log_info "Configuring CPU scheduling..."

cat >> /etc/sysctl.d/99-orchestrator.conf <<EOF

# CPU scheduling
kernel.sched_migration_cost_ns = 5000000
kernel.sched_autogroup_enabled = 0
EOF

sysctl -p /etc/sysctl.d/99-orchestrator.conf

log_info "CPU scheduling configured"

# ========================================
# 4. Verify GRO/GSO (should be on by default)
# ========================================

log_info "Checking GRO/GSO status..."

# Get first active network interface
IFACE=$(ip -o link show | awk -F': ' '{print $2}' | grep -v lo | head -1)

if [ -n "$IFACE" ]; then
    log_info "Checking interface: $IFACE"

    GRO_STATUS=$(ethtool -k "$IFACE" 2>/dev/null | grep "generic-receive-offload" | awk '{print $2}' || echo "unknown")
    GSO_STATUS=$(ethtool -k "$IFACE" 2>/dev/null | grep "generic-segmentation-offload" | awk '{print $2}' || echo "unknown")

    log_info "GRO: $GRO_STATUS"
    log_info "GSO: $GSO_STATUS"

    if [ "$GRO_STATUS" != "on" ] || [ "$GSO_STATUS" != "on" ]; then
        log_warn "Consider enabling GRO/GSO for better performance"
        log_warn "Run: ethtool -K $IFACE gro on gso on"
    fi
else
    log_warn "Could not detect network interface for GRO/GSO check"
fi

# ========================================
# 5. Huge Pages (Optional for Postgres/Kafka)
# ========================================

log_info "Configuring Huge Pages (optional)..."

cat >> /etc/sysctl.d/99-orchestrator.conf <<EOF

# Huge pages (optional, for Postgres/Kafka)
vm.nr_hugepages = 1024
vm.hugetlb_shm_group = 0
EOF

sysctl -p /etc/sysctl.d/99-orchestrator.conf

log_info "Huge pages configured"

# ========================================
# 6. Disk I/O Tuning (for Postgres/CAS)
# ========================================

log_info "Configuring disk I/O..."

cat >> /etc/sysctl.d/99-orchestrator.conf <<EOF

# Disk I/O
vm.dirty_ratio = 10
vm.dirty_background_ratio = 5
vm.swappiness = 10
EOF

sysctl -p /etc/sysctl.d/99-orchestrator.conf

log_info "Disk I/O tuning applied"

# ========================================
# 7. Create Systemd Service Files
# ========================================

log_info "Creating systemd service templates..."

mkdir -p /etc/systemd/system

# Orchestrator service
cat > /etc/systemd/system/orchestrator.service <<EOF
[Unit]
Description=Orchestrator Service
After=network.target postgresql.service

[Service]
Type=simple
User=orchestrator
Group=orchestrator
WorkingDirectory=/opt/orchestrator
ExecStart=/opt/orchestrator/bin/orchestrator

# Environment
Environment="GOMAXPROCS=8"
Environment="GOGC=100"
Environment="GOMEMLIMIT=2GiB"

# CPU pinning (cores 0-7 for control plane)
CPUAffinity=0 1 2 3 4 5 6 7

# Resource limits
LimitNOFILE=262144
LimitNPROC=65536

# Restart policy
Restart=on-failure
RestartSec=5s

[Install]
WantedBy=multi-user.target
EOF

# Runner service (separate CPU cores)
cat > /etc/systemd/system/runner.service <<EOF
[Unit]
Description=Runner Service
After=network.target orchestrator.service

[Service]
Type=simple
User=orchestrator
Group=orchestrator
WorkingDirectory=/opt/orchestrator
ExecStart=/opt/orchestrator/bin/runner

# Environment
Environment="GOMAXPROCS=8"
Environment="GOGC=100"
Environment="GOMEMLIMIT=4GiB"

# CPU pinning (cores 8-15 for runners)
CPUAffinity=8 9 10 11 12 13 14 15

# Resource limits
LimitNOFILE=262144
LimitNPROC=65536

# Restart policy
Restart=on-failure
RestartSec=5s

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload

log_info "Systemd service files created"

# ========================================
# 8. eBPF Setup (Optional)
# ========================================

log_info "Checking eBPF support..."

if command -v bpftool &> /dev/null; then
    log_info "bpftool found, eBPF is available"
elif [ -f /sys/kernel/btf/vmlinux ]; then
    log_info "BTF support detected, eBPF available"
    log_warn "Install bpftool for eBPF management: apt install linux-tools-generic (Ubuntu) or yum install bpftool (RHEL)"
else
    log_warn "eBPF/BTF not detected. Kernel 5.4+ recommended for advanced observability"
fi

# ========================================
# 9. Install Required Packages
# ========================================

log_info "Installing required system packages..."

case "$OS" in
    ubuntu|debian)
        apt-get update
        apt-get install -y ethtool sysstat iotop htop net-tools
        log_info "Packages installed (Debian/Ubuntu)"
        ;;
    centos|rhel|fedora)
        yum install -y ethtool sysstat iotop htop net-tools
        log_info "Packages installed (RHEL/CentOS)"
        ;;
    *)
        log_warn "Unknown OS, skipping package installation"
        ;;
esac

# ========================================
# 10. Summary & Verification
# ========================================

echo ""
echo "========================================="
echo "  OS Optimization Complete"
echo "========================================="
echo ""
echo "✓ Network stack tuned (sysctl)"
echo "✓ File descriptor limits increased"
echo "✓ CPU scheduling optimized"
echo "✓ Systemd service files created"
echo ""
echo "Next steps:"
echo "  1. Reboot to apply all changes: sudo reboot"
echo "  2. Verify limits: ulimit -n"
echo "  3. Check sysctl: sysctl net.core.somaxconn"
echo "  4. Start services: systemctl start orchestrator"
echo ""
echo "Performance monitoring:"
echo "  - CPU: htop, top"
echo "  - Network: ss -s, netstat -s"
echo "  - Disk: iostat -x 1"
echo "  - Connections: ss -tan | wc -l"
echo ""
log_warn "A reboot is recommended to ensure all settings take effect"
