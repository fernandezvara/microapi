#!/usr/bin/env bash
set -euo pipefail
mkdir -p bin
go build -o bin/micro-api ./cmd/micro-api
echo "Built bin/micro-api"
