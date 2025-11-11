// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package servernoop

import (
	"fmt"
	"time"

	"go.uber.org/fx"

	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	configComponent "github.com/DataDog/datadog-agent/comp/core/config"
	// rctypes "github.com/DataDog/datadog-agent/comp/remote-config/rcclient/types"
)

type dependencies struct {
	fx.In

	Lc fx.Lifecycle

	Config configComponent.Component
}

type provides struct {
	fx.Out
	Comp          Component
	StatsEndpoint api.AgentEndpointProvider
	// RCListener    rctypes.ListenerProvider
}

func newServer(deps dependencies) provides {
	s := &server{}
	s.useDogstatsd = deps.Config.GetBool("use_dogstatsd")

	if deps.Config.GetInt("dogstatsd_port") > 0 {
		s.udpLocalAddr = fmt.Sprintf("localhost:%d", deps.Config.GetInt("dogstatsd_port"))
	}

	return provides{
		Comp:          s,
		StatsEndpoint: api.NewAgentEndpointProvider(nil, "/dogstatsd-stats", "GET"),
	}
}

// Server represent a Dogstatsd server
type server struct {
	useDogstatsd bool
	udpLocalAddr string
}

// IsRunning returns true if the server is running
func (s *server) IsRunning() bool {
	return s.useDogstatsd
}

// ServerlessFlush flushes all the data to the aggregator to them send it to the Datadog intake.
func (s *server) ServerlessFlush(_ time.Duration) {
	// no-op
}

// SetExtraTags sets extra tags. All metrics sent to the DogstatsD will be tagged with them.
func (s *server) SetExtraTags(_tags []string) {
	// no-op
}

// UDPLocalAddr returns the local address of the UDP statsd listener, if enabled.
func (s *server) UDPLocalAddr() string {
	return s.udpLocalAddr
}

// SetFilterList sets the filterlist to apply when parsing metrics from the DogStatsD listener.
func (s *server) SetFilterList(_ []string, _ bool) {
	// no-op
}
