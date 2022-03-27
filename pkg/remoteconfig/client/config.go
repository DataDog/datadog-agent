package client

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
)

var (
	ErrNoConfigVersion = errors.New("config has no version in its meta")
)

type configMeta struct {
	Version *uint64  `json:"v"`
	Clients []string `json:"c"`
	Expire  int64    `json:"e"`
}

func parseConfigMeta(rawMeta []byte) (configMeta, error) {
	var meta configMeta
	err := json.Unmarshal(rawMeta, &meta)
	if err != nil {
		return configMeta{}, err
	}
	if meta.Version == nil {
		return configMeta{}, ErrNoConfigVersion
	}
	return meta, nil
}

// Config is a config
type Config struct {
	ID      string
	Version uint64

	expire  int64
	clients []string
	hash    [32]byte
}

func (c *Config) expired(time int64) bool {
	return time > c.expire
}

func (c *Config) targetsClient(clientID string) bool {
	if len(c.clients) == 0 {
		return true
	}
	for _, client := range c.clients {
		if clientID == client {
			return true
		}
	}
	return false
}

func (c *Config) equal(config Config) bool {
	return c.hash == config.hash
}

func parseConfig(path string, meta []byte, contents []byte) (Config, error) {
	configPath, err := parseConfigPath(path)
	if err != nil {
		return Config{}, fmt.Errorf("could not parse config path: %v", err)
	}
	configMeta, err := parseConfigMeta(meta)
	if err != nil {
		return Config{}, fmt.Errorf("could not parse config meta: %v", err)
	}
	return Config{
		ID:      configPath.ConfigID,
		Version: *configMeta.Version,
		expire:  configMeta.Expire,
		clients: configMeta.Clients,
		hash:    configHash(path, meta, contents),
	}, nil
}

func configHash(path string, meta []byte, contents []byte) [32]byte {
	b := bytes.Buffer{}
	b.WriteString(path)
	metaHash := sha256.Sum256(meta)
	b.Write(metaHash[:])
	contentsHash := sha256.Sum256(meta)
	b.Write(contentsHash[:])
	return sha256.Sum256(b.Bytes())
}

// configList is the struct managing the configuration lifecycle
type configList struct {
	apmConfigs []ConfigAPMSamling
}
