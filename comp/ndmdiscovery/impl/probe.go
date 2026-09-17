// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package ndmdiscoveryimpl

import (
	"context"
	"encoding/json"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/ndm/credentials"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/metadata"
	"github.com/DataDog/datadog-agent/pkg/networkdevices/connectivity"
)

// probe is one protocol a range can be scanned with. It is registered once, at startup.
type probe interface {
	kind() string
	detect(ctx context.Context)
	available() bool
	parse(raw json.RawMessage) (probeConfig, error)
}

// probeConfig is one probe's validated configuration for one range.
type probeConfig interface {
	kind() string
	prepare() (probeRun, error)
}

// probeRun is one probe's contribution to one cycle.
type probeRun interface {
	fingerprint() string
	apply(req *connectivity.Request)
	read(d connectivity.DeviceResult) *probeReading
}

// probeReading is what one probe learned about one address.
type probeReading struct {
	Result metadata.ProbeResult
	Name   string
}

// credentialStore reads the credentials of one kind from the Agent configuration.
type credentialStore interface {
	Load() (map[string]credentials.Credential, error)
}

// probeSet is the registered probes, ordered as the connectivity engine
// receives their checks.
type probeSet struct {
	probes []probe
	log    log.Component
}

// newProbeSet registers the probes in the order their checks are sent.
func newProbeSet(logger log.Component, probes ...probe) *probeSet {
	return &probeSet{probes: probes, log: logger}
}

// detect resolves the availability of every probe. It runs once, at startup,
// before any range is parsed.
func (s *probeSet) detect(ctx context.Context) {
	for _, p := range s.probes {
		p.detect(ctx)
		if !p.available() {
			s.log.Warnf("ndmdiscovery: the %s probe is not available on this agent", p.kind())
		}
	}
}

// parse returns the configuration of each usable probe of one range, in
// registry order. An unknown, unavailable, or rejecting probe is dropped and logged.
func (s *probeSet) parse(rangeID string, probes map[string]json.RawMessage) []probeConfig {
	configs := make([]probeConfig, 0, len(probes))
	known := make(map[string]struct{}, len(s.probes))

	for _, p := range s.probes {
		known[p.kind()] = struct{}{}
		raw, asked := probes[p.kind()]
		if !asked {
			continue
		}
		if !p.available() {
			s.log.Warnf("ndmdiscovery: range %s: dropping the %s probe, it is not available on this agent", rangeID, p.kind())
			continue
		}
		cfg, err := p.parse(raw)
		if err != nil {
			s.log.Warnf("ndmdiscovery: range %s: dropping the %s probe: %v", rangeID, p.kind(), err)
			continue
		}
		configs = append(configs, cfg)
	}

	for kind := range probes {
		if _, ok := known[kind]; !ok {
			s.log.Warnf("ndmdiscovery: range %s: dropping the unknown probe %q", rangeID, kind)
		}
	}
	return configs
}

func statusString(success bool) string {
	if success {
		return statusReachable
	}
	return statusUnreachable
}
