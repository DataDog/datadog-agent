// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build test

package util

import (
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"
)

// SetAuthTokenInMemory is only expected to be used for unit-tests
// It sets the auth token, client TLS config and server TLS config in memory
// and initializes the initSource to setAuthTokenInMemory
func SetAuthTokenInMemory(t *testing.T) {
	tokenLock.Lock()
	defer tokenLock.Unlock()

	if initSource != uninitialized {
		if initSource != setAuthTokenInMemory {
			t.Fatal("the auth stack have been initialized by un underlying part of the code")
		}
		t.Log("the auth stack have been initialized before calling SetAuthTokenInMemory")
	}

	t.Log("set custom values for token, clientConfig and serverConfig")

	key := make([]byte, 32)
	_, err := rand.Read(key)
	if err != nil {
		t.Fatalf("can't create agent authentication token value: %v", err.Error())
	}

	// convert the raw token to an hex string
	token = hex.EncodeToString(key)

	// Starting a TLS httptest server to retrieve a localhost selfsigned tlsCert
	ts := httptest.NewTLSServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
	serverTLSConfig = ts.TLS.Clone()
	ts.Close()

	clientTLSConfig = &tls.Config{
		InsecureSkipVerify: true,
	}

	// Cleanup the auth token, client TLS config and server TLS config in memory
	// when the test is done
	// This avoid difference in behavior between running one test or an entire test suite in some cases
	t.Cleanup(CleanupAuthTokenInMemory)

	initSource = setAuthTokenInMemory
}

// CleanupAuthTokenInMemory is only expected to be used for unit-tests
// It cleans up the auth token, client TLS config and server TLS config in memory
// and initializes the initSource to uninitialized
func CleanupAuthTokenInMemory() {
	tokenLock.Lock()
	defer tokenLock.Unlock()
	initSource = uninitialized
	token = ""
	clientTLSConfig = nil
	serverTLSConfig = nil
}
