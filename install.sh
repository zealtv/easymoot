#!/bin/sh
# macOS and Linux: build Easymoot and link it into Go's executable directory
# (or the supplied one). Keep this script compatible with /bin/sh on both.
# The same shape as Easyhog's and EasyLoom's install.sh, so all three install alike.
set -eu

if [ "$#" -gt 1 ]; then
    echo "Usage: $0 [bin-directory]" >&2
    exit 2
fi

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
go_cmd=${GO:-go}
# Resolve the install directory from the user's own Go settings, before the
# build below points GOPATH at the repo-local cache.
install_bin=${1:-$("$go_cmd" env GOBIN)}
if [ -z "$install_bin" ]; then
    go_path=$("$go_cmd" env GOPATH)
    install_bin=${go_path%%:*}/bin
fi

mkdir -p "$install_bin"
install_bin=$(CDPATH= cd -- "$install_bin" && pwd)
binary=$repo_root/bin/easymoot
link=$install_bin/easymoot
if [ "$link" = "$binary" ]; then
    echo "Choose an install directory other than the repo's bin directory." >&2
    exit 1
fi
if [ -d "$link" ]; then
    echo "Cannot replace directory: $link" >&2
    exit 1
fi

cd "$repo_root"
version=$(cat VERSION 2>/dev/null || true)
# The same isolated caches as the Makefile: never reuse another worktree's
# module or build cache.
CGO_ENABLED=0 \
    GOPATH="$repo_root/.cache/go" \
    GOMODCACHE="$repo_root/.cache/go/pkg/mod" \
    GOCACHE="$repo_root/.cache/go-build" \
    "$go_cmd" build ${version:+-ldflags "-X main.version=v$version"} -o "$binary" ./cmd/easymoot
ln -sfn "$binary" "$link"
echo "Installed: $link -> $binary"
case ":$PATH:" in
    *":$install_bin:"*) ;;
    *) echo "Add $install_bin to PATH to run easymoot from any directory." ;;
esac
