package http

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const contentType = "application/json"

// Destination sends a payload over HTTP.
type Destination struct {
	endpoint            config.Endpoint
	url                 string
	client              *http.Client
	destinationsContext *client.DestinationsContext
}

// NewDestination returns a new Destination.
func NewDestination(endpoint config.Endpoint, destinationsContext *client.DestinationsContext) *Destination {
	var scheme string
	if endpoint.UseSSL {
		scheme = "https"
	} else {
		scheme = "http"
	}
	return &Destination{
		endpoint: endpoint,
		url:      fmt.Sprintf("%v://%v/v1/input/%v", scheme, endpoint.Host, endpoint.APIKey),
		client: &http.Client{
			Timeout:   time.Second * 10,
			Transport: util.CreateHTTPTransport(),
		},
		destinationsContext: destinationsContext,
	}
}

// Send sends a payload over HTTP.
func (d *Destination) Send(payload []byte) error {

	ctx := d.destinationsContext.Context()
	req, err := http.NewRequest("POST", d.url, strings.NewReader(string(payload)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", contentType)
	req = req.WithContext(ctx)

	resp, err := d.client.Do(req)
	if err != nil {
		if ctx.Err() == context.Canceled {
			return ctx.Err()
		}
		log.Info(err)
		return err
	}

	defer resp.Body.Close()
	_, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode >= 500 {
		log.Debug("Internal server error")
		return fmt.Errorf("internal server error")
	} else if resp.StatusCode >= 400 {
		log.Debug("Bad request")
		return fmt.Errorf("bad request")
	} else {
		return nil
	}
}

// SendAsync is not implemented for HTTP.
func (d *Destination) SendAsync(payload []byte) {
	return
}
