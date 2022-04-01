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

type errUnknwonProduct struct {
	product string
}

func (e *errUnknwonProduct) Error() string {
	return fmt.Sprintf("unknown product %s", e.product)
}

type configCustom struct {
	Version *uint64  `json:"v"`
	Clients []string `json:"c"`
	Expire  int64    `json:"e"`
}

func parseConfigCustom(rawCustom []byte) (configCustom, error) {
	var custom configCustom
	err := json.Unmarshal(rawCustom, &custom)
	if err != nil {
		return configCustom{}, err
	}
	if custom.Version == nil {
		return configCustom{}, ErrNoConfigVersion
	}
	return custom, nil
}

type configMeta struct {
	hash   [32]byte
	custom configCustom
	path   configPath
}

func (c *configMeta) expired(time int64) bool {
	return time > c.custom.Expire
}

func (c *configMeta) scopedToClient(clientID string) bool {
	if len(c.custom.Clients) == 0 {
		return true
	}
	for _, client := range c.custom.Clients {
		if clientID == client {
			return true
		}
	}
	return false
}

// func (c *configMeta) equal(configMeta configMeta) bool {
// 	return c.hash == configMeta.hash
// }

func configMetaHash(path string, custom []byte) [32]byte {
	b := bytes.Buffer{}
	b.WriteString(path)
	metaHash := sha256.Sum256(custom)
	b.Write(metaHash[:])
	return sha256.Sum256(b.Bytes())
}

func parseConfigMeta(path string, custom []byte) (configMeta, error) {
	configPath, err := parseConfigPath(path)
	if err != nil {
		return configMeta{}, fmt.Errorf("could not parse config path: %v", err)
	}
	configCustom, err := parseConfigCustom(custom)
	if err != nil {
		return configMeta{}, fmt.Errorf("could not parse config meta: %v", err)
	}
	return configMeta{
		custom: configCustom,
		path:   configPath,
		hash:   configMetaHash(path, custom),
	}, nil
}

type config struct {
	meta     configMeta
	contents []byte
}

// func (c *config) equal(config config) bool {
// 	return c.meta.equal(config.meta) && bytes.Equal(c.contents, config.contents)
// }

// configList is a config list
type configList struct {
	configs    map[string]struct{}
	apmConfigs []ConfigAPMSamling
}

func newConfigList() *configList {
	return &configList{
		configs: make(map[string]struct{}),
	}
}

func (cl *configList) addConfig(c config) error {
	if _, exist := cl.configs[c.meta.path.ConfigID]; exist {
		return fmt.Errorf("duplicated config id: %s", c.meta.path.ConfigID)
	}
	switch c.meta.path.Product {
	case ProductAPMSampling:
		pc, err := parseConfigAPMSampling(c)
		if err != nil {
			return err
		}
		cl.apmConfigs = append(cl.apmConfigs, pc)
	default:
		return &errUnknwonProduct{product: c.meta.path.Product}
	}
	cl.configs[c.meta.path.ConfigID] = struct{}{}
	return nil
}

func (cl *configList) getCurrentConfigs(clientID string, time int64) Configs {
	configs := Configs{
		ApmConfigs: make([]ConfigAPMSamling, len(cl.apmConfigs)),
	}
	for _, pc := range cl.apmConfigs {
		if !pc.c.meta.expired(time) && pc.c.meta.scopedToClient(clientID) {
			configs.ApmConfigs = append(configs.ApmConfigs, pc)
		}
	}
	return configs
}

type Configs struct {
	ApmConfigs []ConfigAPMSamling
}
