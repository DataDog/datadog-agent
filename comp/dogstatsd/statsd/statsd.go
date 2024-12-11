// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package statsd

import (
	"net"
	"os"
	"strconv"
	"sync"
	"time"

	"go.uber.org/fx"

	ddgostatsd "github.com/DataDog/datadog-go/v5/statsd"

	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newStatsdService))
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

// CreateForAddr returns a pre-configured statsd client that defaults to `addr` if no env var is set
func (hs *service) CreateForAddr(addr string, options ...ddgostatsd.Option) (ddgostatsd.ClientInterface, error) {
	return createClient(addr, options...)
}

// CreateForHostPort returns a pre-configured statsd client that defaults to `host:port` if no env var is set
func (hs *service) CreateForHostPort(host string, port int, options ...ddgostatsd.Option) (ddgostatsd.ClientInterface, error) {
	return createClient(net.JoinHostPort(host, strconv.Itoa(port)), options...)
}

var _ Component = (*service)(nil)

// createClient returns a pre-configured statsd client that defaults to `addr` if no env var is set
// It is exported for callers that might not support components.
func createClient(addr string, options ...ddgostatsd.Option) (ddgostatsd.ClientInterface, error) {
	// We default to STATSD_URL because it's more likely to be what the user wants, the provided
	// address if often a fallback using UDP.

	if envAddr, ok := os.LookupEnv("STATSD_URL"); ok {
		addr = envAddr
	}

	if addr == "" {
		addr = net.JoinHostPort("127.0.0.1", strconv.Itoa(8125))
	}

	options = append(
		[]ddgostatsd.Option{
			// Create a separate client for the telemetry to be sure we don't lose it.
			ddgostatsd.WithTelemetryAddr(addr),
			// Enable recommended settings to reduce the number of packets sent and reduce
			// potential lock contention on the critical path.
			ddgostatsd.WithChannelMode(),
			ddgostatsd.WithClientSideAggregation(),
			ddgostatsd.WithExtendedClientSideAggregation(),
			// Increase timeouts to avoid dropping packets that could have waited a bit more.
			ddgostatsd.WithWriteTimeout(500 * time.Millisecond),
			ddgostatsd.WithConnectTimeout(3 * time.Second),
		},
		options...,
	)
	return ddgostatsd.New(addr, options...)
}

func newStatsdService() Component {
	return &service{}
}
