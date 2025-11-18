// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package httphelpers implements helpers for HTTP communication between Agent processes
package httphelpers

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

type udsClient struct {
	innerClient http.Client
	authToken   string
	config      pkgconfigmodel.Reader
}

// NewClient creates a new secure client
func NewUDSClient(authToken string, clientTLSConfig *tls.Config, config pkgconfigmodel.Reader) ipc.HTTPClient {
	var tr *http.Transport
	if pkgconfigsetup.Datadog().GetBool("agent_ipc.use_uds") {
		socketPath := pkgconfigsetup.Datadog().GetString("agent_ipc.socket_path")
		ipcSocket := socketPath + "/agent_ipc.socket"
		tr = &http.Transport{
			TLSClientConfig: clientTLSConfig,
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", ipcSocket)
			},
		}
	} else {
		tr = &http.Transport{
			TLSClientConfig: clientTLSConfig,
		}
	}

	return &udsClient{
		innerClient: http.Client{Transport: tr},
		authToken:   authToken,
		config:      config,
	}
}

func (s *udsClient) Get(url string, opts ...ipc.RequestOption) (resp []byte, err error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	return s.Do(req, opts...)
}

func (s *udsClient) Head(url string, opts ...ipc.RequestOption) (resp []byte, err error) {
	req, err := http.NewRequest("HEAD", url, nil)
	if err != nil {
		return nil, err
	}

	return s.Do(req, opts...)
}

func (s *udsClient) Do(req *http.Request, opts ...ipc.RequestOption) (resp []byte, err error) {
	return s.do(req, "application/json", nil, opts...)
}

func (s *udsClient) Post(url string, contentType string, body io.Reader, opts ...ipc.RequestOption) (resp []byte, err error) {
	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return nil, err
	}

	return s.do(req, contentType, nil, opts...)
}

func (s *udsClient) PostChunk(url string, contentType string, body io.Reader, onChunk func([]byte), opts ...ipc.RequestOption) (err error) {
	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return err
	}

	_, err = s.do(req, contentType, onChunk, opts...)
	return err
}

func (s *udsClient) PostForm(url string, data url.Values, opts ...ipc.RequestOption) (resp []byte, err error) {
	return s.Post(url, "application/x-www-form-urlencoded", strings.NewReader(data.Encode()), opts...)
}

func (s *udsClient) do(req *http.Request, contentType string, onChunk func([]byte), opts ...ipc.RequestOption) (resp []byte, err error) {
	// Apply all options to the request
	params := ipc.RequestParams{
		Request: req,
		Timeout: s.innerClient.Timeout,
	}
	for _, opt := range opts {
		opt(&params)
	}

	// Some options replace the request pointer, so we need to make a shallow copy
	req = params.Request

	// Create a shallow copy of the client to avoid modifying the original client's timeout.
	// This is efficient since http.Client is lightweight and only the Transport field (which is the heavy part)
	// is shared between copies. This approach enables per-request timeout customization.
	client := s.innerClient
	client.Timeout = params.Timeout

	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Authorization", "Bearer "+s.authToken)

	r, err := client.Do(req)

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
		return body, fmt.Errorf("status code: %d, body: %s", r.StatusCode, string(body))
	}
	return body, nil
}

// WithCloseConnection is a request option that closes the connection after the request
func WithCloseConnection(req *ipc.RequestParams) {
	req.Close = true
}

// WithLeaveConnectionOpen is a request option that leaves the connection open after the request
func WithLeaveConnectionOpen(req *ipc.RequestParams) {
	req.Close = false
}

// WithContext is a request option that sets the context for the request
func WithContext(ctx context.Context) ipc.RequestOption {
	return func(params *ipc.RequestParams) {
		params.Request = params.Request.WithContext(ctx)
	}
}

// WithTimeout is a request option that sets the timeout for the request
func WithTimeout(timeout time.Duration) ipc.RequestOption {
	return func(params *ipc.RequestParams) {
		params.Timeout = timeout
	}
}

// WithValues is a request option that sets the values for the request
func WithValues(values url.Values) ipc.RequestOption {
	return func(params *ipc.RequestParams) {
		params.Request.URL.RawQuery = values.Encode()
	}
}

// NewIPCEndpoint constructs a new IPC Endpoint using the given config, path, and options
func (s *ipcClient) NewIPCEndpoint(endpointPath string) (ipc.Endpoint, error) {
	return nil, fmt.Error("the IPCEndpoint interface is not currently supported with the UDS client")
}
