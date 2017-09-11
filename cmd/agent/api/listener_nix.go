// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build !windows

package api

import (
	"net"
	"net/http"
	// "github.com/DataDog/datadog-agent/pkg/config"
)

// getListener returns a listening connection to a Unix socket
// on non-windows platforms.
func getListener() (net.Listener, error) {
	return net.Listen("tcp", ":5555")
}

// HTTP doesn't need anything from TCP so we can use a Unix socket to dial
func fakeDial(proto, addr string) (conn net.Conn, err error) {
	return net.Dial("tcp", "localhost:5555")
}

// GetClient is a convenience function returning an http
// client suitable to use a unix socket transport
func GetClient() *http.Client {
	tr := &http.Transport{
		Dial: fakeDial,
	}
	return &http.Client{Transport: tr}
}
