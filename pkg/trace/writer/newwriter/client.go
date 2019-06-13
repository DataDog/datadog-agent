package writer

import (
	"crypto/tls"
	"errors"
	"math"
	"net"
	"net/http"
	"net/url"
	"time"

	coreconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/osutil"
	"github.com/DataDog/datadog-agent/pkg/util"
)

// timeout is the HTTP timeout for POST requests to the Datadog backend
const timeout = 10 * time.Second

// newSenders returns a list of senders based on the given agent configuration, using climit
// as the maximum number of concurrent outgoing connections, writing to path. The given
// namespace is used as a prefix for reported metrics.
func newSenders(cfg *config.AgentConfig, r eventRecorder, path string, climit int) []*sender {
	if e := cfg.Endpoints; len(e) == 0 || e[0].Host == "" || e[0].APIKey == "" {
		panic(errors.New("config was not properly validated"))
	}
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: cfg.SkipSSLValidation},
		// below field values are from http.DefaultTransport (go1.12)
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
			DualStack: true,
		}).DialContext,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	if p := coreconfig.GetProxies(); p != nil {
		transport.Proxy = util.GetProxyTransportFunc(p)
	}
	client := &http.Client{
		Timeout:   10 * time.Second,
		Transport: transport,
	}
	// spread out the the maximum connection limit (climit) between senders
	maxConns := math.Min(1, float64(climit/len(cfg.Endpoints)))
	senders := make([]*sender, len(cfg.Endpoints))
	for i, endpoint := range cfg.Endpoints {
		url, err := url.Parse(endpoint.Host + path)
		if err != nil {
			osutil.Exitf("Invalid host endpoint: %q", endpoint.Host)
		}
		senders[i] = newSender(&senderConfig{
			client:   client,
			maxConns: int(maxConns),
			url:      url,
			apiKey:   endpoint.APIKey,
			recorder: r,
		})
	}
	return senders
}
