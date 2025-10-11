#!/usr/bin/env bash
set -euo pipefail

# ========================================
# macOS System Optimization Script
# For Orchestrator Platform (Development)
# ========================================
# Note: macOS has limited tuning vs Linux

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

echo "========================================="
echo "  macOS System Optimization"
echo "  (Development/Local Testing)"
echo "========================================="
echo ""

# ========================================
# 1. File Descriptor Limits
# ========================================

log_info "Increasing file descriptor limits..."

# Check current limits
CURRENT_SOFT=$(ulimit -n)
log_info "Current soft limit: $CURRENT_SOFT"

# Try to increase (may require admin privileges)
ulimit -n 10240 2>/dev/null && log_info "Increased to 10240" || log_warn "Could not increase limit (try running with sudo)"

# Make permanent (requires reboot)
if [ "$EUID" -eq 0 ]; then
    cat > /Library/LaunchDaemons/limit.maxfiles.plist <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
  <dict>
    <key>Label</key>
    <string>limit.maxfiles</string>
    <key>ProgramArguments</key>
    <array>
      <string>launchctl</string>
      <string>limit</string>
      <string>maxfiles</string>
      <string>65536</string>
      <string>200000</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>ServiceIPC</key>
    <false/>
  </dict>
</plist>
EOF
    launchctl load -w /Library/LaunchDaemons/limit.maxfiles.plist 2>/dev/null || true
    log_info "Permanent limit configured (takes effect after reboot)"
else
    log_warn "Run with sudo to make limits permanent"
fi

# ========================================
# 2. Network Tuning (Limited on macOS)
# ========================================

log_info "Checking network settings..."

# macOS network tuning is limited; mostly defaults are good
# Show current settings
log_info "TCP buffer sizes:"
sysctl net.inet.tcp.recvspace
sysctl net.inet.tcp.sendspace

log_warn "macOS has limited network tuning compared to Linux"
log_warn "For production workloads, use Linux servers"

# ========================================
# 3. Environment Variables
# ========================================

log_info "Setting up Go environment..."

cat > ~/.orchestrator_env <<EOF
# Orchestrator Platform - Development Environment
export GOMAXPROCS=4  # Adjust based on your Mac CPU cores
export GOGC=100
export GOMEMLIMIT=4GiB

# For development
export LOG_LEVEL=debug
export ENVIRONMENT=development
EOF

log_info "Environment file created: ~/.orchestrator_env"
log_info "Add to your shell: source ~/.orchestrator_env"

# ========================================
# 4. Homebrew Packages
# ========================================

if command -v brew &> /dev/null; then
    log_info "Homebrew detected, installing useful tools..."

    brew install postgresql@15 || log_warn "PostgreSQL already installed"
    brew install redis || log_warn "Redis already installed"

    log_info "Tools installed via Homebrew"
else
    log_warn "Homebrew not found. Install from https://brew.sh"
fi

# ========================================
# 5. Docker Desktop Settings
# ========================================

if command -v docker &> /dev/null; then
    log_info "Docker detected"
    log_info "Recommended Docker Desktop settings:"
    echo "  - CPUs: 4-8"
    echo "  - Memory: 8GB+"
    echo "  - Swap: 2GB"
    echo "  - Enable VirtioFS for better filesystem performance"
else
    log_warn "Docker not found. Consider installing for local testing"
fi

# ========================================
# Summary
# ========================================

echo ""
echo "========================================="
echo "  macOS Optimization Complete"
echo "========================================="
echo ""
echo "✓ File descriptor limits increased"
echo "✓ Environment configured"
echo "✓ Development tools checked"
echo ""
echo "Next steps:"
echo "  1. Restart terminal or run: source ~/.orchestrator_env"
echo "  2. Verify limits: ulimit -n"
echo "  3. Start services: ./start.sh start"
echo ""
echo "⚠️  Note: For production deployments, use Linux servers"
echo "   macOS is suitable for development/testing only"
echo ""
