# Building aob CLI

## Quick Build

```bash
cd cmd/aob-cli
./build.sh
```

## Manual Build

### Prerequisites

Install Rust:
```bash
curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh
```

### Build Commands

```bash
# Development build (faster, larger binary)
cargo build

# Release build (optimized, smaller binary)
cargo build --release

# Clean build
cargo clean && cargo build --release
```

### Build Script Options

```bash
# Normal build
./build.sh

# Clean and build
./build.sh clean
```

## Binary Location

After build:
```
target/release/aob
```

## Test Build

```bash
# Check version
./target/release/aob --version

# Show help
./target/release/aob --help

# Test approve command
./target/release/aob approve --help
```

## Build Optimizations

The release build uses:
- LTO (Link-Time Optimization)
- Stripped symbols
- Optimized codegen
- Panic=abort (smaller binary)

See `Cargo.toml` for complete optimization settings.

## Usage (Without Installing)

```bash
# Run directly from target
./target/release/aob approve run_123 node_456
./target/release/aob logs stream run_123
```

## Installing to System (Optional)

```bash
# Copy to /usr/local/bin (requires sudo)
sudo cp target/release/aob /usr/local/bin/

# Or to user bin
cp target/release/aob ~/.local/bin/

# Or add alias
alias aob='./target/release/aob'
```

## Build Times

- **First build:** 2-5 minutes (downloads dependencies)
- **Incremental:** 10-30 seconds
- **Release build:** 30-60 seconds

## Troubleshooting

### Rust not found
```bash
curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh
source $HOME/.cargo/env
```

### OpenSSL errors
```bash
# On Ubuntu/Debian
sudo apt-get install pkg-config libssl-dev

# On macOS
brew install openssl
```

### Build errors
```bash
# Update Rust
rustup update

# Clean and rebuild
cargo clean
cargo build --release
```

---

**Build once, run anywhere!**
