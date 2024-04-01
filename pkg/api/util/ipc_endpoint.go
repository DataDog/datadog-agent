// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package util

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/system"
)

// IPCEndpoint is an endpoint that IPC requests will be sent to
type IPCEndpoint struct {
	client    *http.Client
	target    url.URL
	closeConn bool
}

// GetOption is an option that can be passed to DoGet
type GetOption func(url.URL) url.URL

// WithValues is an option to add url.Values to the GET request
func WithValues(values url.Values) GetOption {
	return func(u url.URL) url.URL {
		u.RawQuery = values.Encode()
		return u
	}
}

// DoGet sends GET method to the endpoint
func (end *IPCEndpoint) DoGet(options ...GetOption) ([]byte, error) {
	conn := LeaveConnectionOpen
	if end.closeConn {
		conn = CloseConnection
	}

	target := end.target
	for _, opt := range options {
		target = opt(target)
	}

	// TODO: after removing callers to api/util/DoGet, pass `end.token` instead of using global var
	res, err := DoGet(end.client, target.String(), conn)
	if err != nil {
		var errMap = make(map[string]string)
		_ = json.Unmarshal(res, &errMap) //nolint:errcheck
		// If the error has been marshalled into a json object, check it and return it properly
		if errStr, found := errMap["error"]; found {
			return nil, errors.New(errStr)
		}

		return nil, fmt.Errorf("could not reach agent: %v\nMake sure the agent is running before requesting the runtime configuration and contact support if you continue having issues", err)
	}
	return res, err
}

// EndpointOption allows configuration of the IPCEndpoint during construction
type EndpointOption func(*IPCEndpoint)

// WithCloseConnection is an option to close the connection
func WithCloseConnection(state bool) func(*IPCEndpoint) {
	return func(end *IPCEndpoint) {
		end.closeConn = state
	}
}

// WithHTTPClient is an option to assign a different http.Client
func WithHTTPClient(client *http.Client) func(*IPCEndpoint) {
	return func(end *IPCEndpoint) {
		end.client = client
	}
}

// WithURLScheme is an option to set the URL's scheme
func WithURLScheme(scheme string) func(*IPCEndpoint) {
	return func(end *IPCEndpoint) {
		end.target.Scheme = scheme
	}
}

// WithHostAndPort is an option to use a host address for sending IPC requests
// default is the config settings "cmd_host" (default localhost) and "cmd_port" (default 5001)
func WithHostAndPort(cmdHost string, cmdPort int) func(*IPCEndpoint) {
	return func(end *IPCEndpoint) {
		end.target.Host = net.JoinHostPort(cmdHost, strconv.Itoa(cmdPort))
	}
}

// NewIPCEndpoint constructs a new IPC Endpoint using the given config, path, and options
func NewIPCEndpoint(config config.Component, endpointPath string, options ...EndpointOption) (*IPCEndpoint, error) {
	// sets a global `token` in `doget.go`
	// TODO: add `token` to Endpoint struct, instead of storing it in a global var
	if err := SetAuthToken(config); err != nil {
		return nil, err
	}

	var cmdHostKey string
	// ipc_address is deprecated in favor of cmd_host, but we still need to support it
	// if it is set, use it, otherwise use cmd_host
	if config.IsSet("ipc_address") {
		log.Warn("ipc_address is deprecated, use cmd_host instead")
		cmdHostKey = "ipc_address"
	} else {
		cmdHostKey = "cmd_host"
	}

	// only IPC over localhost is currently supported
	ipcHost, err := system.IsLocalAddress(config.GetString(cmdHostKey))
	if err != nil {
		return nil, fmt.Errorf("%s: %s", cmdHostKey, err)
	}
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{Transport: tr}

	ipcPort := config.GetInt("cmd_port")
	targetURL := url.URL{
		Scheme: "https",
		Host:   net.JoinHostPort(ipcHost, strconv.Itoa(ipcPort)),
		Path:   endpointPath,
	}

	// construct the endpoint and apply any options
	endpoint := IPCEndpoint{
		client:    client,
		target:    targetURL,
		closeConn: false,
	}
	for _, opt := range options {
		opt(&endpoint)
	}
	return &endpoint, nil
}
