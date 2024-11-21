// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ipc

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
)

type tcpEndpoint struct {
	addr string
}

// NewTCPEndpoint return a new TCP endpoint usable with http.Client and http.Server
func NewTCPEndpoint(addr string) Endpoint {
	return &tcpEndpoint{
		addr: addr,
	}
}

func (t *tcpEndpoint) RoundTripper(tlsConfig *tls.Config) http.RoundTripper {
	return &http.Transport{
		DialContext: t.tcpDialContext(),
		// Proxy:           t.tcpProxyFunx(),
		TLSClientConfig: tlsConfig,
	}
	// return net.Dial("tcp", t.addr)
}

func (t *tcpEndpoint) Listener() (net.Listener, error) {
	return net.Listen("tcp", t.addr)
}

func (t *tcpEndpoint) Addr() string {
	return t.addr
}

// func newDialContext(config config.Reader) DialContext {
func (t *tcpEndpoint) tcpDialContext() func(ctx context.Context, network string, addr string) (net.Conn, error) {
	return func(_ context.Context, _ string, _ string) (net.Conn, error) {
		return net.Dial("tcp", t.addr)
	}
}

// // func newDialContext(config config.Reader) DialContext {
// func (t *tcpEndpoint) tcpProxyFunx() func(req *http.Request) (*url.URL, error) {
// 	return func(req *http.Request) (*url.URL, error) {

// 		log.Printf("url: %+v\n", req.URL)
// 		proxified := req.URL
// 		proxified.Host = t.addr

// 		return proxified, nil
// 	}
// }
