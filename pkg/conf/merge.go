package conf

import (
	"fmt"
	"os"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Merge will merge additional configuration into an existing configuration
func Merge(configPaths []string, cfg Config) error {
	for _, configPath := range configPaths {
		if f, err := os.Open(configPath); err == nil {
			err = cfg.MergeConfig(f)
			_ = f.Close()
			if err != nil {
				return fmt.Errorf("error merging %s config file: %w", configPath, err)
			}
		} else {
			log.Infof("no config exists at %s, ignoring...", configPath)
		}
	}

	return nil
}
