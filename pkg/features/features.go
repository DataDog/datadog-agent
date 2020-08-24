package features

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"github.com/StackVista/stackstate-agent/pkg/trace/config"
	"github.com/StackVista/stackstate-agent/pkg/trace/info"
	"github.com/StackVista/stackstate-agent/pkg/trace/watchdog"
	log "github.com/cihub/seelog"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"time"
)

type featureEndpoint struct {
	*config.Endpoint
	client *http.Client
}

// Features represents features supported by the StackState backend
type Features struct {
	config               *config.AgentConfig
	endpoint             *featureEndpoint
	featureRequestTicker *time.Ticker
	featureChan          chan map[string]bool
	retriesLeft          int
	features             map[string]bool
	mux                  sync.Mutex
}

// NewTestFeatures returns a Features type given the config
func NewTestFeatures(conf *config.AgentConfig, channel chan map[string]bool) *Features {
	return makeFeatures(conf, channel)
}

// NewFeatures returns a Features type given the config
func NewFeatures(conf *config.AgentConfig) *Features {
	return makeFeatures(conf, make(chan map[string]bool, 1))
}

// makeFeatures returns a Features type given the config
func makeFeatures(conf *config.AgentConfig, channel chan map[string]bool) *Features {
	endpoint := conf.Endpoints[0]
	client := newClient(conf, false)
	if endpoint.NoProxy {
		client = newClient(conf, true)
	}

	return &Features{
		config: conf,
		endpoint: &featureEndpoint{
			Endpoint: endpoint,
			client:   client,
		},
		retriesLeft:          conf.FeaturesConfig.MaxRetries,
		featureChan:          channel,
		featureRequestTicker: time.NewTicker(conf.FeaturesConfig.FeatureRequestTickerDuration),
	}
}

// Start begins the request cycle that fetched and populates the features supported by the StackState API
func (f *Features) Start() {
	go func() {
		defer watchdog.LogOnPanic()
		defer close(f.featureChan)
		for {
			select {
			case <-f.featureRequestTicker.C:
				f.getSupportedFeatures()
			case featuresMap := <-f.featureChan:
				f.mux.Lock()
				// Set the supported features
				f.features = featuresMap
				f.mux.Unlock()
				// Stop polling and close this channel
				f.featureRequestTicker.Stop()
			}
		}
	}()
}

// Stop stops the request ticker
func (f *Features) Stop() {
	f.featureRequestTicker.Stop()
}

// getSupportedFeatures returns the features supported by the StackState API
func (f *Features) getSupportedFeatures() {
	f.mux.Lock()
	// Lock so only one goroutine at a time can access the map
	f.retriesLeft = f.retriesLeft - 1
	if f.retriesLeft == 0 {
		f.featureChan <- map[string]bool{}
	}
	f.mux.Unlock()

	resp, accessErr := f.makeFeatureRequest()
	// Handle error response
	if accessErr != nil {
		// Soo we got a 404, meaning we were able to contact stackstate, but it had no features path. We can publish a result
		if resp != nil {
			log.Info("Found StackState version which does not support feature detection yet")
			return
		}
		// Log
		_ = log.Error(accessErr)
		return
	}

	defer resp.Body.Close()

	// Get byte array
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		_ = log.Errorf("could not decode response body from features: %s", err)
		return
	}
	var data interface{}
	// Parse json
	err = json.Unmarshal(body, &data)
	if err != nil {
		_ = log.Errorf("error unmarshalling features json: %s of body %s", err, body)
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

	log.Infof("Server supports features: %v", featuresParsed)
	f.featureChan <- featuresParsed
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

func (f *Features) makeFeatureRequest() (*http.Response, error) {
	url := fmt.Sprintf("%s/features", f.endpoint.Host)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("could not create request to %s/features: %s", url, err)
	}

	req.Header.Add("content-encoding", "identity")
	req.Header.Add("sts-api-key", f.endpoint.APIKey)
	req.Header.Add("sts-hostname", f.endpoint.Host)
	req.Header.Add("sts-agent-version", info.Version)

	resp, err := f.endpoint.client.Do(req)
	if err != nil {
		if isHTTPTimeout(err) {
			return nil, fmt.Errorf("timeout detected on %s, %s", url, err)
		}
		return nil, fmt.Errorf("error submitting payload to %s: %s", url, err)
	}

	if resp.StatusCode < 200 || resp.StatusCode > 300 {
		defer resp.Body.Close()
		_, _ = io.Copy(ioutil.Discard, resp.Body)
		return resp, fmt.Errorf("unexpected response from %s. Status: %s", url, resp.Status)
	}

	return resp, nil
}

// newClient returns a http.Client configured with the Agent options.
func newClient(conf *config.AgentConfig, ignoreProxy bool) *http.Client {
	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		TLSClientConfig:       &tls.Config{InsecureSkipVerify: conf.SkipSSLValidation},
	}
	if conf.ProxyURL != nil && !ignoreProxy {
		log.Infof("configuring proxy through: %s", conf.ProxyURL.String())
		transport.Proxy = http.ProxyURL(conf.ProxyURL)
	}
	return &http.Client{Timeout: conf.FeaturesConfig.HTTPRequestTimeoutDuration, Transport: transport}
}

// IsTimeout returns true if the error is due to reaching the timeout limit on the http.client
func isHTTPTimeout(err error) bool {
	if netErr, ok := err.(interface {
		Timeout() bool
	}); ok && netErr.Timeout() {
		return true
	} else if strings.Contains(err.Error(), "use of closed network connection") { //To deprecate when using GO > 1.5
		return true
	}
	return false
}
