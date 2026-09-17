// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package ndmdiscoveryimpl

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/metadata"
	"github.com/DataDog/datadog-agent/pkg/networkdevices/connectivity"
)

// stubProbe is a probe whose kind, availability and failures a test sets.
type stubProbe struct {
	name        string
	check       string
	unavailable bool
	parseErr    error
	prepareErr  error
	detected    int
}

func (p *stubProbe) kind() string           { return p.name }
func (p *stubProbe) detect(context.Context) { p.detected++ }
func (p *stubProbe) available() bool        { return !p.unavailable }

func (p *stubProbe) parse(raw json.RawMessage) (probeConfig, error) {
	if p.parseErr != nil {
		return nil, p.parseErr
	}
	return &stubProbeConfig{name: p.name, check: p.check, raw: string(raw), prepareErr: p.prepareErr}, nil
}

type stubProbeConfig struct {
	name       string
	check      string
	raw        string
	prepareErr error
}

func (c *stubProbeConfig) kind() string { return c.name }

func (c *stubProbeConfig) prepare() (probeRun, error) {
	if c.prepareErr != nil {
		return nil, c.prepareErr
	}
	return &stubProbeRun{name: c.name, check: c.check, raw: c.raw}, nil
}

// stubProbeRun reads the connectivity field its check owns, so the scripted
// fakeChecker answers of the sweeper tests need no change.
type stubProbeRun struct {
	name  string
	check string
	raw   string
}

func (r *stubProbeRun) fingerprint() string { return r.name + ":" + r.raw }

func (r *stubProbeRun) apply(req *connectivity.Request) {
	req.Checks = append(req.Checks, r.check)
}

func (r *stubProbeRun) read(d connectivity.DeviceResult) *probeReading {
	var (
		res     *connectivity.CheckResult
		credID  string
		sysName string
	)
	switch r.check {
	case connectivity.CheckPing:
		if d.PingResult == nil {
			return nil
		}
		res = &d.PingResult.CheckResult
	case connectivity.CheckSNMP:
		if d.SNMPResult == nil {
			return nil
		}
		res = &d.SNMPResult.CheckResult
		credID, sysName = d.SNMPResult.CredID, d.SNMPResult.SysName
	default:
		return nil
	}

	reading := &probeReading{Result: metadata.ProbeResult{
		Kind:   r.name,
		Status: statusString(res.Success),
		RttMs:  res.RttMs,
	}}
	if res.Success {
		reading.Result.CredID = credID
		reading.Name = sysName
	}
	return reading
}

func kinds(configs []probeConfig) []string {
	got := make([]string, 0, len(configs))
	for _, c := range configs {
		got = append(got, c.kind())
	}
	return got
}

func TestProbeSetParseKeepsRegistryOrder(t *testing.T) {
	set := newProbeSet(logmock.New(t), &stubProbe{name: "ping"}, &stubProbe{name: "snmp"})

	configs := set.parse("ad-1", map[string]json.RawMessage{
		"snmp": json.RawMessage(`{"port":161}`),
		"ping": json.RawMessage(`{"count":2}`),
	})

	assert.Equal(t, []string{"ping", "snmp"}, kinds(configs))
}

func TestProbeSetParseSkipsAKindTheRangeDoesNotAskFor(t *testing.T) {
	set := newProbeSet(logmock.New(t), &stubProbe{name: "ping"}, &stubProbe{name: "snmp"})

	configs := set.parse("ad-1", map[string]json.RawMessage{"snmp": json.RawMessage(`{}`)})

	assert.Equal(t, []string{"snmp"}, kinds(configs))
}

func TestProbeSetParseDropsAnUnknownKind(t *testing.T) {
	set := newProbeSet(logmock.New(t), &stubProbe{name: "snmp"})

	configs := set.parse("ad-1", map[string]json.RawMessage{
		"snmp": json.RawMessage(`{}`),
		"ssh":  json.RawMessage(`{}`),
	})

	assert.Equal(t, []string{"snmp"}, kinds(configs))
}

func TestProbeSetParseDropsAnUnavailableProbe(t *testing.T) {
	set := newProbeSet(logmock.New(t),
		&stubProbe{name: "ping", unavailable: true},
		&stubProbe{name: "snmp"},
	)

	configs := set.parse("ad-1", map[string]json.RawMessage{
		"ping": json.RawMessage(`{}`),
		"snmp": json.RawMessage(`{}`),
	})

	assert.Equal(t, []string{"snmp"}, kinds(configs))
}

func TestProbeSetParseDropsAProbeThatRejectsItsOptions(t *testing.T) {
	set := newProbeSet(logmock.New(t),
		&stubProbe{name: "ping", parseErr: errors.New("bad options")},
		&stubProbe{name: "snmp"},
	)

	configs := set.parse("ad-1", map[string]json.RawMessage{
		"ping": json.RawMessage(`{}`),
		"snmp": json.RawMessage(`{}`),
	})

	assert.Equal(t, []string{"snmp"}, kinds(configs))
}

func TestProbeSetParseReturnsNothingWhenNoProbeIsUsable(t *testing.T) {
	set := newProbeSet(logmock.New(t), &stubProbe{name: "snmp", unavailable: true})

	configs := set.parse("ad-1", map[string]json.RawMessage{"snmp": json.RawMessage(`{}`)})

	assert.Empty(t, configs)
}

func TestProbeSetDetectRunsEveryProbeOnce(t *testing.T) {
	ping := &stubProbe{name: "ping"}
	snmp := &stubProbe{name: "snmp"}
	set := newProbeSet(logmock.New(t), ping, snmp)

	set.detect(context.Background())

	assert.Equal(t, 1, ping.detected)
	assert.Equal(t, 1, snmp.detected)
}

func TestProbeSetParsePassesTheOptionsThroughUntouched(t *testing.T) {
	set := newProbeSet(logmock.New(t), &stubProbe{name: "snmp"})

	configs := set.parse("ad-1", map[string]json.RawMessage{"snmp": json.RawMessage(`{"port":1161}`)})

	require.Len(t, configs, 1)
	cfg, ok := configs[0].(*stubProbeConfig)
	require.True(t, ok)
	assert.JSONEq(t, `{"port":1161}`, cfg.raw)
}
