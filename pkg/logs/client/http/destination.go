package http

import (
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const httpPath = "/v1/input/"
const contentType = "application/json"

// Destination sends a payload over HTTP.
type Destination struct {
	endpoint config.Endpoint
	url      string
	client   *http.Client
}

// NewDestination returns a new Destination.
func NewDestination(endpoint config.Endpoint) *Destination {
	url := endpoint.Host + httpPath + endpoint.APIKey
	netTransport := &http.Transport{
		MaxIdleConns:        10,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     30 * time.Second,
		DisableCompression:  true,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
			DualStack: true,
		}).DialContext,
	}
	var netClient = &http.Client{
		Timeout:   time.Second * 10,
		Transport: netTransport,
	}
	return &Destination{
		endpoint: endpoint,
		url:      url,
		client:   netClient,
	}
}

// Send sends a payload over HTTP.
func (d *Destination) Send(payload []byte) error {
	// TODO(achntrl) Create a client or a transport
	resp, err := d.client.Post(d.url, contentType, strings.NewReader(string(payload)))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	// TODO(achntrl) make sure the response is handled properly to keep connection alive
	_, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	// TODO(achntrl): Count failures here
	if resp.StatusCode >= 400 && resp.StatusCode < 500 {
		log.Info("Bad request")
		return fmt.Errorf("bad request")
	}
	if resp.StatusCode >= 500 {
		log.Info("Internal server error")
		return fmt.Errorf("internal server error, need to retry")
	}
	return nil
}

// SendAsync is not implemented for HTTP.
func (d *Destination) SendAsync(payload []byte) {
	return
}
