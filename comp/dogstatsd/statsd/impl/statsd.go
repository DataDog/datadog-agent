// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package statsdimpl implements the statsd component.
package statsdimpl

import (
	"net"
	"os"
	"strconv"
	"sync"
	"time"

	ddgostatsd "github.com/DataDog/datadog-go/v5/statsd"

	statsd "github.com/DataDog/datadog-agent/comp/dogstatsd/statsd/def"
)

// Requires defines the dependencies for the statsd component.
type Requires struct{}

// Provides defines the output of the statsd component.
type Provides struct {
	Comp statsd.Component
}

// NewComponent creates a new statsd component.
func NewComponent(_ Requires) (Provides, error) {
	return Provides{
		Comp: &service{},
	}, nil
}

type service struct {
	sync.Mutex
	// The default shared client.
	client ddgostatsd.ClientInterface
}

// Get returns a pre-configured and shared statsd client (requires STATSD_URL env var to be set)
func (hs *service) Get() (ddgostatsd.ClientInterface, error) {
	hs.Lock()
	defer hs.Unlock()

	if hs.client == nil {
		var err error
		hs.client, err = hs.Create()
		if err != nil {
			return nil, err
		}
	}
	return hs.client, nil
}

// Create returns a pre-configured statsd client
func (hs *service) Create(options ...ddgostatsd.Option) (ddgostatsd.ClientInterface, error) {
	return createClient("", options...)
}

// CreateForAddr returns a pre-configured statsd client that defaults to addr if no env var is set
func (hs *service) CreateForAddr(addr string, options ...ddgostatsd.Option) (ddgostatsd.ClientInterface, error) {
	return createClient(addr, options...)
}

// CreateForHostPort returns a pre-configured statsd client that defaults to host:port if no env var is set
func (hs *service) CreateForHostPort(host string, port int, options ...ddgostatsd.Option) (ddgostatsd.ClientInterface, error) {
	return createClient(net.JoinHostPort(host, strconv.Itoa(port)), options...)
}

var _ statsd.Component = (*service)(nil)

// createClient returns a pre-configured statsd client that defaults to addr if no env var is set
func createClient(addr string, options ...ddgostatsd.Option) (ddgostatsd.ClientInterface, error) {
	if envAddr, ok := os.LookupEnv("STATSD_URL"); ok {
		addr = envAddr
	}

	if addr == "" {
		addr = net.JoinHostPort("127.0.0.1", strconv.Itoa(8125))
	}

	options = append(
		[]ddgostatsd.Option{
			ddgostatsd.WithTelemetryAddr(addr),
			ddgostatsd.WithChannelMode(),
			ddgostatsd.WithClientSideAggregation(),
			ddgostatsd.WithExtendedClientSideAggregation(),
			ddgostatsd.WithWriteTimeout(500 * time.Millisecond),
			ddgostatsd.WithConnectTimeout(3 * time.Second),
		},
		options...,
	)
	return ddgostatsd.New(addr, options...)
}
