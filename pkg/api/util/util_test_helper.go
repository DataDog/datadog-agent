// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build test

package util

import (
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"
)

// SetAuthTokenInMemory is only expected to be used for unit-tests
// It sets the auth token, client TLS config and server TLS config in memory
// and initializes the initSource to setAuthTokenInMemory
func SetAuthTokenInMemory(t testing.TB) {
	tokenLock.Lock()
	defer tokenLock.Unlock()

	if initSource != uninitialized {
		if initSource != setAuthTokenInMemory {
			t.Fatal("the auth stack have been initialized by un underlying part of the code")
		}
		t.Log("the auth stack have been initialized in a previous call to SetAuthTokenInMemory, no need to generate new values")
		return
	}

	t.Log("generating random token, clientTLSConfig and serverTLSConfig for test")

	key := make([]byte, 32)
	_, err := rand.Read(key)
	if err != nil {
		t.Fatalf("can't create agent auth token value: %v", err)
	}

	// convert the raw token to an hex string
	token = hex.EncodeToString(key)

	// Starting a TLS httptest server to retrieve a localhost selfsigned tlsCert
	ts := httptest.NewTLSServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
	serverTLSConfig = ts.TLS.Clone()
	certpool := x509.NewCertPool()
	certpool.AddCert(ts.Certificate())
	ts.Close()

	clientTLSConfig = &tls.Config{
		RootCAs: certpool,
	}

	initSource = setAuthTokenInMemory
}
