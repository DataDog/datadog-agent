package remote

import (
	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Config is a config
type Config struct {
	ID      string
	Version uint64
}

type configFiles []configFile

func (c configFiles) version() uint64 {
	version := uint64(0)
	for _, file := range c {
		if file.version > version {
			version = file.version
		}
	}
	return version
}

type configFile struct {
	pathMeta data.ConfigPath
	version  uint64
	raw      []byte
}

type configs struct {
	apmSampling *apmSamplingConfigs
}

func newConfigs() *configs {
	return &configs{
		apmSampling: newApmSamplingConfigs(),
	}
}

type update struct {
	apmSamplingUpdate *APMSamplingUpdate
}

func (c *configs) update(products []data.Product, files configFiles) update {
	productConfigIDFiles := make(map[data.Product]map[string]configFiles)
	for _, file := range files {
		product := data.Product(file.pathMeta.Product)
		if _, exist := productConfigIDFiles[product]; !exist {
			productConfigIDFiles[product] = make(map[string]configFiles)
		}
		productConfigIDFiles[product][file.pathMeta.ConfigID] = append(productConfigIDFiles[product][file.pathMeta.ConfigID], file)
	}
	var update update
	for _, product := range products {
		switch product {
		case data.ProductAPMSampling:
			apmSamplingUpdate, err := c.apmSampling.update(productConfigIDFiles[product])
			if err != nil {
				log.Errorf("could not refresh apm sampling configurations: %v", err)
				continue
			}
			update.apmSamplingUpdate = apmSamplingUpdate
		default:
			log.Warnf("received %d files for unknown product %v", len(productConfigIDFiles[product]), product)
		}
	}
	return update
}

func (c *configs) state() []*pbgo.Config {
	var configs []*pbgo.Config
	if c.apmSampling.config != nil {
		for _, config := range c.apmSampling.config.Configs {
			configs = append(configs, &pbgo.Config{
				Id:      config.ID,
				Version: config.Version,
			})
		}
	}
	return configs
}
