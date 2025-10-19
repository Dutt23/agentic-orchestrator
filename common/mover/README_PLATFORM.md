# Mover Service - Platform Requirements

## Linux Only

**Mover service requires Linux kernel 5.1+** (for io_uring support)

### Supported Platforms

✅ **Linux** (Ubuntu 20.04+, Debian 11+, RHEL 8+, Amazon Linux 2023+)
- Native io_uring support
- Full performance (microsecond latency, zero-copy)

❌ **macOS** (Cannot build)
- io_uring is Linux-specific
- No macOS equivalent
- BSD kqueue is different API

❌ **Windows** (Cannot build)
- io_uring is Linux-only
- IOCP is different API

---

## Development on macOS

**Option 1: Test Without Mover**
```bash
# On macOS - test baseline (no mover)
USE_MOVER=false ./scripts/start-all.sh
```

**Option 2: Use Docker (Linux VM)**
```bash
# Docker on macOS runs Linux VM
# Build mover inside Docker
docker-compose up --build
```

**Option 3: Deploy to Linux Server**
```bash
# SSH to Linux server
ssh user@linux-server
# Build and test there
```

---

## Docker Network Mode for Performance

**Problem:** Docker bridge network adds ~1-5ms latency
- Masks mover's microsecond-level improvements
- Can't accurately measure io_uring benefits

**Solution: Host Network Mode**

```bash
# Use host networking (Linux only)
docker-compose -f docker-compose.yml -f docker-compose.host-network.yml up
```

**Benefits:**
- No bridge overhead
- Direct host network access
- True performance measurements

**Limitations:**
- ⚠️ Linux only (macOS Docker uses VM, doesn't support host mode)
- ⚠️ Services bind to host ports directly
- ⚠️ Less isolation

---

## Building Mover

### On Linux
```bash
cd common/mover
cargo build --release
./target/release/mover
```

### Cross-Compile from macOS
```bash
# Install cross-compilation tools
brew install messense/macos-cross-toolchains/x86_64-unknown-linux-gnu

# Build Linux binary on macOS
cargo build --release --target x86_64-unknown-linux-gnu
```

### In Docker
```bash
# Let Docker build it (Linux container)
docker-compose build mover
```

---

## Testing Strategy

**Development (macOS):**
1. Test baseline without mover
2. Build in Docker
3. Compare Docker bridge vs no-mover

**Production (Linux):**
1. Build mover natively
2. Use host networking
3. Measure true io_uring benefits

---

## See Also

- [docker-compose.host-network.yml](../../docker/docker-compose.host-network.yml) - Host networking config
- [Cargo.toml](./Cargo.toml) - Rust dependencies
- [config.rs](./src/config.rs) - Configuration options
