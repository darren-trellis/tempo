package config

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"sync"

	"gopkg.in/yaml.v3"
)

var (
	fileHashMu    sync.Mutex
	knownFileHash string
)

func noteFileHash(data []byte) {
	fileHashMu.Lock()
	knownFileHash = hashBytes(data)
	fileHashMu.Unlock()
}

func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func parseConfig(data []byte) (*Config, error) {
	cfg := DefaultConfig()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	cfg.ensureDefaults()
	cfg.loadExternalProfiles()
	return cfg, nil
}

// ReadChangedConfigFile returns the config file bytes when contents have
// changed since the last Load, Save, or AcknowledgeConfigFile.
func ReadChangedConfigFile() (data []byte, changed bool, err error) {
	data, err = os.ReadFile(ConfigPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("reading config: %w", err)
	}

	h := hashBytes(data)
	fileHashMu.Lock()
	defer fileHashMu.Unlock()
	if h == knownFileHash {
		return nil, false, nil
	}
	return data, true, nil
}

// ParseConfigFile parses config YAML. It does not update the known file hash.
func ParseConfigFile(data []byte) (*Config, error) {
	return parseConfig(data)
}

// AcknowledgeConfigFile records the current file contents so Tempo's own
// writes and already-applied reloads are not treated as new changes.
func AcknowledgeConfigFile(data []byte) {
	noteFileHash(data)
}

// ConnectionSettingsEqual reports whether two profiles would open the same connection.
func ConnectionSettingsEqual(a, b ConnectionConfig) bool {
	a = a.ExpandEnv()
	b = b.ExpandEnv()
	if a.Address != b.Address ||
		a.Namespace != b.Namespace ||
		a.APIKey != b.APIKey ||
		a.CodecEndpoint != b.CodecEndpoint ||
		a.TLS.Cert != b.TLS.Cert ||
		a.TLS.Key != b.TLS.Key ||
		a.TLS.CA != b.TLS.CA ||
		a.TLS.ServerName != b.TLS.ServerName ||
		a.TLS.SkipVerify != b.TLS.SkipVerify {
		return false
	}
	if len(a.GRPCMeta) != len(b.GRPCMeta) {
		return false
	}
	for k, v := range a.GRPCMeta {
		if b.GRPCMeta[k] != v {
			return false
		}
	}
	return true
}
