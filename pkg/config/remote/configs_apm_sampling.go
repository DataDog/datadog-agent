package remote

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/trace/pb"
)

type APMSamplingConfig struct {
	Config
	pb.APMSampling
}

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
	var update *APMSamplingUpdate
	for configID, files := range configFiles {
		if len(files) != 1 {
			return nil, fmt.Errorf("apm sampling expects one file per config max. %d received", len(files))
		}
		file := files[0]
		if c.config != nil && c.config.ID == configID && c.config.Version >= files.version() {
			return nil, nil
		}
		var mpconfig pb.APMSampling
		_, err := mpconfig.UnmarshalMsg(file.raw)
		if err != nil {
			return nil, fmt.Errorf("could not parse apm sampling config: %v", err)
		}
		update = &APMSamplingUpdate{
			Config: &APMSamplingConfig{
				Config: Config{
					ID:      file.pathMeta.ConfigID,
					Version: file.version,
				},
				APMSampling: mpconfig,
			},
		}
	}
	return update, nil
}
