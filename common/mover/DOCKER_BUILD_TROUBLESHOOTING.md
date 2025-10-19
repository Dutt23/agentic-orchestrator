# Docker Build Troubleshooting

## SSL Certificate Issues

If you see:
```
SSL certificate problem: unable to get local issuer certificate
```

### Solution 1: Use Sparse Index (Already Applied)
```dockerfile
ENV CARGO_REGISTRIES_CRATES_IO_PROTOCOL=sparse
```

### Solution 2: Pre-download Dependencies on Host
```bash
# On your Mac (downloads to ~/.cargo/registry)
cd common/mover
cargo fetch

# Then in Dockerfile, copy cached registry
COPY ~/.cargo/registry /usr/local/cargo/registry
```

### Solution 3: Use Vendor Mode
```bash
# Download all dependencies to vendor/
cd common/mover
cargo vendor

# In Dockerfile
COPY common/mover/vendor ./vendor
RUN cargo build --release --offline
```

### Solution 4: Use Different Base Image
```dockerfile
# Try different Rust image
FROM rust:1.80-bookworm AS builder  # Debian-based, not slim
```

### Solution 5: Disable SSL Verification (Unsafe!)
```dockerfile
ENV CARGO_HTTP_CHECK_REVOKE=false
ENV CARGO_HTTP_CAINFO=/etc/ssl/certs/ca-certificates.crt
```

## Network Issues

### Use Docker BuildKit with Network Mode
```bash
DOCKER_BUILDKIT=1 docker build --network=host ...
```

### Configure DNS
```dockerfile
RUN echo "nameserver 8.8.8.8" > /etc/resolv.conf
```

## Testing Mover Without Docker

**Best option on macOS:**
Skip Docker entirely for now:

1. Deploy code to Linux server via git
2. Build mover natively there
3. Test on real Linux

Or use GitHub Actions (Linux runners):
```yaml
- name: Build mover
  run: cargo build --release --manifest-path common/mover/Cargo.toml
```
