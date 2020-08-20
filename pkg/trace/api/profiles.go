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
)

// mainProfilingEndpoint returns the main profiling intake API URL based on agent
// configuration. When multiple endpoints are in use, the response from the main
// endpoint is proxied back to the client, while for all aditional endpoints the
// response is discarded.
func mainProfilingEndpoint() string {
	if v := config.Datadog.GetString("apm_config.profiling_dd_url"); v != "" {
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
	if config.Datadog.IsSet("apm_config.profiling_additional_endpoints") {
		return config.Datadog.GetStringMapStringSlice("apm_config.profiling_additional_endpoints")
	}
	return make(map[string][]string)
}

// profileProxyHandler returns a new HTTP handler which will proxy requests to the profiling intakes.
// If the main intake URL can not be computed because of config, the returned handler will always
// return http.StatusInternalServerError along with a clarification.
func (r *HTTPReceiver) profileProxyHandler() http.Handler {
	e := mainProfilingEndpoint()
	u, err := url.Parse(e)
	if err != nil {
		log.Errorf("Error parsing main intake URL %s: %v", e, err)
		return errorHandler(e)
	}
	targets, keys := []*url.URL{u}, []string{r.conf.APIKey()}

	for e, ks := range additionalProfilingEndpoints() {
		u, err := url.Parse(e)
		if err != nil {
			log.Errorf("Error parsing additional intake URL %s: %v", e, err)
			continue
		}
		for _, k := range ks {
			targets = append(targets, u)
			keys = append(keys, k)
		}
	}
	tags := fmt.Sprintf("host:%s,default_env:%s", r.conf.Hostname, r.conf.DefaultEnv)
	return newProfileProxy(r.conf.NewHTTPTransport(), targets, keys, tags)
}

func errorHandler(endpoint string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		msg := fmt.Sprintf("Profile forwarder is OFF because of invalid intake URL configuration: %v", endpoint)
		http.Error(w, msg, http.StatusInternalServerError)
	})
}

// newProfileProxy creates a single-host reverse proxy with the given target, attaching
// the specified apiKey.
func newProfileProxy(transport http.RoundTripper, targets []*url.URL, keys []string, tags string) *httputil.ReverseProxy {
	director := func(req *http.Request) {
		// URL, Host and key are set in the transport for each outbound request
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
		Transport: &multiTransport{transport, targets, keys},
	}
}

type multiTransport struct {
	rt      http.RoundTripper
	targets []*url.URL
	keys    []string
}

func (m *multiTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	setTarget := func(r *http.Request, u *url.URL, apiKey string) {
		r.Host = u.Host
		r.URL = u
		r.Header.Set("DD-API-KEY", apiKey)
	}
	if len(m.targets) == 1 {
		setTarget(req, m.targets[0], m.keys[0])
		return m.rt.RoundTrip(req)
	}
	slurp, err := ioutil.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	var resp *http.Response
	var rerr error
	for i, u := range m.targets {
		newreq := req.Clone(req.Context())
		newreq.Body = ioutil.NopCloser(bytes.NewReader(slurp))
		setTarget(newreq, u, m.keys[i])
		if i == 0 {
			// given the way we construct the list of targets the main endpoint
			// will be the first one called, we return its response and error
			resp, rerr = m.rt.RoundTrip(newreq)
			err = rerr
		} else {
			// we discard responses for all subsequent requests and log all errors
			_, err = m.rt.RoundTrip(newreq)
		}
		if err != nil {
			log.Error(err)
		}
	}
	return resp, rerr
}
