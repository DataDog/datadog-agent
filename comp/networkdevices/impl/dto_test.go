// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package networkdevicesimpl

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/networkdevice/probe"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/probe/failure"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/probe/pingprobe"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/probe/snmpprobe"
	"github.com/DataDog/datadog-agent/pkg/networkdevices/connectivity"
)

func TestTheFailureReasonsMatchTheWireContract(t *testing.T) {
	assert.Equal(t, connectivity.FailureNone, failure.None)
	assert.Equal(t, connectivity.FailureUnreachable, failure.Unreachable)
	assert.Equal(t, connectivity.FailureTimeout, failure.Timeout)
	assert.Equal(t, connectivity.FailureConnectionRefused, failure.ConnectionRefused)
	assert.Equal(t, connectivity.FailureHostUnreachable, failure.HostUnreachable)
	assert.Equal(t, connectivity.FailureNetworkUnreachable, failure.NetworkUnreachable)
	assert.Equal(t, connectivity.FailureAuthenticationFailed, failure.AuthenticationFailed)
	assert.Equal(t, connectivity.FailureDecryptionFailed, failure.DecryptionFailed)
	assert.Equal(t, connectivity.FailureUnknownUser, failure.UnknownUser)
	assert.Equal(t, connectivity.FailureUnsupportedSecurityLevel, failure.UnsupportedSecurityLevel)
	assert.Equal(t, connectivity.FailureUnknownEngineID, failure.UnknownEngineID)
	assert.Equal(t, connectivity.FailureUnknown, failure.Unknown)
}

func TestToProbeOptionsMapsPing(t *testing.T) {
	req := connectivity.Request{
		Targets:     []string{"10.0.0.1"},
		Checks:      []string{connectivity.CheckPing},
		PingOptions: &connectivity.PingOptions{Count: 3, IntervalMs: 200, TimeoutMs: 1500},
	}

	opts, err := toProbeOptions(req, pingprobe.Capability{Available: true, UseRawSocket: true})

	require.NoError(t, err)
	require.NotNil(t, opts.Ping)
	assert.Nil(t, opts.SNMP)
	assert.Equal(t, 3, opts.Ping.Count)
	assert.Equal(t, 200*time.Millisecond, opts.Ping.Interval)
	assert.Equal(t, 1500*time.Millisecond, opts.Ping.Timeout)
	assert.True(t, opts.Ping.UseRawSocket)
}

func TestToProbeOptionsLetsTheRequestOverrideTheSocketType(t *testing.T) {
	useRawSocket := false
	req := connectivity.Request{
		Checks:      []string{connectivity.CheckPing},
		PingOptions: &connectivity.PingOptions{Count: 1, UseRawSocket: &useRawSocket},
	}

	opts, err := toProbeOptions(req, pingprobe.Capability{Available: true, UseRawSocket: true})

	require.NoError(t, err)
	require.NotNil(t, opts.Ping)
	assert.False(t, opts.Ping.UseRawSocket)
}

func TestToProbeOptionsMapsSNMP(t *testing.T) {
	req := connectivity.Request{
		Checks:      []string{connectivity.CheckSNMP},
		SNMPOptions: &connectivity.SNMPOptions{Port: 161, TimeoutMs: 2000, Retries: 2},
		Credentials: []connectivity.SNMPCredential{{ID: "cred-1", Version: "2c", Community: "public"}},
	}

	opts, err := toProbeOptions(req, pingprobe.Capability{Available: true})

	require.NoError(t, err)
	assert.Nil(t, opts.Ping)
	require.NotNil(t, opts.SNMP)
	assert.Equal(t, uint16(161), opts.SNMP.Port)
	assert.Equal(t, 2*time.Second, opts.SNMP.Timeout)
	assert.Equal(t, 2, opts.SNMP.Retries)
	require.Len(t, opts.SNMP.Credentials, 1)
	assert.Equal(t, snmpprobe.Credential{ID: "cred-1", Version: "2c", Community: "public"}, opts.SNMP.Credentials[0])
}

