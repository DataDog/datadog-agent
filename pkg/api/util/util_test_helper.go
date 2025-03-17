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
	"fmt"
	"net/http"
	"net/http/httptest"
)

// SetAuthTokenInMemory is only expected to be used for unit-tests
// It sets the auth token, client TLS config and server TLS config in memory
// and initializes the initSource to setAuthTokenInMemory
func SetAuthTokenInMemory() {
	if initSource != uninitialized {
		if initSource != setAuthTokenInMemory {
			panic("the auth stack have been initialized by un underlying part of the code")
		}
		fmt.Printf("the auth stack have been initialized before calling SetAuthTokenInMemory")
		return
	}

	fmt.Printf("set custom values for token, clientConfig and serverConfig")
	tokenLock.Lock()
	defer tokenLock.Unlock()

	key := make([]byte, 32)
	_, err := rand.Read(key)
	if err != nil {
		panic(fmt.Sprintf("can't create agent authentication token value: %v", err.Error()))
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
	initSource = setAuthTokenInMemory
}
