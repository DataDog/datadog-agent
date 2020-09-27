package features

import (
	"encoding/json"
	config "github.com/StackVista/stackstate-agent/pkg/features/config"
	"github.com/StackVista/stackstate-agent/pkg/httpclient"
	log "github.com/cihub/seelog"
	"reflect"
	"sync"
)

// Features represents features supported by the StackState backend
type Features struct {
	config      *config.FeaturesConfig
	client      *httpclient.RetryableHTTPClient
	features    map[string]bool
	retriesLeft int
	mux         sync.Mutex
}

// NewFeatures returns a Features type given the config
func NewFeatures() *Features {
	conf := config.MakeFeaturesConfig()
	return &Features{
		config:      conf,
		client:      httpclient.NewStackStateClient(),
		features:    make(map[string]bool),
		retriesLeft: conf.MaxRetries,
	}
}

// Start begins the request cycle that fetched and populates the features supported by the StackState API
func (f *Features) Start() {
	f.getSupportedFeatures()
}

// Stop stops the request ticker
func (f *Features) Stop() {}

// getSupportedFeatures returns the features supported by the StackState API
func (f *Features) getSupportedFeatures() {
	if response := f.client.GetWithRetry("features", f.config.FeatureRequestTickerDuration, f.config.MaxRetries); response.Err == nil {
		var data interface{}
		// Parse json
		err := json.Unmarshal(response.Body, &data)
		if err != nil {
			_ = log.Errorf("error unmarshalling features json: %s of body %s", err, response.Body)
			return
		}

		// Validate structure
		featureMap, ok := data.(map[string]interface{})
		if !ok {
			_ = log.Errorf("Json was wrongly formatted, expected map type, got: %s", reflect.TypeOf(data))
			return
		}

		featuresParsed := make(map[string]bool)

		for k, v := range featureMap {
			featureValue, okV := v.(bool)
			if !okV {
				_ = log.Warnf("Json was wrongly formatted, expected boolean type, got: %s, skipping feature %s", reflect.TypeOf(v), k)
			}
			featuresParsed[k] = featureValue
		}

		f.mux.Lock()
		// Lock so only one goroutine at a time can access the map
		defer f.mux.Unlock()
		log.Infof("Server supports features: %v", featuresParsed)
		f.features = featuresParsed
		f.retriesLeft = response.RetriesLeft
	} else {
		log.Errorf("Server does not support features: %s", response.Err)
	}
}

// GetRetriesLeft returns the retry count
func (f *Features) GetRetriesLeft() int {
	f.mux.Lock()
	// Lock so only one goroutine at a time can access the map
	defer f.mux.Unlock()
	return f.retriesLeft
}

// FeatureEnabled checks whether a certain feature is enabled
func (f *Features) FeatureEnabled(feature string) bool {
	f.mux.Lock()
	// Lock so only one goroutine at a time can access the map
	defer f.mux.Unlock()
	if supported, ok := f.features[feature]; ok {
		return supported
	}
	return false
}
