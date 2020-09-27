package config

import (
	"github.com/StackVista/stackstate-agent/pkg/config"
	"time"
)

// FeaturesConfig contains the configuration to customize the behaviour of the Features functionality.
type FeaturesConfig struct {
	// FeatureRequestTickerDuration is the duration in between each of these requests
	FeatureRequestTickerDuration time.Duration
	// MaxRetries is the maximum number of times we'll try to fetch features from StackState
	MaxRetries int
}

// MakeFeaturesConfig creates a new instance of a FeaturesConfig using default values and override it with the config values
func MakeFeaturesConfig() *FeaturesConfig {
	return readFeaturesConfigYaml()
}

// defaultFeaturesConfig creates a new instance of a FeaturesConfig using default values.
func defaultFeaturesConfig() *FeaturesConfig {
	return &FeaturesConfig{
		FeatureRequestTickerDuration: 60 * time.Second,
		MaxRetries:                   15,
	}
}

type features struct {
	RetryIntervalMillis int `mapstructure:"retry_interval_millis"`
	MaxRetries          int `mapstructure:"max_retries"`
}

func readFeaturesConfigYaml() *FeaturesConfig {
	w := features{}
	c := defaultFeaturesConfig()

	if err := config.Datadog.UnmarshalKey("features", &w); err == nil {
		if w.MaxRetries > 0 {
			c.MaxRetries = w.MaxRetries
		}
		if w.RetryIntervalMillis > 0 {
			c.FeatureRequestTickerDuration = time.Duration(w.RetryIntervalMillis) * time.Millisecond
		}
	}
	return c
}
