#!/bin/sh
# Сборка готовых файлов игры в папку release/.
# Windows-версия собирается на любой ОС; Linux-версии нужны dev-пакеты X11/OpenGL.
set -e
cd "$(dirname "$0")"
mkdir -p release
echo "Тесты..."
go test ./...
echo "Windows (amd64)..."
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -H windowsgui" -o release/HexTactics.exe ./cmd/hextactics
echo "Linux (amd64)..."
GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o release/hextactics-linux ./cmd/hextactics
ls -la release
