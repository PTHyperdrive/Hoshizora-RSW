#!/bin/bash
# Build p2pnode as shared library for Linux (.so)
# Usage: ./build-dll.sh

echo "Building p2pnode.so..."

export CGO_ENABLED=1

go build -buildmode=c-shared -o p2pnode.so .

if [ $? -eq 0 ]; then
    echo "[SUCCESS] Built p2pnode.so"
    ls -lh p2pnode.so p2pnode.h 2>/dev/null
else
    echo "[ERROR] Build failed"
    exit 1
fi
