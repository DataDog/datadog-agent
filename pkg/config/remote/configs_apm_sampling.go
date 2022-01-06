package remote

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/trace/pb"
)

// APMSamplingConfig is an apm sampling config
type APMSamplingConfig struct {
	Config
	Rates []pb.APMSampling
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
	if len(configFiles) > 1 {
		return nil, fmt.Errorf("apm sampling expects one config max. %d received", len(configFiles))
	}
	for configID, files := range configFiles {
		if c.config != nil && c.config.ID == configID && c.config.Version >= files.version() {
			return nil, nil
		}
		update := &APMSamplingUpdate{
			Config: &APMSamplingConfig{
				Config: Config{
					ID:      configID,
					Version: files.version(),
				},
			},
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
		return update, nil
	}
	return nil, nil
}
