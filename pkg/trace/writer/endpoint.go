package writer

import (
	"bytes"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/StackVista/stackstate-agent/pkg/trace/config"
	"github.com/StackVista/stackstate-agent/pkg/trace/info"
	log "github.com/cihub/seelog"
)

const languageHeaderKey = "X-Datadog-Reported-Languages"

// endpoint is an interface where we send the data from the Agent.
type endpoint interface {
	// Write writes the payload to the endpoint.
	write(payload *payload) error

	// baseURL returns the base URL for this endpoint. e.g. For the URL "https://trace.agent.datadoghq.eu/api/v0.2/traces"
	// it returns "https://trace.agent.datadoghq.eu".
	baseURL() string
}

// nullEndpoint is a void endpoint dropping data.
type nullEndpoint struct{}

// Write of nullEndpoint just drops the payload and log its size.
func (ne *nullEndpoint) write(payload *payload) error {
	log.Debug("null endpoint: dropping payload, size: %d", len(payload.bytes))
	return nil
}

// BaseURL implements Endpoint.
func (ne *nullEndpoint) baseURL() string { return "<nullEndpoint>" }

// retriableError is an endpoint error that signifies that the associated operation can be retried at a later point.
type retriableError struct {
	err      error
	endpoint endpoint
}

// Error returns the error string.
func (re *retriableError) Error() string {
	return fmt.Sprintf("%s: %v", re.endpoint, re.err)
}

const (
	userAgentPrefix     = "Datadog Trace Agent"
	userAgentSupportURL = "https://github.com/DataDog/datadog-trace-agent"
)

// userAgent is the computed user agent we'll use when
// communicating with Datadog
var userAgent = fmt.Sprintf(
	"%s-%s-%s (+%s)",
	userAgentPrefix, info.Version, info.GitCommit, userAgentSupportURL,
)

// datadogEndpoint sends payloads to Datadog API.
type datadogEndpoint struct {
	apiKey string
	host   string
	client *http.Client
	path   string
}

// NewEndpoints returns the set of endpoints configured in the AgentConfig, appending the given path.
// The first endpoint is the main API endpoint, followed by any additional endpoints.
func newEndpoints(conf *config.AgentConfig, path string) []endpoint {
	if !conf.Enabled {
		log.Info("API interface is disabled, flushing to /dev/null instead")
		return []endpoint{&nullEndpoint{}}
	}
	if e := conf.Endpoints; len(e) == 0 || e[0].Host == "" || e[0].APIKey == "" {
		panic(errors.New("must have at least one endpoint with key"))
	}
	endpoints := make([]endpoint, len(conf.Endpoints))
	ignoreProxy := true
	client := newClient(conf, !ignoreProxy)
	clientIgnoreProxy := newClient(conf, ignoreProxy)
	for i, e := range conf.Endpoints {
		c := client
		if e.NoProxy {
			c = clientIgnoreProxy
		}
		endpoints[i] = &datadogEndpoint{
			apiKey: e.APIKey,
			host:   e.Host,
			path:   path,
			client: c,
		}
	}
	return endpoints
}

// baseURL implements Endpoint.
func (e *datadogEndpoint) baseURL() string { return e.host }

// write will send the serialized traces payload to the Datadog traces endpoint.
func (e *datadogEndpoint) write(payload *payload) error {
	// Create the request to be sent to the API
	url := e.host + e.path
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(payload.bytes))
	if err != nil {
		return err
	}

	req.Header.Add("sts-api-key", e.apiKey)
	req.Header.Add("sts-hostname", e.host)
	req.Header.Set("User-Agent", userAgent)
	for key, value := range payload.headers {
		req.Header.Set(key, value)
	}

	resp, err := e.client.Do(req)

	if err != nil {
		return &retriableError{
			err:      err,
			endpoint: e,
		}
	}
	defer resp.Body.Close()

	// We check the status code to see if the request has succeeded.
	// TODO: define all legit status code and behave accordingly.
	if resp.StatusCode/100 != 2 {
		err := fmt.Errorf("request to %s responded with %s", url, resp.Status)
		if resp.StatusCode/100 == 5 {
			// 5xx errors are retriable
			return &retriableError{
				err:      err,
				endpoint: e,
			}
		}

		// All others aren't
		return err
	}

	// Everything went fine
	return nil
}

func (e *datadogEndpoint) String() string {
	return fmt.Sprintf("DataDogEndpoint(%q)", e.host+e.path)
}

// timeout is the HTTP timeout for POST requests to the Datadog backend
const timeout = 10 * time.Second

// newClient returns a http.Client configured with the Agent options.
func newClient(conf *config.AgentConfig, ignoreProxy bool) *http.Client {
	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
			DualStack: true,
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
	return &http.Client{Timeout: timeout, Transport: transport}
}
