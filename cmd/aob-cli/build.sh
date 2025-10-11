#!/usr/bin/env bash
set -euo pipefail

# Build script for aob CLI

echo "Building aob CLI (Rust)..."

# Check if cargo is installed
if ! command -v cargo &> /dev/null; then
    echo "Error: Cargo not found. Install Rust from https://rustup.rs/"
    exit 1
fi

# Build release binary
echo "Building release binary (optimized for size)..."
cargo build --release

# Get binary size
BINARY="target/release/aob"
if [ -f "$BINARY" ]; then
    SIZE=$(ls -lh "$BINARY" | awk '{print $5}')
    echo "✓ Build complete: $BINARY ($SIZE)"

    # Show help
    echo ""
    echo "To install globally:"
    echo "  sudo cp $BINARY /usr/local/bin/"
    echo ""
    echo "To test:"
    echo "  $BINARY --version"
    echo "  $BINARY help"
else
    echo "Error: Binary not found"
    exit 1
fi
