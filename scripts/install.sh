#!/usr/bin/env bash
set -euo pipefail

REPO="Pfgoriaux/clawring"

detect_platform() {
    local os arch
    os="$(uname -s | tr '[:upper:]' '[:lower:]')"
    arch="$(uname -m)"

    case "$arch" in
        x86_64|amd64) arch="amd64" ;;
        aarch64|arm64) arch="arm64" ;;
        *) echo "Unsupported architecture: $arch" >&2; exit 1 ;;
    esac

    case "$os" in
        linux|darwin) ;;
        *) echo "Unsupported OS: $os" >&2; exit 1 ;;
    esac

    echo "${os}_${arch}"
}

sha256_hash() {
    if command -v sha256sum &>/dev/null; then
        sha256sum "$1" | awk '{print $1}'
    elif command -v shasum &>/dev/null; then
        shasum -a 256 "$1" | awk '{print $1}'
    else
        echo "Error: no sha256sum or shasum found. Install coreutils and retry." >&2
        exit 1
    fi
}

main() {
    local install_dir="$HOME/.local/bin"

    for arg in "$@"; do
        case "$arg" in
            --help|-h)
                echo "Usage: install.sh [--help]"
                echo "Downloads the clawring binary to ~/.local/bin/"
                exit 0
                ;;
            *) echo "Unknown option: $arg" >&2; exit 1 ;;
        esac
    done

    local platform download_url tmp_dir
    platform="$(detect_platform)"
    download_url="https://github.com/$REPO/releases/latest/download"
    tmp_dir="$(mktemp -d)"
    trap 'rm -rf "$tmp_dir"' EXIT

    echo "Downloading clawring for $platform..."
    curl -fsSL "$download_url/clawring_${platform}" -o "$tmp_dir/clawring"
    curl -fsSL "$download_url/checksums.txt" -o "$tmp_dir/checksums.txt"

    local expected actual
    expected="$(grep "clawring_${platform}$" "$tmp_dir/checksums.txt" | awk '{print $1}')"
    actual="$(sha256_hash "$tmp_dir/clawring")"
    if [ "$expected" != "$actual" ]; then
        echo "Checksum mismatch! Expected $expected, got $actual" >&2
        exit 1
    fi

    mkdir -p "$install_dir"
    mv "$tmp_dir/clawring" "$install_dir/clawring"
    chmod 755 "$install_dir/clawring"

    echo "Installed to $install_dir/clawring"

    if [[ ":$PATH:" != *":$install_dir:"* ]]; then
        echo "Add to PATH: export PATH=\"$install_dir:\$PATH\""
    fi
}

main "$@"
