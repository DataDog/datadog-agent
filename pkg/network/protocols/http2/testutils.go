// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && test

package http2

import (
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
)

// StartH2CServer starts a new HTTP/2 server with the given address and returns a function to stop it.
func StartH2CServer(t *testing.T, address string, isTLS bool) func() {
	srv := &http.Server{
		Addr: address,
		Handler: h2c.NewHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			statusCode := testutil.StatusFromPath(r.URL.Path)
			if statusCode == 0 {
				w.WriteHeader(http.StatusOK)
			} else {
				w.WriteHeader(int(statusCode))
			}
			defer func() { _ = r.Body.Close() }()
			_, _ = io.Copy(w, r.Body)
		}), &http2.Server{}),
		IdleTimeout: 2 * time.Second,
	}

	require.NoError(t, http2.ConfigureServer(srv, nil), "could not configure server")

	l, err := net.Listen("tcp", address)
	require.NoError(t, err, "could not listen")

	if isTLS {
		cert, key, err := testutil.GetCertsPaths()
		require.NoError(t, err, "could not get certs paths")
		go func() {
			if err := srv.ServeTLS(l, cert, key); err != http.ErrServerClosed {
				require.NoError(t, err, "could not serve TLS")
			}
		}()
	} else {
		go func() {
			if err := srv.Serve(l); err != http.ErrServerClosed {
				require.NoError(t, err, "could not serve")
			}
		}()
	}

	return func() { _ = srv.Shutdown(context.Background()) }
}
