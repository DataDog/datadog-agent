// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package ndmdiscoveryimpl

import (
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/ndm/credentials"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/probe/pingprobe"
)

type stubCredentialStore struct {
	creds map[string]credentials.Credential
	err   error

	mu    sync.Mutex
	loads int
}

func (s *stubCredentialStore) Load() (map[string]credentials.Credential, error) {
	s.mu.Lock()
	s.loads++
	s.mu.Unlock()
	if s.err != nil {
		return nil, s.err
	}
	return s.creds, nil
}

func (s *stubCredentialStore) loadCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loads
}

func v2cStore() *stubCredentialStore {
	return &stubCredentialStore{creds: map[string]credentials.Credential{
		"cred-1": {ID: "cred-1", SNMPVersion: "2c", CommunityString: "public"},
		"cred-2": {ID: "cred-2", SNMPVersion: "2c", CommunityString: "other"},
	}}
}

func available() pingprobe.Capability {
	return pingprobe.Capability{Available: true}
}

func raw(t *testing.T, kinds map[string]string) map[string]json.RawMessage {
	t.Helper()
	out := map[string]json.RawMessage{}
	for kind, body := range kinds {
		out[kind] = json.RawMessage(body)
	}
	return out
}

func TestParseProbesReadsBothKinds(t *testing.T) {
	params := parseProbes("r1", raw(t, map[string]string{
		"ping": `{"count":3,"interval_ms":200,"timeout_ms":1500}`,
		"snmp": `{"credential_ids":["cred-1"],"port":1161,"timeout_ms":3000,"retries":0}`,
	}), available(), logmock.New(t))

	require.NotNil(t, params.Ping)
	assert.Equal(t, 3, params.Ping.Count)
	assert.Equal(t, 200*time.Millisecond, params.Ping.Interval)
	assert.Equal(t, 1500*time.Millisecond, params.Ping.Timeout)

	require.NotNil(t, params.SNMP)
	assert.Equal(t, uint16(1161), params.SNMP.Port)
	assert.Equal(t, 3*time.Second, params.SNMP.Timeout)
	assert.Equal(t, 0, params.SNMP.Retries)
	assert.Equal(t, []string{"cred-1"}, params.SNMP.CredentialIDs)
}

func TestParseProbesAppliesTheDefaults(t *testing.T) {
	params := parseProbes("r1", raw(t, map[string]string{
		"ping": `{}`,
		"snmp": `{"credential_ids":["cred-1"]}`,
	}), available(), logmock.New(t))

	require.NotNil(t, params.Ping)
	assert.Equal(t, defaultPingCount, params.Ping.Count)
	assert.Equal(t, time.Duration(defaultPingIntervalMs)*time.Millisecond, params.Ping.Interval)
	assert.Equal(t, time.Duration(defaultPingTimeoutMs)*time.Millisecond, params.Ping.Timeout)

	require.NotNil(t, params.SNMP)
	assert.Equal(t, uint16(defaultSNMPPort), params.SNMP.Port)
	assert.Equal(t, time.Duration(defaultSNMPTimeoutMs)*time.Millisecond, params.SNMP.Timeout)
	assert.Equal(t, defaultSNMPRetries, params.SNMP.Retries)
}

func TestParseProbesFallsBackOnAnOutOfBoundsKnob(t *testing.T) {
	params := parseProbes("r1", raw(t, map[string]string{
		"ping": `{"count":9000,"interval_ms":-1,"timeout_ms":600000}`,
		"snmp": `{"credential_ids":["cred-1"],"port":70000,"timeout_ms":600000,"retries":99}`,
	}), available(), logmock.New(t))

	require.NotNil(t, params.Ping)
	assert.Equal(t, defaultPingCount, params.Ping.Count)
	assert.Equal(t, time.Duration(defaultPingIntervalMs)*time.Millisecond, params.Ping.Interval)
	assert.Equal(t, time.Duration(defaultPingTimeoutMs)*time.Millisecond, params.Ping.Timeout)

	require.NotNil(t, params.SNMP)
	assert.Equal(t, uint16(defaultSNMPPort), params.SNMP.Port)
	assert.Equal(t, time.Duration(defaultSNMPTimeoutMs)*time.Millisecond, params.SNMP.Timeout)
	assert.Equal(t, defaultSNMPRetries, params.SNMP.Retries)
}

