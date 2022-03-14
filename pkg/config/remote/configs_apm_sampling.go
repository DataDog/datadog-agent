package remote

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/trace/pb"
)

// APMSamplingConfig is an apm sampling config
type APMSamplingConfig struct {
	Configs map[string]Config
	Rates   []pb.APMSampling
}

// APMSamplingUpdate is an apm sampling config update
type APMSamplingUpdate struct {
	Config *APMSamplingConfig
}

type apmSamplingConfigs struct {
	config *APMSamplingConfig
}

func newApmSamplingConfigs() *apmSamplingConfigs {
	return &apmSamplingConfigs{}
}

func (c *apmSamplingConfigs) update(configFiles map[string]configFiles) (*APMSamplingUpdate, error) {
	if !c.shouldUpdate(configFiles) {
		return nil, nil
	}
	update := &APMSamplingUpdate{
		Config: &APMSamplingConfig{
			Configs: make(map[string]Config, len(configFiles)),
		},
	}
	for configID, files := range configFiles {
		update.Config.Configs[configID] = Config{
			ID:      configID,
			Version: files.version(),
		}
		for _, file := range files {
			var mpconfig pb.APMSampling
			_, err := mpconfig.UnmarshalMsg(file.raw)
			if err != nil {
				return nil, fmt.Errorf("could not parse apm sampling config: %v", err)
			}
			update.Config.Rates = append(update.Config.Rates, mpconfig)
		}
		c.config = update.Config
	}
	return update, nil
}

func (c *apmSamplingConfigs) shouldUpdate(configFiles map[string]configFiles) bool {
	if c.config == nil || len(c.config.Configs) != len(configFiles) {
		return true
	}
	for configID, files := range configFiles {
		if config, ok := c.config.Configs[configID]; !ok || config.Version < files.version() {
			return true
		}
	}
	return false
}
