package util

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/config/setup"
)

type IPCEndpoint struct {
	client    *http.Client
	target    url.URL
	closeConn bool
}

func (end *IPCEndpoint) SetCloseConnection(state bool) {
	end.closeConn = state
}

// send GET method to the endpoint
func (end *IPCEndpoint) DoGet() ([]byte, error) {
	conn := LeaveConnectionOpen
	if end.closeConn {
		conn = CloseConnection
	}
	// TODO: after removing callers to api/util/DoGet, pass `end.token` instead of using global var
	res, err := DoGet(end.client, end.target.String(), conn)
	if err != nil {
		var errMap = make(map[string]string)
		_ = json.Unmarshal(res, &errMap) //nolint:errcheck
		// If the error has been marshalled into a json object, check it and return it properly
		if errStr, found := errMap["error"]; found {
			return nil, errors.New(errStr)
		}

		return nil, fmt.Errorf("Could not reach agent: %v\nMake sure the agent is running before requesting the runtime configuration and contact support if you continue having issues", err)
	}
	return res, err
}

func (end *IPCEndpoint) WithValues(values url.Values) *IPCEndpoint {
	end.target.RawQuery = values.Encode()
	return end
}

func NewIPCEndpoint(config config.Component, endpointPath string) (*IPCEndpoint, error) {
	// sets a global `token` in `doget.go`
	// TODO: add `token` to Endpoint struct, instead of storing it in a global var
	if err := SetAuthToken(); err != nil {
		return nil, err
	}

	// To add TLS support, use this instead
	// tr := &http.Transport{
	//   TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	// }
	// client = &http.Client{Transport: tr}
	client := &http.Client{}

	// get host:port from the config
	ipcHost, err := setup.GetIPCAddress(config)
	if err != nil {
		return nil, err
	}
	ipcPort := config.GetInt("cmd_port")
	targetURL := url.URL{
		Scheme: "https",
		Host:   fmt.Sprintf("%s:%d", ipcHost, ipcPort),
		Path:   endpointPath,
	}

	// return the encapsulated endpoint
	return &IPCEndpoint{
		client:    client,
		target:    targetURL,
		closeConn: false,
	}, nil
}
