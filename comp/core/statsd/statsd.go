// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package statsd

import (
	"net"
	"os"
	"strconv"

	"go.uber.org/fx"

	ddgostatsd "github.com/DataDog/datadog-go/v5/statsd"

	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for this component.
var Module = fxutil.Component(
	fx.Provide(newStatsdService),
)

type service struct{}

// Get returns a pre-configured statsd client
func (hs *service) Get(options ...ddgostatsd.Option) (ddgostatsd.ClientInterface, error) {
	return GetClient("", options...)
}

// GetForAddr returns a pre-configured statsd client that defaults to `addr` if no env var is set
func (hs *service) GetForAddr(addr string, options ...ddgostatsd.Option) (ddgostatsd.ClientInterface, error) {
	return GetClient(addr, options...)
}

// GetForHostPort returns a pre-configured statsd client that defaults to `host:port` if no env var is set
func (hs *service) GetForHostPort(host string, port int, options ...ddgostatsd.Option) (ddgostatsd.ClientInterface, error) {
	return GetClient(net.JoinHostPort(host, strconv.Itoa(port)), options...)
}

var _ Component = (*service)(nil)

// GetClient returns a pre-configured statsd client that defaults to `addr` if no env var is set
// It is exported for callers that might not support components.
func GetClient(addr string, options ...ddgostatsd.Option) (ddgostatsd.ClientInterface, error) {
	// We default to STATSD_URL because it's more likely to be what the user wants, the provided
	// address if often a fallback using UDP.

	if envAddr, ok := os.LookupEnv("STATSD_URL"); ok {
		addr = envAddr
	}

	if addr == "" {
		addr = net.JoinHostPort("locahost", strconv.Itoa(8125))
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
		},
		options...,
	)
	return ddgostatsd.New(addr, options...)
}

func newStatsdService() Component {
	return &service{}
}
