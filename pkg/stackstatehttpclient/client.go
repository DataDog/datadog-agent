package stackstatehttpclient

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"github.com/StackVista/stackstate-agent/pkg/trace/config"
	"github.com/StackVista/stackstate-agent/pkg/trace/info"
	"github.com/StackVista/stackstate-agent/pkg/util/log"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"strings"
	"time"
)

// GET
const GET = "GET"
// POST
const POST = "POST"
// PUT
const PUT = "PUT"

// StackStateClient creates a http client to communicate to StackState
type StackStateClient struct {
	*config.Endpoint
	*http.Client
}

// NewStackStateClient returns a StackStateClient containing a http.Client configured with the Agent options.
func NewStackStateClient(conf *config.AgentConfig, ignoreProxy bool) *StackStateClient {
	endpoint := conf.Endpoints[0]
	client := newClient(conf, false)
	if endpoint.NoProxy {
		client = newClient(conf, true)
	}
	return &StackStateClient{Client: client, Endpoint: endpoint}
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

// Get
func (sc *StackStateClient) Get(path string) (*http.Response, error) {
	return sc.Request(GET, path, nil)
}

// Put
func (sc *StackStateClient) Put(path string, body []byte) (*http.Response, error) {
	return sc.Request(GET, path, body)
}

// Post
func (sc *StackStateClient) Post(path string, body []byte) (*http.Response, error) {
	return sc.Request(GET, path, body)
}

// Request
func (sc *StackStateClient) Request(method, path string, body []byte) (*http.Response, error) {
	url := fmt.Sprintf("%s/%s", sc.Host, path)
	var req *http.Request
	var err error
	if body != nil {
		req, err = http.NewRequest(method, url, bytes.NewBuffer(body))
	} else {
		req, err = http.NewRequest(method, url, nil)
	}

	if err != nil {
		return nil, fmt.Errorf("could not create request to %s/features: %s", url, err)
	}

	req.Header.Add("content-encoding", "identity")
	req.Header.Add("sts-api-key", sc.APIKey)
	req.Header.Add("sts-hostname", sc.Host)
	req.Header.Add("sts-agent-version", info.Version)

	resp, err := sc.Do(req)
	if err != nil {
		if sc.isHTTPTimeout(err) {
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
func (sc *StackStateClient) isHTTPTimeout(err error) bool {
	if netErr, ok := err.(interface {
		Timeout() bool
	}); ok && netErr.Timeout() {
		return true
	} else if strings.Contains(err.Error(), "use of closed network connection") { //To deprecate when using GO > 1.5
		return true
	}
	return false
}
