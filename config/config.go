// Package config loads and validates runtime configuration from environment variables and secret files.
package config

import (
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	MasterKey      []byte
	AdminToken     string
	BindAddr       string
	AdminPort      string
	DataPort       string
	DBPath         string
	TrustedProxies []string // comma-separated list from TRUSTED_PROXIES env var
}

func Load() (*Config, error) {
	masterKeyFile := envOr("MASTER_KEY_FILE", "/etc/openclaw-proxy/master_key")
	adminTokenFile := envOr("ADMIN_TOKEN_FILE", "/etc/openclaw-proxy/admin_token")
	bindAddr := envOr("BIND_ADDR", "127.0.0.1")
	adminPort := envOr("ADMIN_PORT", "9100")
	dataPort := envOr("DATA_PORT", "9101")
	dbPath := envOr("DB_PATH", "/var/lib/openclaw-proxy/proxy.db")
	trustedProxiesRaw := os.Getenv("TRUSTED_PROXIES") // optional, comma-separated

	masterKeyHex, err := readFileStripped(masterKeyFile)
	if err != nil {
		return nil, fmt.Errorf("reading master key from %s: %w", masterKeyFile, err)
	}
	masterKey, err := hex.DecodeString(masterKeyHex)
	if err != nil {
		return nil, fmt.Errorf("decoding master key hex: %w", err)
	}
	if len(masterKey) != 32 {
		return nil, fmt.Errorf("master key must be 32 bytes (64 hex chars), got %d bytes", len(masterKey))
	}

	adminToken, err := readFileStripped(adminTokenFile)
	if err != nil {
		return nil, fmt.Errorf("reading admin token from %s: %w", adminTokenFile, err)
	}
	if adminToken == "" {
		return nil, fmt.Errorf("admin token is empty")
	}

	if err := validatePort(adminPort, "ADMIN_PORT"); err != nil {
		return nil, err
	}
	if err := validatePort(dataPort, "DATA_PORT"); err != nil {
		return nil, err
	}

	var trustedProxies []string
	if trustedProxiesRaw != "" {
		for _, p := range strings.Split(trustedProxiesRaw, ",") {
			if t := strings.TrimSpace(p); t != "" {
				trustedProxies = append(trustedProxies, t)
			}
		}
	}

	return &Config{
		MasterKey:      masterKey,
		AdminToken:     adminToken,
		BindAddr:       bindAddr,
		AdminPort:      adminPort,
		DataPort:       dataPort,
		DBPath:         dbPath,
		TrustedProxies: trustedProxies,
	}, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func validatePort(port, name string) error {
	n, err := strconv.Atoi(port)
	if err != nil {
		return fmt.Errorf("%s %q is not a valid integer: %w", name, port, err)
	}
	if n < 1 || n > 65535 {
		return fmt.Errorf("%s %d is out of range (1-65535)", name, n)
	}
	return nil
}

func readFileStripped(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}
