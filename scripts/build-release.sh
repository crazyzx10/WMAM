#!/usr/bin/env bash
set -euo pipefail

TARGET="${1:-current}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
GO_APP="$REPO_ROOT/go-app"
DIST="$REPO_ROOT/dist"
export GOCACHE="$GO_APP/.gocache"
export GOPATH="$GO_APP/.gopath"
export GOMODCACHE="$GO_APP/.gomodcache"

mkdir -p "$GOCACHE" "$GOPATH" "$GOMODCACHE"

build_frontend() {
  cd "$GO_APP/web"
  npm run build
}

copy_common_files() {
  local out_dir="$1"
  cp "$GO_APP/config.yaml.example" "$out_dir/config.yaml.example"
  cp "$REPO_ROOT/README.md" "$out_dir/README.md"
  cp "$REPO_ROOT/WMAM-Deployment-Guide.md" "$out_dir/WMAM-Deployment-Guide.md"
}

build_go_target() {
  local name="$1"
  local goos="$2"
  local goarch="$3"
  local binary_name="$4"
  local out_dir="$DIST/wmam-$name"

  mkdir -p "$out_dir"
  cd "$GO_APP"
  GOOS="$goos" GOARCH="$goarch" go build -trimpath -o "$out_dir/$binary_name" .
  copy_common_files "$out_dir"
  echo "Built $name -> $out_dir"
}

build_frontend
mkdir -p "$DIST"

case "$TARGET" in
  current)
    case "$(uname -s)" in
      Linux*) build_go_target "linux-amd64" "linux" "amd64" "wmam-server" ;;
      MINGW*|MSYS*|CYGWIN*) build_go_target "windows-amd64" "windows" "amd64" "wmam-server.exe" ;;
      *) build_go_target "linux-amd64" "linux" "amd64" "wmam-server" ;;
    esac
    ;;
  windows-amd64)
    build_go_target "windows-amd64" "windows" "amd64" "wmam-server.exe"
    ;;
  linux-amd64)
    build_go_target "linux-amd64" "linux" "amd64" "wmam-server"
    ;;
  all)
    build_go_target "windows-amd64" "windows" "amd64" "wmam-server.exe"
    build_go_target "linux-amd64" "linux" "amd64" "wmam-server"
    ;;
  *)
    echo "Usage: $0 [current|windows-amd64|linux-amd64|all]" >&2
    exit 2
    ;;
esac
