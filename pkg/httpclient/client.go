package httpclient

import (
	"bytes"
	"crypto/tls"
	"errors"
	"fmt"
	"github.com/StackVista/stackstate-agent/pkg/config"
	"github.com/StackVista/stackstate-agent/pkg/trace/info"
	"github.com/StackVista/stackstate-agent/pkg/trace/watchdog"
	"github.com/StackVista/stackstate-agent/pkg/util/log"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// GET is used for HTTP GET calls
const GET = "GET"

// POST is used for HTTP POST calls
const POST = "POST"

// PUT is used for HTTP PUT calls
const PUT = "PUT"

// HttpResponse is used to represent the response from the request
type HttpResponse struct {
	Response    *http.Response
	Body        []byte
	RetriesLeft int
	Err         error
}

// ClientHost specifies an host that the client communicates with.
type ClientHost struct {
	APIKey string `json:"-"` // never marshal this
	Host   string

	// NoProxy will be set to true when the proxy setting for the trace API endpoint
	// needs to be ignored (e.g. it is part of the "no_proxy" list in the yaml settings).
	NoProxy           bool
	ProxyURL          *url.URL
	SkipSSLValidation bool
}

// RetryableHttpClient creates a http client to communicate to StackState
type RetryableHttpClient struct {
	*ClientHost
	*http.Client
	mux sync.Mutex
}

// NewStackStateClient returns a RetryableHttpClient containing a http.Client configured with the Agent options.
func NewStackStateClient() *RetryableHttpClient {
	return retryableHttpClient("sts_url")
}

// NewStackStateClient returns a RetryableHttpClient containing a http.Client configured with the Agent options.
func NewHttpClient(baseUrlConfigKey string) *RetryableHttpClient {
	return retryableHttpClient(baseUrlConfigKey)
}

func retryableHttpClient(baseUrlConfigKey string) *RetryableHttpClient {
	host := &ClientHost{}
	if hostUrl := config.Datadog.GetString(baseUrlConfigKey); hostUrl != "" {
		host.Host = hostUrl
	}

	proxyList := config.Datadog.GetStringSlice("proxy.no_proxy")
	noProxy := make(map[string]bool, len(proxyList))
	for _, host := range proxyList {
		// map of hosts that need to be skipped by proxy
		noProxy[host] = true
	}
	host.NoProxy = noProxy[host.Host]

	if addr := config.Datadog.GetString("proxy.https"); addr != "" {
		url, err := url.Parse(addr)
		if err == nil {
			host.ProxyURL = url
		} else {
			log.Errorf("Failed to parse proxy URL from proxy.https configuration: %s", err)
		}
	}

	if config.Datadog.IsSet("skip_ssl_validation") {
		host.SkipSSLValidation = config.Datadog.GetBool("skip_ssl_validation")
	}

	return &RetryableHttpClient{
		ClientHost: host,
		Client:     newClient(host),
	}
}

// newClient returns a http.Client configured with the Agent options.
func newClient(host *ClientHost) *http.Client {
	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		TLSClientConfig:       &tls.Config{InsecureSkipVerify: host.SkipSSLValidation},
	}
	if host.ProxyURL != nil && !host.NoProxy {
		log.Infof("configuring proxy through: %s", host.ProxyURL.String())
		transport.Proxy = http.ProxyURL(host.ProxyURL)
	}
	return &http.Client{Timeout: 30 * time.Second, Transport: transport}
}

// Get performs a GET request to some path
func (rc *RetryableHttpClient) Get(path string) *HttpResponse {
	return rc.requestRetryHandler(GET, path, nil, 5*time.Second, 5)
}

// GetWithRetry performs a GET request to some path with a set retry interval and count
func (rc *RetryableHttpClient) GetWithRetry(path string, retryInterval time.Duration, retryCount int) *HttpResponse {
	return rc.requestRetryHandler(GET, path, nil, retryInterval, retryCount)
}

// Put performs a PUT request to some path
func (rc *RetryableHttpClient) Put(path string, body []byte) *HttpResponse {
	return rc.requestRetryHandler(PUT, path, body, 5*time.Second, 5)
}

// PutWithRetry performs a PUT request to some path with a set retry interval and count
func (rc *RetryableHttpClient) PutWithRetry(path string, body []byte, retryInterval time.Duration, retryCount int) *HttpResponse {
	return rc.requestRetryHandler(PUT, path, body, retryInterval, retryCount)
}

