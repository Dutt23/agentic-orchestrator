#!/usr/bin/env bash
set -euo pipefail

# ========================================
# Verify OS Optimizations Script
# Checks if all recommended settings are applied
# ========================================

GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

PASS=0
WARN=0
FAIL=0

check_pass() {
    echo -e "${GREEN}✓${NC} $1"
    ((PASS++))
}

check_warn() {
    echo -e "${YELLOW}⚠${NC} $1"
    ((WARN++))
}

check_fail() {
    echo -e "${RED}✗${NC} $1"
    ((FAIL++))
}

echo "========================================="
echo "  Verifying OS Optimizations"
echo "========================================="
echo ""

# ========================================
# 1. File Descriptor Limits
# ========================================

echo "1. File Descriptor Limits"
ULIMIT=$(ulimit -n)
if [ "$ULIMIT" -ge 10240 ]; then
    check_pass "ulimit -n: $ULIMIT (recommended: ≥10240)"
else
    check_fail "ulimit -n: $ULIMIT (current) < 10240 (recommended)"
fi
echo ""

# ========================================
# 2. Network Settings (Linux only)
# ========================================

if [[ "$OSTYPE" == "linux-gnu"* ]]; then
    echo "2. Network Settings"

    # somaxconn
    SOMAXCONN=$(sysctl -n net.core.somaxconn 2>/dev/null || echo "0")
    if [ "$SOMAXCONN" -ge 4096 ]; then
        check_pass "net.core.somaxconn: $SOMAXCONN"
    else
        check_warn "net.core.somaxconn: $SOMAXCONN (recommended: 4096)"
    fi

    # TCP backlog
    BACKLOG=$(sysctl -n net.ipv4.tcp_max_syn_backlog 2>/dev/null || echo "0")
    if [ "$BACKLOG" -ge 4096 ]; then
        check_pass "net.ipv4.tcp_max_syn_backlog: $BACKLOG"
    else
        check_warn "net.ipv4.tcp_max_syn_backlog: $BACKLOG (recommended: 8192)"
    fi

    # Port range
    PORT_RANGE=$(sysctl -n net.ipv4.ip_local_port_range 2>/dev/null || echo "unknown")
    check_pass "net.ipv4.ip_local_port_range: $PORT_RANGE"

    echo ""
fi

# ========================================
# 3. Go Environment
# ========================================

echo "3. Go Environment"
if [ -n "${GOMAXPROCS:-}" ]; then
    check_pass "GOMAXPROCS: $GOMAXPROCS"
else
    check_warn "GOMAXPROCS not set (will use default: $(nproc 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null || echo 'unknown'))"
fi

if [ -n "${GOMEMLIMIT:-}" ]; then
    check_pass "GOMEMLIMIT: $GOMEMLIMIT"
else
    check_warn "GOMEMLIMIT not set"
fi
echo ""

# ========================================
# 4. GRO/GSO (Linux only)
# ========================================

if [[ "$OSTYPE" == "linux-gnu"* ]] && command -v ethtool &> /dev/null; then
    echo "4. Network Offloading (GRO/GSO)"

    IFACE=$(ip -o link show | awk -F': ' '{print $2}' | grep -v lo | head -1)
    if [ -n "$IFACE" ]; then
        GRO=$(ethtool -k "$IFACE" 2>/dev/null | grep "generic-receive-offload" | awk '{print $2}' || echo "unknown")
        GSO=$(ethtool -k "$IFACE" 2>/dev/null | grep "generic-segmentation-offload" | awk '{print $2}' || echo "unknown")

        if [ "$GRO" = "on" ]; then
            check_pass "GRO (Generic Receive Offload): $GRO"
        else
            check_warn "GRO: $GRO (recommended: on)"
        fi

        if [ "$GSO" = "on" ]; then
            check_pass "GSO (Generic Segmentation Offload): $GSO"
        else
            check_warn "GSO: $GSO (recommended: on)"
        fi
    fi
    echo ""
fi

# ========================================
# 5. Process Limits
# ========================================

echo "5. Process Limits"
NPROC=$(ulimit -u)
if [ "$NPROC" != "unlimited" ] && [ "$NPROC" -lt 4096 ]; then
    check_warn "Max processes (ulimit -u): $NPROC (recommended: ≥4096 or unlimited)"
else
    check_pass "Max processes (ulimit -u): $NPROC"
fi
echo ""

# ========================================
# 6. Database Connectivity
# ========================================

echo "6. PostgreSQL Connectivity"
if command -v psql &> /dev/null; then
    PGHOST="${POSTGRES_HOST:-localhost}"
    PGPORT="${POSTGRES_PORT:-5432}"

    if nc -z "$PGHOST" "$PGPORT" 2>/dev/null; then
        check_pass "PostgreSQL reachable at $PGHOST:$PGPORT"
    else
        check_fail "PostgreSQL not reachable at $PGHOST:$PGPORT"
    fi
else
    check_warn "psql not installed (optional for verification)"
fi
echo ""

# ========================================
# 7. Disk I/O (Linux only)
# ========================================

if [[ "$OSTYPE" == "linux-gnu"* ]]; then
    echo "7. Disk I/O Settings"

    SWAPPINESS=$(sysctl -n vm.swappiness 2>/dev/null || echo "unknown")
    if [ "$SWAPPINESS" = "unknown" ]; then
        check_warn "vm.swappiness: unknown"
    elif [ "$SWAPPINESS" -le 10 ]; then
        check_pass "vm.swappiness: $SWAPPINESS (low swapping, good for databases)"
    else
        check_warn "vm.swappiness: $SWAPPINESS (recommended: ≤10 for databases)"
    fi
    echo ""
fi

# ========================================
# 8. eBPF Support (Linux only)
# ========================================

if [[ "$OSTYPE" == "linux-gnu"* ]]; then
    echo "8. eBPF Support"

    if [ -f /sys/kernel/btf/vmlinux ]; then
        check_pass "BTF (BPF Type Format) available"
    else
        check_warn "BTF not available (Kernel 5.4+ recommended for eBPF)"
    fi

    if command -v bpftool &> /dev/null; then
        check_pass "bpftool installed"
    else
        check_warn "bpftool not installed (optional for eBPF management)"
    fi
    echo ""
fi

# ========================================
# Summary
# ========================================

echo "========================================="
echo "  Verification Summary"
echo "========================================="
echo ""
echo -e "${GREEN}Passed:${NC}  $PASS"
echo -e "${YELLOW}Warnings:${NC} $WARN"
echo -e "${RED}Failed:${NC}  $FAIL"
echo ""

if [ $FAIL -gt 0 ]; then
    echo "❌ Some critical checks failed"
    echo "   Run setup script: sudo ./scripts/setup-linux.sh"
    exit 1
elif [ $WARN -gt 0 ]; then
    echo "⚠️  Some optimizations are missing (optional)"
    echo "   System should work but may not be optimal for production"
    exit 0
else
    echo "✅ All optimizations are in place!"
    exit 0
fi
