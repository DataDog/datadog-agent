package http

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"sync"
	"time"

	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
)

// ContentType options,
const (
	TextContentType = "text/plain"
	JSONContentType = "application/json"
)

// HTTP errors.
var (
	errClient = errors.New("client error")
	errServer = errors.New("server error")
)

// emptyPayload is an empty payload used to check HTTP connectivity without sending logs.
var emptyPayload []byte

// Destination sends a payload over HTTP.
type Destination struct {
	url                 string
	contentType         string
	contentEncoding     ContentEncoding
	client              *httputils.Client
	destinationsContext *client.DestinationsContext
	once                sync.Once
	payloadChan         chan []byte
}

// NewDestination returns a new Destination.
// TODO: add support for SOCKS5
func NewDestination(endpoint config.Endpoint, contentType string, destinationsContext *client.DestinationsContext) *Destination {
	return newDestination(endpoint, contentType, destinationsContext, time.Second*10)
}

func newDestination(endpoint config.Endpoint, contentType string, destinationsContext *client.DestinationsContext, timeout time.Duration) *Destination {
	return &Destination{
		url:                 buildURL(endpoint),
		contentType:         contentType,
		contentEncoding:     buildContentEncoding(endpoint),
		client:              httputils.NewClient(endpoint.ConnectionResetInterval, httpClientFactory(timeout)),
		destinationsContext: destinationsContext,
	}
}

// Send sends a payload over HTTP,
// the error returned can be retryable and it is the responsibility of the callee to retry.
func (d *Destination) Send(payload []byte) error {
	ctx := d.destinationsContext.Context()

	encodedPayload, err := d.contentEncoding.encode(payload)
	if err != nil {
		return err
	}
	metrics.BytesSent.Add(int64(len(payload)))
	metrics.EncodedBytesSent.Add(int64(len(encodedPayload)))

	req, err := http.NewRequest("POST", d.url, bytes.NewReader(encodedPayload))
	if err != nil {
		// the request could not be built,
		// this can happen when the method or the url are valid.
		return err
	}
	req.Header.Set("Content-Type", d.contentType)
	req.Header.Set("Content-Encoding", d.contentEncoding.name())
	req = req.WithContext(ctx)

	resp, err := d.client.Do(req)
	if err != nil {
		if ctx.Err() == context.Canceled {
			return ctx.Err()
		}
		// most likely a network or a connect error, the callee should retry.
		return client.NewRetryableError(err)
	}

	defer resp.Body.Close()
	_, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		// the read failed because the server closed or terminated the connection
		// *after* serving the request.
		return err
	}

	if resp.StatusCode >= 500 {
		// the server could not serve the request,
		// most likely because of an internal error
		return client.NewRetryableError(errServer)
	} else if resp.StatusCode >= 400 {
		// the logs-agent is likely to be misconfigured,
		// the URL or the API key may be wrong.
		return errClient
	} else {
		return nil
	}
}

// SendAsync sends a payload in background.
func (d *Destination) SendAsync(payload []byte) {
	d.once.Do(func() {
		payloadChan := make(chan []byte, config.ChanSize)
		d.sendInBackground(payloadChan)
		d.payloadChan = payloadChan
	})
	d.payloadChan <- payload
}

// sendInBackground sends all payloads from payloadChan in background.
func (d *Destination) sendInBackground(payloadChan chan []byte) {
	ctx := d.destinationsContext.Context()
	go func() {
		for {
			select {
			case payload := <-payloadChan:
				d.Send(payload) //nolint:errcheck
			case <-ctx.Done():
				return
			}
		}
	}()
}

func httpClientFactory(timeout time.Duration) func() *http.Client {
	return func() *http.Client {
		return &http.Client{
			Timeout: timeout,
			// reusing core agent HTTP transport to benefit from proxy settings.
			Transport: httputils.CreateHTTPTransport(),
		}
	}
}

// buildURL buils a url from a config endpoint.
func buildURL(endpoint config.Endpoint) string {
	var scheme string
	if endpoint.UseSSL {
		scheme = "https"
	} else {
		scheme = "http"
	}
	var address string
	if endpoint.Port != 0 {
		address = fmt.Sprintf("%v:%v", endpoint.Host, endpoint.Port)
	} else {
		address = endpoint.Host
	}
	return fmt.Sprintf("%v://%v/v1/input/%v", scheme, address, endpoint.APIKey)
}

func buildContentEncoding(endpoint config.Endpoint) ContentEncoding {
	if endpoint.UseCompression {
		return NewGzipContentEncoding(endpoint.CompressionLevel)
	}
	return IdentityContentType
}

// CheckConnectivity check if sending logs through HTTP works
func CheckConnectivity(endpoint config.Endpoint) config.HTTPConnectivity {
	log.Info("Checking HTTP connectivity...")
	ctx := client.NewDestinationsContext()
	ctx.Start()
	defer ctx.Stop()
	// Lower the timeout to 5s because HTTP connectivity test is done synchronously during the agent bootstrap sequence
	destination := newDestination(endpoint, JSONContentType, ctx, time.Second*5)
	log.Infof("Sending HTTP connectivity request to %s...", destination.url)
	err := destination.Send(emptyPayload)
	if err != nil {
		log.Warnf("HTTP connectivity failure: %v", err)
	} else {
		log.Info("HTTP connectivity successful")
	}
	return err == nil
}