func TestParseProbesCarriesTheDetectedSocketType(t *testing.T) {
	params := parseProbes("r1", raw(t, map[string]string{"ping": `{}`}),
		pingprobe.Capability{Available: true, UseRawSocket: true}, logmock.New(t))

	require.NotNil(t, params.Ping)
	assert.True(t, params.Ping.UseRawSocket)
}

func TestParseProbesDropsPingWhenItIsNotAvailable(t *testing.T) {
	params := parseProbes("r1", raw(t, map[string]string{
		"ping": `{}`,
		"snmp": `{"credential_ids":["cred-1"]}`,
	}), pingprobe.Capability{Reason: "ping is not supported on plan9"}, logmock.New(t))

	assert.Nil(t, params.Ping)
	assert.NotNil(t, params.SNMP)
}

func TestParseProbesDropsWhatItCannotUse(t *testing.T) {
	tests := map[string]map[string]string{
		"a malformed ping block":      {"ping": `"not-an-object"`},
		"a malformed snmp block":      {"snmp": `[]`},
		"snmp with no credential ids": {"snmp": `{"credential_ids":[]}`},
		"an unknown kind":             {"ssh": `{}`},
	}

	for name, kinds := range tests {
		t.Run(name, func(t *testing.T) {
			params := parseProbes("r1", raw(t, kinds), available(), logmock.New(t))

			assert.Nil(t, params.Ping)
			assert.Nil(t, params.SNMP)
		})
	}
}

func TestResolveBuildsTheProbeOptionsInProbeOrder(t *testing.T) {
	params := parseProbes("r1", raw(t, map[string]string{
		"ping": `{}`,
		"snmp": `{"credential_ids":["cred-2","cred-1"]}`,
	}), available(), logmock.New(t))

	opts, dropped := params.resolve(v2cStore())

	assert.Empty(t, dropped)
	require.NotNil(t, opts.Ping)
	require.NotNil(t, opts.SNMP)
	require.Len(t, opts.SNMP.Credentials, 2)
	assert.Equal(t, "cred-2", opts.SNMP.Credentials[0].ID)
	assert.Equal(t, "cred-1", opts.SNMP.Credentials[1].ID)
	assert.Equal(t, "other", opts.SNMP.Credentials[0].Community)

	fps := opts.Fingerprints()
	require.Len(t, fps, 2)
	assert.Equal(t, opts.Ping.Fingerprint(), fps[0])
}

func TestResolveDropsOnlyTheProbeWhoseCredentialIsMissing(t *testing.T) {
	params := parseProbes("r1", raw(t, map[string]string{
		"ping": `{}`,
		"snmp": `{"credential_ids":["cred-9"]}`,
	}), available(), logmock.New(t))

	opts, dropped := params.resolve(v2cStore())

	require.NotNil(t, opts.Ping)
	assert.Nil(t, opts.SNMP)
	require.Len(t, dropped, 1)
	assert.Contains(t, dropped[0], "cred-9")
}

func TestResolveSurfacesALoadFailureWithoutTheCredentialMaterial(t *testing.T) {
	params := parseProbes("r1", raw(t, map[string]string{
		"snmp": `{"credential_ids":["cred-1"]}`,
	}), available(), logmock.New(t))

	opts, dropped := params.resolve(&stubCredentialStore{err: errors.New("the secret backend is down")})

	assert.True(t, opts.Empty())
	require.Len(t, dropped, 1)
	assert.Contains(t, dropped[0], "the secret backend is down")
	assert.NotContains(t, dropped[0], "public")
}

func TestResolveRejectsAnInvalidCredential(t *testing.T) {
	params := parseProbes("r1", raw(t, map[string]string{
		"snmp": `{"credential_ids":["cred-bad"]}`,
	}), available(), logmock.New(t))

	store := &stubCredentialStore{creds: map[string]credentials.Credential{
		"cred-bad": {ID: "cred-bad", SNMPVersion: "9", CommunityString: "s3cret-community"},
	}}

	opts, dropped := params.resolve(store)

	assert.True(t, opts.Empty())
	require.Len(t, dropped, 1)
	assert.Contains(t, dropped[0], "cred-bad")
	assert.NotContains(t, dropped[0], "s3cret-community")
}

func TestResolveReadsTheStoreOnEveryCall(t *testing.T) {
	params := parseProbes("r1", raw(t, map[string]string{
		"snmp": `{"credential_ids":["cred-1"]}`,
	}), available(), logmock.New(t))
	store := v2cStore()

	params.resolve(store)
	params.resolve(store)

	assert.Equal(t, 2, store.loadCount())
}
