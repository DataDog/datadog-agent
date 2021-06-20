package api

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	stdlog "log"
	"mime"
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
	tracingURLTemplate = "https://trace.agent.%s/"
	// profilingURLDefault specifies the default intake API URL.
	tracingURLDefault = "https://api.datadoghq.com/"
)

// profilingEndpoints returns the profiling intake urls and their corresponding
// api keys based on agent configuration. The main endpoint is always returned as
// the first element in the slice.
func tracingEndpoints(apiKey string) (urls []*url.URL, apiKeys []string, err error) {
	main := tracingURLDefault
	if v := config.Datadog.GetString("apm_config.tracing_dd_url"); v != "" {
		main = v
	} else if site := config.Datadog.GetString("site"); site != "" {
		main = fmt.Sprintf(tracingURLTemplate, site)
	}
	u, err := url.Parse(main)
	if err != nil {
		// if the main intake URL is invalid we don't use additional endpoints
		return nil, nil, fmt.Errorf("error parsing main profiling intake URL %s: %v", main, err)
	}
	urls = append(urls, u)
	apiKeys = append(apiKeys, apiKey)

	if opt := "apm_config.tracing_additional_endpoints"; config.Datadog.IsSet(opt) {
		extra := config.Datadog.GetStringMapStringSlice(opt)
		for endpoint, keys := range extra {
			u, err := url.Parse(endpoint)
			if err != nil {
				log.Errorf("Error parsing additional tracing intake URL %s: %v", endpoint, err)
				continue
			}
			for _, key := range keys {
				urls = append(urls, u)
				apiKeys = append(apiKeys, key)
			}
		}
	}
	return urls, apiKeys, nil
}

// profileProxyHandler returns a new HTTP handler which will proxy requests to the profiling intakes.
// If the main intake URL can not be computed because of config, the returned handler will always
// return http.StatusInternalServerError along with a clarification.
func (r *HTTPReceiver) tracingProxyHandler() http.Handler {
	targets, keys, err := tracingEndpoints(r.conf.APIKey())
	if err != nil {
		return errorProxyHandler(err)
	}
	return newTracingProxy(r.conf.NewHTTPTransport(), targets, keys, r.conf.Hostname, r.conf.DefaultEnv)
}

func errorProxyHandler(err error) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		msg := fmt.Sprintf("Profile forwarder is OFF: %v", err)
		http.Error(w, msg, http.StatusInternalServerError)
	})
}

func isMultipart(req *http.Request) (bool, string) {
	contentType := req.Header.Get("Content-type")
	if contentType == "" {
		return false, ""
	}
	contentType, params, err := mime.ParseMediaType(contentType)

	if err != nil {
		// TODO: ? DEBUG LOG?
		return false, ""
	}
	return contentType == "multipart/form-data", ""
}

// newTracingProxy creates an http.ReverseProxy which can forward requests to
// one or more endpoints.
//
// The endpoint URLs are passed in through the targets slice. Each endpoint
// must have a corresponding API key in the same position in the keys slice.
//
// The tags will be added as a header to all proxied requests.
// For more details please see multiTransport.
func newTracingProxy(transport http.RoundTripper, targets []*url.URL, keys []string, agentHostname string, agentEnv string) *httputil.ReverseProxy {
	director := func(req *http.Request) {
		req.Header.Set("Via", fmt.Sprintf("trace-agent %s", info.Version))
		if _, ok := req.Header["User-Agent"]; !ok {
			// explicitly disable User-Agent so it's not set to the default value
			// that net/http gives it: Go-http-client/1.1
			// See https://codereview.appspot.com/7532043
			req.Header.Set("User-Agent", "")
		}

		containerID := req.Header.Get(headerContainerID)
		if ctags := getContainerTags(containerID); ctags != "" {
			// #TODO - add payload
		}

		req.Header.Get("DD-Agent-Hostname")

		req.Header.Set("DD-Agent-Hostname", agentHostname)
		req.Header.Set("DD-Agent-Env", agentEnv)

		metrics.Count("datadog.trace_agent.proxy", 1, nil, 1)
	}
	logger := logutil.NewThrottled(5, 10*time.Second) // limit to 5 messages every 10 seconds
	return &httputil.ReverseProxy{
		Director:  director,
		ErrorLog:  stdlog.New(logger, "trace.Proxy: ", 0),
		Transport: &multiProxyTransport{transport, targets, keys},
	}
}

// multiTransport sends HTTP requests to multiple targets using an
// underlying http.RoundTripper. API keys are set separately for each target.
// When multiple endpoints are in use the response from the main endpoint
// is proxied back to the client, while for all aditional endpoints the
// response is discarded. There is no de-duplication done between endpoint
// hosts or api keys.
type multiProxyTransport struct {
	rt      http.RoundTripper
	targets []*url.URL
	keys    []string
}

// func newMultiProxyTransport() *multiProxyTransport {
//
// }

func (m *multiProxyTransport) setTarget(r *http.Request, u *url.URL, apiKey string) error {
	// if !strings.HasPrefix(r.URL.Path, "/proxy/") {
	// 	return fmt.Errorf("Bad URL (WIP)")
	// }
	log.Errorf("hostname: %s", r.Host)
	// subPath := strings.Replace(r.URL.Path, "/proxy/", "/", 1)

	newUrl := *u
	// newUrl.Path = path.Join(u.Path, r.URL.Path)
	newUrl.Path = r.URL.Path

	r.Host = u.Host
	r.URL = &newUrl
	log.Errorf("Routed URLs: %s %v", u, &newUrl)

	r.Header.Set("DD-API-KEY", apiKey)
	return nil
}

func (m *multiProxyTransport) RoundTrip(req *http.Request) (*http.Response, error) {

	if len(m.targets) == 1 {
		m.setTarget(req, m.targets[0], m.keys[0])
		return m.rt.RoundTrip(req)
	}
	slurp, err := ioutil.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	var (
		rresp *http.Response
		rerr  error
	)
	for i, u := range m.targets {
		newreq := req.Clone(req.Context())
		newreq.Body = ioutil.NopCloser(bytes.NewReader(slurp))
		m.setTarget(newreq, u, m.keys[i])
		if i == 0 {
			// given the way we construct the list of targets the main endpoint
			// will be the first one called, we return its response and error
			rresp, rerr = m.rt.RoundTrip(newreq)
			continue
		}

		if resp, err := m.rt.RoundTrip(newreq); err == nil {
			// we discard responses for all subsequent requests
			io.Copy(ioutil.Discard, resp.Body)
			resp.Body.Close()
		} else {
			log.Error(err)
		}
	}
	return rresp, rerr
}
