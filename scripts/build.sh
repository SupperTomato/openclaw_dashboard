#!/bin/bash
# OpenClaw Dashboard - Build Script
# Builds all binaries with minimal size optimization
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
BACKEND_DIR="$PROJECT_DIR/backend"
BIN_DIR="$BACKEND_DIR/bin"

echo "========================================"
echo " OpenClaw Dashboard Build"
echo "========================================"
echo ""

# Check Go installation
if ! command -v go &> /dev/null; then
    echo "ERROR: Go is not installed."
    echo "Install Go: https://go.dev/dl/"
    exit 1
fi

echo "Go version: $(go version)"
echo "Project: $PROJECT_DIR"
echo ""

# Create bin directory
mkdir -p "$BIN_DIR"

# Build dashboard
echo "[1/11] Building dashboard server..."
cd "$BACKEND_DIR"
go mod tidy
CGO_ENABLED=0 go build -ldflags="-s -w" -trimpath -o "$BIN_DIR/openclaw-dashboard" ./cmd/dashboard/
echo "  -> $(ls -lh "$BIN_DIR/openclaw-dashboard" | awk '{print $5}')"

# Build modules
MODULES=(
    "system-health"
    "session-manager"
    "live-feed"
    "log-viewer"
    "file-manager"
    "cost-analyzer"
    "rate-limiter"
    "memory-viewer"
    "service-control"
    "cron-manager"
)

for i in "${!MODULES[@]}"; do
    MODULE="${MODULES[$i]}"
    NUM=$((i + 2))
    echo "[$NUM/11] Building module: $MODULE..."
    CGO_ENABLED=0 go build -ldflags="-s -w" -trimpath -o "$BIN_DIR/module-$MODULE" "./modules/$MODULE/"
    echo "  -> $(ls -lh "$BIN_DIR/module-$MODULE" | awk '{print $5}')"
done

echo ""
echo "========================================"
echo " Build Complete!"
echo "========================================"
echo ""
echo "Binaries in: $BIN_DIR"
echo ""
echo "Binary sizes:"
ls -lh "$BIN_DIR/" | tail -n +2 | awk '{printf "  %-30s %s\n", $9, $5}'
echo ""
echo "Total size: $(du -sh "$BIN_DIR" | awk '{print $1}')"
echo ""
echo "To start the dashboard:"
echo "  cd $PROJECT_DIR"
echo "  ./backend/bin/openclaw-dashboard --config config/openclaw.config.json"
