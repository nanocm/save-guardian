#!/usr/bin/env bash
# Build SaveGuardian.exe for Windows (amd64) from any platform.
set -euo pipefail
cd "$(dirname "$0")"

echo "== go test =="
go test ./...

echo "== build Windows amd64 =="
GOOS=windows GOARCH=amd64 go build -ldflags "-s -w" -o SaveGuardian.exe .

echo "Done: $(ls -la SaveGuardian.exe | awk '{print $5, $NF}')"
