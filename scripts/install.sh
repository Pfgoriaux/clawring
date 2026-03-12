#!/usr/bin/env bash
set -euo pipefail

REPO="Pfgoriaux/clawring"
BIN_NAME="clawring"
SERVICE_NAME="openclaw-proxy"
SERVICE_USER="openclaw-proxy"
CONFIG_DIR="/etc/openclaw-proxy"
DATA_DIR="/var/lib/openclaw-proxy"

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
        echo "Error: no sha256sum or shasum found." >&2
        exit 1
    fi
}

download_binary() {
    local platform="$1" install_dir="$2"
    local download_url="https://github.com/$REPO/releases/latest/download"
    local tmp_dir
    tmp_dir="$(mktemp -d)"
    trap 'rm -rf "$tmp_dir"' RETURN

    echo "Downloading $BIN_NAME for $platform..."
    curl -fsSL "$download_url/clawring_${platform}" -o "$tmp_dir/$BIN_NAME"
    curl -fsSL "$download_url/checksums.txt" -o "$tmp_dir/checksums.txt"

    local expected actual
    expected="$(grep "clawring_${platform}$" "$tmp_dir/checksums.txt" | awk '{print $1}')"
    actual="$(sha256_hash "$tmp_dir/$BIN_NAME")"
    if [ "$expected" != "$actual" ]; then
        echo "Checksum mismatch! Expected $expected, got $actual" >&2
        exit 1
    fi

    mkdir -p "$install_dir"
    mv "$tmp_dir/$BIN_NAME" "$install_dir/$BIN_NAME"
    chmod 755 "$install_dir/$BIN_NAME"
    echo "Binary installed to $install_dir/$BIN_NAME"
}

install_user_only() {
    local install_dir="$HOME/.local/bin"
    local platform
    platform="$(detect_platform)"
    download_binary "$platform" "$install_dir"

    if [[ ":$PATH:" != *":$install_dir:"* ]]; then
        echo ""
        echo "Add to PATH: export PATH=\"$install_dir:\$PATH\""
    fi
}

install_systemd() {
    if [ "$(id -u)" -ne 0 ]; then
        echo "Systemd install requires root. Run with sudo:" >&2
        echo "  curl -fsSL https://raw.githubusercontent.com/$REPO/main/scripts/install.sh | sudo bash -s -- --systemd" >&2
        exit 1
    fi

    if [ "$(uname -s)" != "Linux" ]; then
        echo "Systemd install is only supported on Linux." >&2
        exit 1
    fi

    local platform
    platform="$(detect_platform)"
    download_binary "$platform" "/usr/local/bin"

    # Create service user
    if ! id "$SERVICE_USER" &>/dev/null; then
        useradd --system --shell /usr/sbin/nologin --no-create-home "$SERVICE_USER"
        echo "Created system user: $SERVICE_USER"
    fi

    # Create directories
    mkdir -p "$CONFIG_DIR" "$DATA_DIR"

    # Generate secrets if they don't exist
    if [ ! -f "$CONFIG_DIR/master_key" ]; then
        openssl rand -hex 32 > "$CONFIG_DIR/master_key"
        chmod 640 "$CONFIG_DIR/master_key"
        echo "Generated master key"
    else
        echo "Master key already exists, skipping"
    fi

    if [ ! -f "$CONFIG_DIR/admin_token" ]; then
        openssl rand -hex 32 > "$CONFIG_DIR/admin_token"
        chmod 640 "$CONFIG_DIR/admin_token"
        echo "Generated admin token"
    else
        echo "Admin token already exists, skipping"
    fi

    # Set ownership
    chown -R "$SERVICE_USER:$SERVICE_USER" "$DATA_DIR"
    chown root:"$SERVICE_USER" "$CONFIG_DIR"
    chown root:"$SERVICE_USER" "$CONFIG_DIR/master_key" "$CONFIG_DIR/admin_token"

    # Install systemd unit
    cat > "/etc/systemd/system/${SERVICE_NAME}.service" <<'UNIT'
[Unit]
Description=Clawring Credential Injection Proxy
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=openclaw-proxy
Group=openclaw-proxy
ExecStart=/usr/local/bin/clawring
Restart=on-failure
RestartSec=5

Environment=MASTER_KEY_FILE=/etc/openclaw-proxy/master_key
Environment=ADMIN_TOKEN_FILE=/etc/openclaw-proxy/admin_token
Environment=DB_PATH=/var/lib/openclaw-proxy/proxy.db

# Hardening
NoNewPrivileges=yes
ProtectSystem=strict
ProtectHome=yes
ReadWritePaths=/var/lib/openclaw-proxy
ReadOnlyPaths=/etc/openclaw-proxy
PrivateTmp=yes
PrivateDevices=yes
ProtectKernelTunables=yes
ProtectKernelModules=yes
ProtectControlGroups=yes
MemoryDenyWriteExecute=yes
RestrictRealtime=yes
RestrictSUIDSGID=yes
CapabilityBoundingSet=CAP_NET_BIND_SERVICE
SystemCallFilter=@system-service

[Install]
WantedBy=multi-user.target
UNIT

    systemctl daemon-reload
    systemctl enable "$SERVICE_NAME"
    systemctl start "$SERVICE_NAME"

    echo ""
    echo "========================================"
    echo "  Clawring installed and running"
    echo "========================================"
    echo ""
    echo "Admin token: $(cat "$CONFIG_DIR/admin_token")"
    echo ""
    echo "Save this token now — you need it to manage keys and agents."
    echo ""
    echo "Verify:  curl http://127.0.0.1:9100/admin/health"
    echo "Logs:    journalctl -u $SERVICE_NAME -f"
    echo "Config:  systemctl edit $SERVICE_NAME"
    echo ""
    echo "To bind to a specific IP (e.g. Tailscale):"
    echo "  sudo systemctl edit $SERVICE_NAME"
    echo "  # Add: Environment=BIND_ADDR=100.x.x.x"
    echo "  sudo systemctl restart $SERVICE_NAME"
}

main() {
    local mode="user"

    for arg in "$@"; do
        case "$arg" in
            --systemd) mode="systemd" ;;
            --help|-h)
                echo "Usage: install.sh [--systemd] [--help]"
                echo ""
                echo "  (default)    Download binary to ~/.local/bin/"
                echo "  --systemd    Full install: binary, systemd service, secrets, user (requires root)"
                exit 0
                ;;
            *) echo "Unknown option: $arg" >&2; exit 1 ;;
        esac
    done

    case "$mode" in
        user) install_user_only ;;
        systemd) install_systemd ;;
    esac
}

main "$@"
