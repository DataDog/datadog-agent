package api

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/trace/info"
	"github.com/DataDog/datadog-agent/pkg/trace/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// profilingURLTemplate specifies the template for obtaining the profiling URL along with the site.
	profilingURLTemplate = "https://intake.profile.%s/v1/input"
	// profilingURLDefault specifies the default intake API URL.
	profilingURLDefault = "https://intake.profile.datadoghq.com/v1/input"
)

// profilingEndpoint returns the profiling intake API URL based on agent configuration.
func profilingEndpoint() string {
	if v := config.Datadog.GetString("apm_config.profiling_dd_url"); v != "" {
		return v
	}
	if site := config.Datadog.GetString("site"); site != "" {
		return fmt.Sprintf(profilingURLTemplate, site)
	}
	return profilingURLDefault
}

// profileProxyHandler returns a new HTTP handler which will proxy requests to the profiling intake.
// If the URL can not be computed because of a malformed 'site' config, the returned handler will always
// return http.StatusInternalServerError along with a clarification.
func (r *HTTPReceiver) profileProxyHandler() http.Handler {
	target := profilingEndpoint()
	u, err := url.Parse(target)
	if err != nil {
		log.Errorf("Profile forwarder is OFF because of invalid intake URL: %v", err)
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			msg := fmt.Sprintf("Agent is misconfigured with an invalid intake URL: %q", target)
			http.Error(w, msg, http.StatusInternalServerError)
		})
	}
	tags := fmt.Sprintf("host:%s,default_env:%s", r.conf.Hostname, r.conf.DefaultEnv)
	return newProfileProxy(u, r.conf.APIKey(), tags)
}

// newProfileProxy creates a single-host reverse proxy with the given target, attaching
// the specified apiKey.
func newProfileProxy(target *url.URL, apiKey, tags string) *httputil.ReverseProxy {
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
	return &httputil.ReverseProxy{Director: director}
}
