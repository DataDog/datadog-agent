package config

import "time"

// FeaturesConfig contains the configuration to customize the behaviour of the Features functionality.
type FeaturesConfig struct {
	// HTTPRequestTimeoutDuration is the HTTP timeout for POST requests to the StackState backend
	HTTPRequestTimeoutDuration time.Duration
	// FeatureRequestTickerDuration is the duration in between each of these requests
	FeatureRequestTickerDuration time.Duration
	// MaxRetries is the maximum number of times we'll try to fetch features from StackState
	MaxRetries int
}

// DefaultFeaturesConfig creates a new instance of a FeaturesConfig using default values.
func DefaultFeaturesConfig() FeaturesConfig {
	return FeaturesConfig{
		HTTPRequestTimeoutDuration:   10 * time.Second,
		FeatureRequestTickerDuration: 60 * time.Second,
		MaxRetries:                   15,
	}
}
