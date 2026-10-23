#!/bin/sh
# Builds the website (docs/) and the desktop programs (dist/). Requires Go 1.24+.
set -e
export CGO_ENABLED=0
GOOS=js GOARCH=wasm go build -trimpath -ldflags="-s -w" -o docs/chordstudio.wasm .
cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" docs/
mkdir -p dist
GOOS=darwin  GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o dist/chordstudio-mac-apple .
GOOS=darwin  GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o dist/chordstudio-mac-intel .
GOOS=windows GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o dist/ChordDiagramStudio.exe .
GOOS=linux   GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o dist/chordstudio-linux-x64 .
GOOS=linux   GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o dist/chordstudio-linux-arm64 .
echo "Done: website in docs/, programs in dist/"
