package features

import (
	"github.com/StackVista/stackstate-agent/pkg/trace/config"
	"github.com/stretchr/testify/assert"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestFeaturesWithRetries(t *testing.T) {
	mux := sync.Mutex{}
	GlobalRetriesLeft := 0
	featuresTestServer := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			mux.Lock()
			// Lock so only one goroutine at a time can access the map
			defer mux.Unlock()
			if GlobalRetriesLeft <= 7 {
				switch req.URL.Path {
				case "/features":
					w.WriteHeader(http.StatusOK)
					w.Header().Set("Content-Type", "application/json")
					a := `{
							"some-test-feature": true
						}`
					_, err := w.Write([]byte(a))
					if err != nil {
						t.Fatal(err)
					}
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			} else {
				w.WriteHeader(http.StatusNotFound)
			}
		}),
	)

	conf := config.New()
	conf.Endpoints = []*config.Endpoint{
		{Host: featuresTestServer.URL},
	}
	conf.FeaturesConfig.FeatureRequestTickerDuration = 200 * time.Millisecond
	conf.FeaturesConfig.MaxRetries = 10

	featureChan := make(chan map[string]bool, 1)
	features := NewTestFeatures(conf, featureChan)

	// assert feature not supported before fetched
	assert.False(t, features.FeatureEnabled("some-test-feature"))

	done := make(chan bool)
	// assert feature supported after fetch completed
	timeout := time.After(2 * time.Second)
	assertFunc := func() {
		assert.True(t, features.GetRetriesLeft() <= 7, "assert we had at least 3 retries in the test scenario, only got: %d", features.GetRetriesLeft())
		assert.True(t, features.FeatureEnabled("some-test-feature"), "assert that the feature is enabled, so we got the response from the backend")
		// stop feature fetcher
		features.Stop()
		done <- true
	}

	go func() {
	assertLoop:
		for {
			select {
			case <-timeout:
				assertFunc()
				break assertLoop
			default:
				mux.Lock()
				GlobalRetriesLeft = features.GetRetriesLeft()
				mux.Unlock()

				// check on each loop if the condition is satisfied yet, otherwise continue until the timeout
				enabled := features.FeatureEnabled("some-test-feature")
				if enabled {
					assertFunc()
					break assertLoop
				}
			}
		}
	}()

	// start feature fetcher
	features.Start()

	<-done
}
