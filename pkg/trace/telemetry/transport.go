package telemetry

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	stdlog "log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/info"
	"github.com/DataDog/datadog-agent/pkg/trace/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// MultiTransport sends HTTP requests to multiple targets using an
// underlying http.RoundTripper. API keys are set separately for each target.
// The target hostname
// When multiple endpoints are in use the response from the main endpoint
// is proxied back to the client, while for all aditional endpoints the
// response is discarded. There is no de-duplication done between endpoint
// hosts or api keys.
//
// can be extended in the future to allow supporting more product endpoints
// by simply parametrizing metric tags, and logger names
type MultiTransport struct {
	Transport         http.RoundTripper
	BaseTarget        Target
	AdditionalTargets []Target
	Director          func(*http.Request)
}

// Target describes a pair of URL and corresponding API key, used in sending requests
// to multiple backends with different API keys
type Target struct {
	url    *url.URL
	apiKey string
}

func (m *MultiTransport) roundTrip(req *http.Request, target *Target) (*http.Response, error) {
	now := time.Now()
	tags := []string{
		"type:telemetry", fmt.Sprintf("target_host:%s", target.url.Host),
	}

	req.Host = target.url.Host
	req.URL.Host = target.url.Host
	req.URL.Scheme = target.url.Scheme

	req.Header.Set("DD-API-KEY", target.apiKey)

	m.Director(req)

	resp, err := m.Transport.RoundTrip(req)
	metrics.Timing("datadog.trace_agent.proxy.roundtrip.duration", time.Since(now), tags, 1)
	if err != nil {
		metrics.Count("datadog.trace_agent.proxy.roundtrip.errors", 1, tags, 1)
	}
	return resp, err
}

// NewReverseProxy creates an http.ReverseProxy which will forward requests via transport
func NewReverseProxy(transport http.RoundTripper, logger *stdlog.Logger) http.Handler {
	director := func(req *http.Request) {
		req.Header.Set("Via", fmt.Sprintf("trace-agent %s", info.Version))
		if _, ok := req.Header["User-Agent"]; !ok {
			// explicitly disable User-Agent so it's not set to the default value
			// that net/http gives it: Go-http-client/1.1
			// See https://codereview.appspot.com/7532043
			req.Header.Set("User-Agent", "")
		}
	}

	return &httputil.ReverseProxy{
		Director:  director,
		ErrorLog:  logger,
		Transport: transport,
	}
}

// RoundTrip sends request first to MainTarget, then sends a copy of main request to every configurged
// AdditionalTargets receiptient.
//
// All requests will be sent irregardless of any errors
// If any request fails, the error will be logged. Only main target's
// error will be propagated via return value
func (m *MultiTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if len(m.AdditionalTargets) == 0 {
		return m.roundTrip(req, &m.BaseTarget)
	}

	slurp, err := ioutil.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}

	newreq := req.Clone(req.Context())
	newreq.Body = ioutil.NopCloser(bytes.NewReader(slurp))
	rresp, rerr := m.roundTrip(newreq, &m.BaseTarget)

	for _, target := range m.AdditionalTargets {
		newreq := req.Clone(req.Context())
		newreq.Body = ioutil.NopCloser(bytes.NewReader(slurp))
		if resp, err := m.roundTrip(newreq, &target); err == nil {
			// we discard responses for all subsequent requests
			io.Copy(ioutil.Discard, resp.Body)
			resp.Body.Close()
		} else {
			log.Error(err)
		}
	}
	return rresp, rerr
}
