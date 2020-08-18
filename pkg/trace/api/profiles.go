package api

import (
	"bytes"
	"fmt"
	"io/ioutil"
	stdlog "log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/trace/info"
	"github.com/DataDog/datadog-agent/pkg/trace/logutil"
	"github.com/DataDog/datadog-agent/pkg/trace/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// profilingURLTemplate specifies the template for obtaining the profiling URL along with the site.
	profilingURLTemplate = "https://intake.profile.%s/v1/input"
	// profilingURLDefault specifies the default intake API URL.
	profilingURLDefault = "https://intake.profile.datadoghq.com/v1/input"

	profilingMainEndpointConfigKey        = "apm_config.profiling_dd_url"
	profilingAdditionalEndpointsConfigKey = "apm_config.profiling_additional_endpoints"
)

// mainProfilingEndpoint returns the main profiling intake API URL based on agent
// configuration. When multiple endpoints are in use, the response from the main
// endpoint is proxied back to the client, while for all aditional endpoints the
// response is discarded.
func mainProfilingEndpoint() string {
	if v := config.Datadog.GetString(profilingMainEndpointConfigKey); v != "" {
		return v
	}
	if site := config.Datadog.GetString("site"); site != "" {
		return fmt.Sprintf(profilingURLTemplate, site)
	}
	return profilingURLDefault
}

// additionalProfilingEndpoints returns a map of endpoint URLs to a slice of api
// keys to be used for each endpoint. There is no de-duplication between api
// keys or between these additional endpoints and the main endpoint.
func additionalProfilingEndpoints() map[string][]string {
	if config.Datadog.IsSet(profilingAdditionalEndpointsConfigKey) {
		return config.Datadog.GetStringMapStringSlice(profilingAdditionalEndpointsConfigKey)
	}
	return make(map[string][]string)
}

// profileProxyHandler returns a new HTTP handler which will proxy requests to the profiling intakes.
// If the main intake URL can not be computed because of config, the returned handler will always
// return http.StatusInternalServerError along with a clarification.
func (r *HTTPReceiver) profileProxyHandler() http.Handler {
	tags := fmt.Sprintf("host:%s,default_env:%s", r.conf.Hostname, r.conf.DefaultEnv)
	e := mainProfilingEndpoint()
	u, err := url.Parse(e)
	if err != nil {
		log.Errorf("Error parsing main intake URL %s: %v", e, err)
		return errorHandler(e)
	}
	proxies := []*httputil.ReverseProxy{newProfileProxy(r.conf.NewHTTPTransport(), u, r.conf.APIKey(), tags)}

	for e, keys := range additionalProfilingEndpoints() {
		u, err := url.Parse(e)
		if err != nil {
			log.Errorf("Error parsing additional intake URL %s: %v", e, err)
			continue
		}

		for _, apiKey := range keys {
			proxies = append(proxies, newProfileProxy(r.conf.NewHTTPTransport(), u, apiKey, tags))
		}
	}

	switch len(proxies) {
	case 0:
		return errorHandler(e)
	case 1:
		return proxies[0]
	default:
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			bodyBytes, err := ioutil.ReadAll(req.Body)
			if err != nil {
				msg := "Failed to read request body"
				http.Error(w, msg, http.StatusInternalServerError)
				return
			}
			outreq := req.Clone(req.Context())
			outreq.Body = ioutil.NopCloser(bytes.NewReader(bodyBytes))
			// we use the original ResponseWriter with the main endpoint request
			// and forward the response to the client
			proxies[0].ServeHTTP(w, outreq)
			for _, proxy := range proxies[1:] {
				outreq := req.Clone(req.Context())
				outreq.Body = ioutil.NopCloser(bytes.NewReader(bodyBytes))
				// for all additional endpoints we ignore the response
				proxy.ServeHTTP(&nopResponseWriter{}, outreq)
			}
		})
	}
}

// newProfileProxy creates a single-host reverse proxy with the given target, attaching
// the specified apiKey.
func newProfileProxy(transport http.RoundTripper, target *url.URL, apiKey, tags string) *httputil.ReverseProxy {
	director := func(req *http.Request) {
		req.URL = target
		req.Host = target.Host
		req.Header.Set("DD-API-KEY", apiKey)
		req.Header.Set("Via", fmt.Sprintf("trace-agent %s", info.Version))
		if _, ok := req.Header["User-Agent"]; !ok {
			// explicitly disable User-Agent so it's not set to the default value
			// that net/http gives it: Go-http-client/1.1
			// See https://codereview.appspot.com/7532043
			req.Header.Set("User-Agent", "")
		}
		containerID := req.Header.Get(headerContainerID)
		if ctags := getContainerTags(containerID); ctags != "" {
			req.Header.Set("X-Datadog-Container-Tags", ctags)
		}
		req.Header.Set("X-Datadog-Additional-Tags", tags)
		metrics.Count("datadog.trace_agent.profile", 1, nil, 1)
	}
	logger := logutil.NewThrottled(5, 10*time.Second) // limit to 5 messages every 10 seconds
	return &httputil.ReverseProxy{
		Director:  director,
		ErrorLog:  stdlog.New(logger, "profiling.Proxy: ", 0),
		Transport: transport,
	}
}

func errorHandler(endpoint string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		msg := fmt.Sprintf("Profile forwarder is OFF because of invalid intake URL configuration: %v", endpoint)
		http.Error(w, msg, http.StatusInternalServerError)
	})
}

type nopResponseWriter struct{}

func (rw *nopResponseWriter) Header() http.Header {
	return make(map[string][]string)
}

func (rw *nopResponseWriter) Write([]byte) (int, error) {
	return 0, nil
}

func (rw *nopResponseWriter) WriteHeader(statusCode int) {}