func TestToProbeOptionsSkipsSNMPWithNoCredential(t *testing.T) {
	req := connectivity.Request{
		Checks:      []string{connectivity.CheckSNMP},
		SNMPOptions: &connectivity.SNMPOptions{Port: 161},
	}

	opts, err := toProbeOptions(req, pingprobe.Capability{Available: true})

	require.NoError(t, err)
	assert.Nil(t, opts.SNMP)
	assert.True(t, opts.Empty())
}

func TestToProbeOptionsRejectsWhatTheEndpointAnswersWith400(t *testing.T) {
	tests := map[string]connectivity.Request{
		"unsupported check":    {Checks: []string{"telnet"}},
		"ping with no options": {Checks: []string{connectivity.CheckPing}},
		"snmp with no options": {Checks: []string{connectivity.CheckSNMP}},
		"snmp port too high": {
			Checks:      []string{connectivity.CheckSNMP},
			SNMPOptions: &connectivity.SNMPOptions{Port: 65536},
		},
		"snmp port zero": {
			Checks:      []string{connectivity.CheckSNMP},
			SNMPOptions: &connectivity.SNMPOptions{Port: 0},
		},
		"snmp with an unknown version": {
			Checks:      []string{connectivity.CheckSNMP},
			SNMPOptions: &connectivity.SNMPOptions{Port: 161},
			Credentials: []connectivity.SNMPCredential{{ID: "cred-1", Version: "4"}},
		},
	}

	for name, req := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := toProbeOptions(req, pingprobe.Capability{Available: true})

			require.Error(t, err)
			assert.ErrorIs(t, err, connectivity.ErrInvalidRequest)
		})
	}
}

func TestToConnectivityResultMapsASuccessfulReading(t *testing.T) {
	results := []probe.Result{{
		Target: "10.0.0.1",
		Ping:   &pingprobe.Reading{Success: true, RTT: 12 * time.Millisecond},
		SNMP:   &snmpprobe.Reading{Success: true, RTT: 34 * time.Millisecond, CredID: "cred-1", SysName: "switch-1"},
	}}

	res := toConnectivityResult(results)

	require.Len(t, res.Devices, 1)
	d := res.Devices[0]
	assert.Equal(t, "10.0.0.1", d.IPAddress)
	require.NotNil(t, d.PingResult)
	assert.True(t, d.PingResult.Success)
	require.NotNil(t, d.PingResult.RttMs)
	assert.Equal(t, int64(12), *d.PingResult.RttMs)
	require.NotNil(t, d.SNMPResult)
	assert.Equal(t, "cred-1", d.SNMPResult.CredID)
	assert.Equal(t, "switch-1", d.SNMPResult.SysName)
	require.NotNil(t, d.SNMPResult.RttMs)
	assert.Equal(t, int64(34), *d.SNMPResult.RttMs)
}

func TestToConnectivityResultMapsAFailedReading(t *testing.T) {
	results := []probe.Result{{
		Target: "10.0.0.2",
		Ping:   &pingprobe.Reading{FailureReason: failure.Unreachable, Error: "Failed to connect to host '10.0.0.2'"},
	}}

	res := toConnectivityResult(results)

	require.Len(t, res.Devices, 1)
	d := res.Devices[0]
	require.NotNil(t, d.PingResult)
	assert.False(t, d.PingResult.Success)
	assert.Nil(t, d.PingResult.RttMs)
	assert.Equal(t, connectivity.FailureUnreachable, d.PingResult.FailureReason)
	assert.Equal(t, "Failed to connect to host '10.0.0.2'", d.PingResult.Error)
	assert.Nil(t, d.SNMPResult)
}

func TestToConnectivityResultKeepsATargetWithNoReading(t *testing.T) {
	res := toConnectivityResult([]probe.Result{{Target: "10.0.0.3"}})

	require.Len(t, res.Devices, 1)
	assert.Equal(t, "10.0.0.3", res.Devices[0].IPAddress)
	assert.Nil(t, res.Devices[0].PingResult)
	assert.Nil(t, res.Devices[0].SNMPResult)
}
