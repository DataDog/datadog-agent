package httpclient

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"time"
)

const (
	httpRequestAuthTimeout = time.Second * 30
	minTLSVersion          = tls.VersionTLS12
)

type RunnerHttpClient struct{}

type RunnerHttpClientConfig struct {
	MaxRedirect        int
	Transport          *RunnerHttpTransportConfig
	AllowIMDSEndpoints bool
}

type RunnerHttpTransportConfig struct {
	InsecureSkipVerify bool
}

func NewRunnerHttpClient(clientConfig *RunnerHttpClientConfig) (*http.Client, error) {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			MinVersion: minTLSVersion,
		},
	}

	if clientConfig.Transport != nil {
		if clientConfig.Transport.InsecureSkipVerify {
			transport.TLSClientConfig.InsecureSkipVerify = true
		}
	}

	client := &http.Client{
		Timeout:   httpRequestAuthTimeout,
		Transport: transport,
	}
	if clientConfig.MaxRedirect != 0 {
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			if len(via) >= clientConfig.MaxRedirect {
				return fmt.Errorf("stopped after %d redirects", clientConfig.MaxRedirect)
			}
			return nil
		}
	}
	// FIXME NEED TO USE THE IMDS BLOCKER
	return client, nil
}
