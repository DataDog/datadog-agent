// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package httphelpers implements helpers for HTTP communication between Agent processes
package httphelpers

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/system"
)

type ipcClient struct {
	http.Client
	authToken string
	config    pkgconfigmodel.Reader
}

// NewClient creates a new secure client
func NewClient(authToken string, clientTLSConfig *tls.Config, config pkgconfigmodel.Reader) ipc.HTTPClient {
	tr := &http.Transport{
		TLSClientConfig: clientTLSConfig,
	}

	return &ipcClient{
		Client:    http.Client{Transport: tr},
		authToken: authToken,
		config:    config,
	}
}

func (s *ipcClient) Get(url string, opts ...ipc.RequestOption) (resp []byte, err error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	return s.Do(req, opts...)
}

func (s *ipcClient) Head(url string, opts ...ipc.RequestOption) (resp []byte, err error) {
	req, err := http.NewRequest("HEAD", url, nil)
	if err != nil {
		return nil, err
	}

	return s.Do(req, opts...)
}

func (s *ipcClient) Do(req *http.Request, opts ...ipc.RequestOption) (resp []byte, err error) {
	return s.do(req, "application/json", nil, opts...)
}

func (s *ipcClient) Post(url string, contentType string, body io.Reader, opts ...ipc.RequestOption) (resp []byte, err error) {
	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return nil, err
	}

	return s.do(req, contentType, nil, opts...)
}

func (s *ipcClient) PostChunk(url string, contentType string, body io.Reader, onChunk func([]byte), opts ...ipc.RequestOption) (err error) {
	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return err
	}

	_, err = s.do(req, contentType, onChunk, opts...)
	return err
}

func (s *ipcClient) PostForm(url string, data url.Values, opts ...ipc.RequestOption) (resp []byte, err error) {
	return s.Post(url, "application/x-www-form-urlencoded", strings.NewReader(data.Encode()), opts...)
}

func (s *ipcClient) do(req *http.Request, contentType string, onChunk func([]byte), opts ...ipc.RequestOption) (resp []byte, err error) {

	// Apply all options to the request
	for _, opt := range opts {
		req = opt(req)
	}

	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Authorization", "Bearer "+s.authToken)

	r, err := s.Client.Do(req)

	if err != nil {
		return resp, err
	}

	// If onChunk is provided, read the body and call the callback for each chunk
	if onChunk != nil {
		var m int
		buf := make([]byte, 4096)
		for {
			m, err = r.Body.Read(buf)
			if m < 0 || err != nil {
				break
			}
			onChunk(buf[:m])
		}
		r.Body.Close()
		if r.StatusCode == 200 {
			return nil, nil
		}
		return nil, err
	}

	// If onChunk is not provided, read the body and return it
	body, err := io.ReadAll(r.Body)
	r.Body.Close()
	if err != nil {
		return body, err
	}

	if r.StatusCode >= 400 {
		return body, errors.New(string(body))
	}
	return body, nil
}

// IPCEndpoint section

// IPCEndpoint is an endpoint that IPC requests will be sent to
type IPCEndpoint struct {
	client    ipc.HTTPClient
	target    url.URL
	closeConn bool
}

// NewIPCEndpoint constructs a new IPC Endpoint using the given config, path, and options
func (s *ipcClient) NewIPCEndpoint(endpointPath string) (ipc.Endpoint, error) {
	var cmdHostKey string
	// ipc_address is deprecated in favor of cmd_host, but we still need to support it
	// if it is set, use it, otherwise use cmd_host
	if s.config.IsConfigured("ipc_address") {
		log.Warn("ipc_address is deprecated, use cmd_host instead")
		cmdHostKey = "ipc_address"
	} else {
		cmdHostKey = "cmd_host"
	}

	// only IPC over localhost is currently supported
	ipcHost, err := system.IsLocalAddress(s.config.GetString(cmdHostKey))
	if err != nil {
		return nil, fmt.Errorf("%s: %s", cmdHostKey, err)
	}

	ipcPort := s.config.GetInt("cmd_port")
	targetURL := url.URL{
		Scheme: "https",
		Host:   net.JoinHostPort(ipcHost, strconv.Itoa(ipcPort)),
		Path:   endpointPath,
	}

	// construct the endpoint and apply any options
	endpoint := IPCEndpoint{
		client:    s,
		target:    targetURL,
		closeConn: false,
	}
	return &endpoint, nil
}

// DoGet sends GET method to the endpoint
func (end *IPCEndpoint) DoGet(options ...ipc.RequestOption) ([]byte, error) {
	target := end.target

	// TODO: after removing callers to api/util/DoGet, pass `end.token` instead of using global var
	res, err := end.client.Get(target.String(), options...)
	if err != nil {
		var errMap = make(map[string]string)
		_ = json.Unmarshal(res, &errMap) //nolint:errcheck
		// If the error has been marshalled into a json object, check it and return it properly
		if errStr, found := errMap["error"]; found {
			return nil, errors.New(errStr)
		}
		netErr := new(net.OpError)
		if errors.As(err, &netErr) {
			return nil, fmt.Errorf("could not reach agent: %v\nMake sure the agent is running before requesting the runtime configuration and contact support if you continue having issues", err)
		}
		return nil, err
	}
	return res, err
}

// WithCloseConnection is a request option that closes the connection after the request
func WithCloseConnection(req *http.Request) *http.Request {
	req.Close = true
	return req
}

// WithLeaveConnectionOpen is a request option that leaves the connection open after the request
func WithLeaveConnectionOpen(req *http.Request) *http.Request {
	req.Close = false
	return req
}

// WithContext is a request option that sets the context for the request
func WithContext(ctx context.Context) ipc.RequestOption {
	return func(req *http.Request) *http.Request {
		req = req.WithContext(ctx)
		return req
	}
}

// WithValues is a request option that sets the values for the request
func WithValues(values url.Values) ipc.RequestOption {
	return func(req *http.Request) *http.Request {
		req.URL.RawQuery = values.Encode()
		return req
	}
}
