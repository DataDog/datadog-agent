// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package defaultforwarder

import (
	"net/http"
	"sync"
)

// SharedConnection holds a shared http.Client that is used by each worker.
// Access to the client is protected by an RWMutex.
type SharedConnection struct {
	client *http.Client
	lock   *sync.RWMutex
}

// NewSharedConnection creates a new shared connection with the given
// http.Client.
func NewSharedConnection(client *http.Client) *SharedConnection {
	return &SharedConnection{
		lock:   &sync.RWMutex{},
		client: client,
	}
}

// GetClient returns the http.Client.
func (sc *SharedConnection) GetClient() *http.Client {
	sc.lock.RLock()
	defer sc.lock.RUnlock()

	return sc.client
}

// SetClient replaces the client with the given one.
func (sc *SharedConnection) SetClient(client *http.Client) {
	sc.lock.Lock()
	defer sc.lock.Unlock()

	sc.client = client
}
