package http

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const httpPath = "/v1/input/"
const contentType = "application/json"

type Destination struct {
	endpoint config.Endpoint
	url      string
}

func NewDestination(endpoint config.Endpoint) *Destination {
	url := endpoint.Host + httpPath + endpoint.APIKey
	return &Destination{
		endpoint: endpoint,
		url:      url,
	}
}

// TODO(achntrl): Have a buffering mechanism where we aggregate a bunch of logs before sending them as a batch
// to limit the number of requests
func (d *Destination) Send(payload []byte) error {
	// TODO(achntrl) Create a client or a transport
	resp, err := http.Post(d.url, contentType, strings.NewReader(string(payload)))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	// TODO(achntrl) make sure the response is handled properly to keep connection alive
	_, _ = ioutil.ReadAll(resp.Body)

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

func (d *Destination) SendAsync(payload []byte) {
	return
}
