#!/usr/bin/env bash
set -euo pipefail

echo "🔧 Setting up code generation tools..."
echo ""

# Check if Rust is installed
if ! command -v cargo &> /dev/null; then
    echo "❌ Error: Rust/Cargo not found."
    echo "   Install from: https://rustup.rs/"
    exit 1
fi

echo "✓ Rust/Cargo found"

# Install quicktype (cross-language code generator)
echo ""
echo "📦 Installing quicktype (code generator)..."
if command -v npm &> /dev/null; then
    if npm install -g quicktype; then
        echo "✓ quicktype installed successfully"
    else
        echo "⚠️  quicktype installation failed, but continuing..."
    fi
else
    echo "⚠️  npm not found - cannot install quicktype"
    echo "   Install Node.js from: https://nodejs.org/"
    echo "   Then run: npm install -g quicktype"
fi

# Check if Go is installed
if command -v go &> /dev/null; then
    echo ""
    echo "📦 Installing go-jsonschema (Go type generator)..."
    if go install github.com/atombender/go-jsonschema@latest; then
        echo "✓ go-jsonschema installed successfully"
    else
        echo "⚠️  go-jsonschema installation failed, but continuing..."
    fi
else
    echo ""
    echo "⚠️  Go not found - skipping go-jsonschema installation"
    echo "   Install Go from: https://go.dev/dl/"
fi

# Optional: Install ajv for schema validation
echo ""
echo "📦 Optional: Installing ajv-cli (JSON Schema validator)..."
if command -v npm &> /dev/null; then
    if npm install -g ajv-cli; then
        echo "✓ ajv-cli installed successfully"
    else
        echo "⚠️  ajv-cli installation failed, but continuing..."
    fi
else
    echo "⚠️  npm not found - skipping ajv-cli installation"
    echo "   Install Node.js from: https://nodejs.org/"
fi

# Optional: Install fswatch for watch mode (macOS)
echo ""
if [[ "$OSTYPE" == "darwin"* ]]; then
    echo "📦 Optional: Installing fswatch (file watcher for macOS)..."
    if command -v brew &> /dev/null; then
        if brew install fswatch; then
            echo "✓ fswatch installed successfully"
        else
            echo "⚠️  fswatch installation failed, but continuing..."
        fi
    else
        echo "⚠️  Homebrew not found - skipping fswatch installation"
        echo "   Install from: https://brew.sh/"
    fi
fi

echo ""
echo "✅ Setup complete!"
echo ""
echo "Next steps:"
echo "  1. Run 'make generate-types' to generate types from schemas"
echo "  2. Run 'make watch-schema' to auto-regenerate on changes"
echo "  3. See SCHEMA_SETUP.md for detailed documentation"
echo ""