// Post performs a POST request to some path
func (rc *RetryableHttpClient) Post(path string, body []byte) *HttpResponse {
	return rc.requestRetryHandler(POST, path, body, 5*time.Second, 5)
}

// PostWithRetry performs a POST request to some path with a set retry interval and count
func (rc *RetryableHttpClient) PostWithRetry(path string, body []byte, retryInterval time.Duration, retryCount int) *HttpResponse {
	return rc.requestRetryHandler(POST, path, body, retryInterval, retryCount)
}

func (rc *RetryableHttpClient) requestRetryHandler(method, path string, body []byte, retryInterval time.Duration, retryCount int) *HttpResponse {
	retryTicker := time.NewTicker(retryInterval)
	retriesLeft := retryCount
	responseChan := make(chan *HttpResponse, 1)
	var response *HttpResponse

	defer watchdog.LogOnPanic()
	defer close(responseChan)
retry:
	for {
		select {
		case <-retryTicker.C:
			rc.handleRequest(method, path, body, retriesLeft, responseChan)
			rc.mux.Lock()
			// Lock so we can decrement the retriesLeft
			retriesLeft = retriesLeft - 1
			rc.mux.Unlock()
		case response = <-responseChan:
			// Stop retrying and return the response
			retryTicker.Stop()
			break retry
		}
	}
	rc.mux.Lock()
	response.RetriesLeft = retriesLeft
	rc.mux.Unlock()
	return response
}

// getSupportedFeatures returns the features supported by the StackState API
func (rc *RetryableHttpClient) handleRequest(method, path string, body []byte, retriesLeft int, responseChan chan *HttpResponse) {
	rc.mux.Lock()
	// Lock so only one goroutine at a time can access the map
	if retriesLeft == 0 {
		responseChan <- &HttpResponse{Err: errors.New("failed after all retries")}
	}
	rc.mux.Unlock()

	response, err := rc.makeRequest(method, path, body)

	// Handle error response
	if err != nil {
		// Soo we got a 404, meaning we were able to contact StackState, but it did not have the requested path. We can publish a result
		if response != nil {
			//responseChan <- &HttpResponse{
			//	RetriesLeft: retriesLeft,
			//	Err: errors.New("found StackState version which does not have the requested path"),
			//}
			return
		}
		// Log
		_ = log.Error(err)
		return
	}

	defer response.Body.Close()

	// Get byte array
	body, err = ioutil.ReadAll(response.Body)
	if err != nil {
		_ = log.Errorf("could not decode response body from request: %s", err)
		return
	}

	responseChan <- &HttpResponse{Response: response, Body: body, Err: nil}
}

// makeRequest
func (rc *RetryableHttpClient) makeRequest(method, path string, body []byte) (*http.Response, error) {
	url := fmt.Sprintf("%s/%s", rc.Host, path)
	var req *http.Request
	var err error
	if body != nil {
		req, err = http.NewRequest(method, url, bytes.NewBuffer(body))
	} else {
		req, err = http.NewRequest(method, url, nil)
	}

	if err != nil {
		return nil, fmt.Errorf("could not create request to %s/%s: %s", url, path, err)
	}

	req.Header.Add("content-encoding", "identity")
	req.Header.Add("sts-api-key", rc.APIKey)
	req.Header.Add("sts-hostname", rc.Host)
	req.Header.Add("sts-agent-version", info.Version)

	resp, err := rc.Do(req)
	if err != nil {
		if rc.isHTTPTimeout(err) {
			return nil, fmt.Errorf("timeout detected on %s, %s", url, err)
		}
		return nil, fmt.Errorf("error submitting payload to %s: %s", url, err)
	}

	if resp.StatusCode < 200 || resp.StatusCode > 300 {
		defer resp.Body.Close()
		io.Copy(ioutil.Discard, resp.Body)
		return resp, fmt.Errorf("unexpected response from %s. Status: %s", url, resp.Status)
	}

	return resp, nil
}

// IsTimeout returns true if the error is due to reaching the timeout limit on the http.client
func (rc *RetryableHttpClient) isHTTPTimeout(err error) bool {
	if netErr, ok := err.(interface {
		Timeout() bool
	}); ok && netErr.Timeout() {
		return true
	} else if strings.Contains(err.Error(), "use of closed network connection") { //To deprecate when using GO > 1.5
		return true
	}
	return false
}
