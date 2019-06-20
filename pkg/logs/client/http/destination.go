package http

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/util"
)

const contentType = "application/json"

// HTTP errors
var (
	errClient = errors.New("client error")
	errServer = errors.New("server error")
)

// Destination sends a payload over HTTP.
type Destination struct {
	url                 string
	client              *http.Client
	destinationsContext *client.DestinationsContext
	once                sync.Once
	payloadChan         chan []byte
}

// NewDestination returns a new Destination.
// TODO: add support for SOCKS5
func NewDestination(endpoint config.Endpoint, destinationsContext *client.DestinationsContext) *Destination {
	return &Destination{
		url: buildURL(endpoint),
		client: &http.Client{
			Timeout: time.Second * 10,
			// reusing core agent HTTP transport to benefit from proxy settings.
			Transport: util.CreateHTTPTransport(),
		},
		destinationsContext: destinationsContext,
	}
}

// Send sends a payload over HTTP,
// the error returned can be retryable and it is the responsibility of the callee to retry.
func (d *Destination) Send(payload []byte) error {
	ctx := d.destinationsContext.Context()
	req, err := http.NewRequest("POST", d.url, strings.NewReader(string(payload)))
	if err != nil {
		// the request could not be built,
		// this can happen when the method or the url are valid.
		return err
	}
	req.Header.Set("Content-Type", contentType)
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
		payloadChan := make(chan []byte, 100)
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
				d.Send(payload)
			case <-ctx.Done():
				return
			}
		}
	}()
}

// builURL buils a url from a config endpoint.
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
