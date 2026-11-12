// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package ndmconnectivitycheckimpl

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

func TestResolveRawSocketHonorsTheRequestOnLinux(t *testing.T) {
	useRawSocket := true

	assert.True(t, resolveRawSocket("linux", false, &useRawSocket))
}

func TestResolveRawSocketKeepsTheDetectedTypeWhereThePlatformForcesIt(t *testing.T) {
	udp := false
	raw := true

	assert.True(t, resolveRawSocket("windows", true, &udp), "windows has no UDP pinger")
	assert.False(t, resolveRawSocket("darwin", false, &raw), "macOS has no raw pinger")
}

func TestResolveRawSocketKeepsTheDetectedTypeWithoutAnOverride(t *testing.T) {
	assert.True(t, resolveRawSocket("linux", true, nil))
	assert.False(t, resolveRawSocket("linux", false, nil))
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

func TestToConnectivityResultRoundsTheRoundTripToTheNearestMillisecond(t *testing.T) {
	results := []probe.Result{{
		Target: "10.0.0.1",
		Ping:   &pingprobe.Reading{Success: true, RTT: 600 * time.Microsecond},
		SNMP:   &snmpprobe.Reading{Success: true, RTT: 1540 * time.Microsecond},
	}}

	res := toConnectivityResult(results)

	require.Len(t, res.Devices, 1)
	d := res.Devices[0]
	require.NotNil(t, d.PingResult.RttMs)
	assert.Equal(t, int64(1), *d.PingResult.RttMs)
	require.NotNil(t, d.SNMPResult.RttMs)
	assert.Equal(t, int64(2), *d.SNMPResult.RttMs)
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

func groupedRequest() connectivity.Request {
	return connectivity.Request{
		Targets:     []string{"10.0.0.1", "10.0.0.2", "10.0.0.3"},
		Checks:      []string{connectivity.CheckSNMP},
		SNMPOptions: &connectivity.SNMPOptions{Port: 161},
		Credentials: []connectivity.SNMPCredential{
			{ID: "cred-a", Version: "2c", Community: "a"},
			{ID: "cred-b", Version: "2c", Community: "b"},
			{ID: "cred-c", Version: "2c", Community: "c"},
		},
		TargetGroups: []connectivity.TargetGroup{
			{ID: "group-1", TargetIPs: []string{"10.0.0.1", "10.0.0.2"}, CredentialIDs: []string{"cred-c", "cred-a"}},
			{ID: "group-2", TargetIPs: []string{"10.0.0.3"}, CredentialIDs: []string{"cred-b"}},
		},
	}
}

func credentialIDs(opts probe.Options) []string {
	if opts.SNMP == nil {
		return nil
	}
	ids := make([]string, 0, len(opts.SNMP.Credentials))
	for _, c := range opts.SNMP.Credentials {
		ids = append(ids, c.ID)
	}
	return ids
}

func TestToTargetPlansTriesEachTargetOnlyWithItsGroupsCredentials(t *testing.T) {
	plans, err := toTargetPlans(groupedRequest(), pingprobe.Capability{Available: true})

	require.NoError(t, err)
	require.Len(t, plans, 3)
	assert.Equal(t, "group-1", plans["10.0.0.1"].groupID)
	assert.Equal(t, []string{"cred-c", "cred-a"}, credentialIDs(plans["10.0.0.1"].opts), "in the group's order")
	assert.Equal(t, "group-1", plans["10.0.0.2"].groupID)
	assert.Equal(t, []string{"cred-c", "cred-a"}, credentialIDs(plans["10.0.0.2"].opts))
	assert.Equal(t, "group-2", plans["10.0.0.3"].groupID)
	assert.Equal(t, []string{"cred-b"}, credentialIDs(plans["10.0.0.3"].opts))
	assert.Equal(t, snmpprobe.Credential{ID: "cred-b", Version: "2c", Community: "b"}, plans["10.0.0.3"].opts.SNMP.Credentials[0])
}

func TestToTargetPlansRunsNoSNMPForAGroupWithNoCredential(t *testing.T) {
	req := groupedRequest()
	req.Checks = []string{connectivity.CheckPing, connectivity.CheckSNMP}
	req.PingOptions = &connectivity.PingOptions{Count: 1}
	req.TargetGroups[1].CredentialIDs = nil

	plans, err := toTargetPlans(req, pingprobe.Capability{Available: true})

	require.NoError(t, err)
	assert.NotNil(t, plans["10.0.0.3"].opts.Ping)
	assert.Nil(t, plans["10.0.0.3"].opts.SNMP)
	assert.NotNil(t, plans["10.0.0.1"].opts.SNMP)
}

func TestToTargetPlansRejectsWhatTheEndpointAnswersWith400(t *testing.T) {
	tests := map[string]func(*connectivity.Request){
		"a target in two groups": func(r *connectivity.Request) {
			r.TargetGroups[1].TargetIPs = append(r.TargetGroups[1].TargetIPs, "10.0.0.1")
		},
		"a target twice in one group": func(r *connectivity.Request) {
			r.TargetGroups[1].TargetIPs = append(r.TargetGroups[1].TargetIPs, "10.0.0.3")
		},
		"an unknown credential id": func(r *connectivity.Request) {
			r.TargetGroups[0].CredentialIDs = append(r.TargetGroups[0].CredentialIDs, "cred-z")
		},
		"a credential named twice in one group": func(r *connectivity.Request) {
			r.TargetGroups[1].CredentialIDs = append(r.TargetGroups[1].CredentialIDs, "cred-b")
		},
		"a credential id that is not unique": func(r *connectivity.Request) {
			r.Credentials = append(r.Credentials, connectivity.SNMPCredential{ID: "cred-a", Version: "2c", Community: "other"})
		},
		"a target in no group": func(r *connectivity.Request) {
			r.Targets = append(r.Targets, "10.0.0.4")
		},
		"a group target missing from the targets": func(r *connectivity.Request) {
			r.Targets = r.Targets[:2]
		},
		"no group for the targets": func(r *connectivity.Request) {
			r.TargetGroups = []connectivity.TargetGroup{}
		},
		"a group with no id": func(r *connectivity.Request) {
			r.TargetGroups[1].ID = ""
		},
		"a group id that is not unique": func(r *connectivity.Request) {
			r.TargetGroups[1].ID = "group-1"
		},
	}

	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			req := groupedRequest()
			mutate(&req)

			_, err := toTargetPlans(req, pingprobe.Capability{Available: true})

			require.Error(t, err)
			assert.ErrorIs(t, err, connectivity.ErrInvalidRequest)
		})
	}
}
