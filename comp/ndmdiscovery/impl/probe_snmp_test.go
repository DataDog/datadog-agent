// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package ndmdiscoveryimpl

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/ndm/credentials"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/metadata"
	"github.com/DataDog/datadog-agent/pkg/networkdevices/connectivity"
)

// stubCredentialStore is a credentialStore backed by a map.
type stubCredentialStore struct {
	creds   map[string]credentials.Credential
	loadErr error

	mu        sync.Mutex
	loadCalls int
}

func (s *stubCredentialStore) Load() (map[string]credentials.Credential, error) {
	s.mu.Lock()
	s.loadCalls++
	s.mu.Unlock()
	return s.creds, s.loadErr
}

func (s *stubCredentialStore) loads() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadCalls
}

func TestResolveCredentials(t *testing.T) {
	store := &stubCredentialStore{creds: map[string]credentials.Credential{
		"cred-a": {ID: "cred-a", SNMPVersion: "2c", CommunityString: "public"},
		"cred-b": {ID: "cred-b", SNMPVersion: "3", User: "datadog", ContextEngineID: "engine"},
	}}

	creds, err := resolveCredentials(store, []string{"cred-a", "cred-b"})
	require.NoError(t, err)
	require.Len(t, creds, 2)
	assert.Equal(t, "cred-a", creds[0].ID)
	assert.Equal(t, "2c", creds[0].Version)
	assert.Equal(t, "public", creds[0].Community)
	assert.Equal(t, "cred-b", creds[1].ID)
	assert.Equal(t, "engine", creds[1].ContextEngineID)
}

func TestResolveCredentialsMissingID(t *testing.T) {
	store := &stubCredentialStore{creds: map[string]credentials.Credential{
		"cred-a": {ID: "cred-a", SNMPVersion: "2c"},
	}}

	_, err := resolveCredentials(store, []string{"cred-a", "nope"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nope")
}

func TestResolveCredentialsRejectsUnknownVersion(t *testing.T) {
	store := &stubCredentialStore{creds: map[string]credentials.Credential{
		"bad": {ID: "bad", SNMPVersion: "4"},
	}}

	_, err := resolveCredentials(store, []string{"bad"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "version")
}

func TestResolveCredentialsRejectsEmptyList(t *testing.T) {
	store := &stubCredentialStore{}
	_, err := resolveCredentials(store, nil)
	require.Error(t, err)
}

func TestResolveCredentialsSurfacesALoadFailure(t *testing.T) {
	store := &stubCredentialStore{loadErr: errors.New("boom")}
	_, err := resolveCredentials(store, []string{"cred-a"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "boom")
}

func TestResolveCredentialsRejectsUnknownV3Protocols(t *testing.T) {
	tests := []struct {
		name    string
		cred    credentials.Credential
		errPart string
	}{
		{
			name:    "bad auth protocol",
			cred:    credentials.Credential{ID: "bad", SNMPVersion: "3", User: "datadog", AuthProtocol: "sha-256", AuthKey: "secret"},
			errPart: "authProtocol",
		},
		{
			name:    "bad priv protocol",
			cred:    credentials.Credential{ID: "bad", SNMPVersion: "3", User: "datadog", PrivProtocol: "aes-256", PrivKey: "secret"},
			errPart: "privProtocol",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &stubCredentialStore{creds: map[string]credentials.Credential{"bad": tt.cred}}
			_, err := resolveCredentials(store, []string{"bad"})
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errPart)
			assert.NotContains(t, err.Error(), "secret", "an error must never carry key material")
		})
	}
}

func TestResolveCredentialsAcceptsEveryValidV3Protocol(t *testing.T) {
	auths := []string{"", "md5", "sha", "sha224", "sha256", "sha384", "sha512", "SHA256"}
	privs := []string{"", "des", "aes", "aes192", "aes256", "aes192c", "aes256c", "AES256C"}

	for _, a := range auths {
		for _, p := range privs {
			store := &stubCredentialStore{creds: map[string]credentials.Credential{
				"cred": {ID: "cred", SNMPVersion: "3", User: "datadog", AuthProtocol: a, PrivProtocol: p},
			}}
			_, err := resolveCredentials(store, []string{"cred"})
			require.NoErrorf(t, err, "auth=%q priv=%q must be accepted", a, p)
		}
	}
}

func TestResolveCredentialsIgnoresProtocolsOnV2c(t *testing.T) {
	store := &stubCredentialStore{creds: map[string]credentials.Credential{
		"cred": {ID: "cred", SNMPVersion: "2c", CommunityString: "public", AuthProtocol: "sha-256", PrivProtocol: "aes-256"},
	}}
	_, err := resolveCredentials(store, []string{"cred"})
	require.NoError(t, err)
}

func newTestSNMPProbe(t *testing.T) (*snmpProbe, *stubCredentialStore) {
	t.Helper()
	store := &stubCredentialStore{creds: map[string]credentials.Credential{
		"cred-a": {ID: "cred-a", SNMPVersion: "2c", CommunityString: "public"},
	}}
	return newSNMPProbe(store, logmock.New(t)), store
}

func snmpRunFor(t *testing.T, raw string) *snmpRun {
	t.Helper()
	p, _ := newTestSNMPProbe(t)
	cfg, err := p.parse(json.RawMessage(raw))
	require.NoError(t, err)
	run, err := cfg.prepare()
	require.NoError(t, err)
	snmp, ok := run.(*snmpRun)
	require.True(t, ok)
	return snmp
}

func TestSNMPProbeKindAndAvailability(t *testing.T) {
	p, _ := newTestSNMPProbe(t)
	p.detect(context.Background())

	assert.Equal(t, "snmp", p.kind())
	assert.True(t, p.available(), "snmp needs no privilege this agent might lack")
}

func TestSNMPProbeParseFull(t *testing.T) {
	run := snmpRunFor(t, `{"credential_ids":["cred-a"],"port":1161,"timeout_ms":3000,"retries":2}`)

	assert.Equal(t, 1161, run.options.Port)
	assert.Equal(t, 3000, run.options.TimeoutMs)
	assert.Equal(t, 2, run.options.Retries)
	require.Len(t, run.credentials, 1)
	assert.Equal(t, "cred-a", run.credentials[0].ID)
}

func TestSNMPProbeParseDefaults(t *testing.T) {
	run := snmpRunFor(t, `{"credential_ids":["cred-a"]}`)

	assert.Equal(t, defaultSNMPPort, run.options.Port)
	assert.Equal(t, defaultSNMPTimeoutMs, run.options.TimeoutMs)
	assert.Equal(t, defaultSNMPRetries, run.options.Retries)
}

func TestSNMPProbeParseKeepsAnExplicitZeroRetries(t *testing.T) {
	run := snmpRunFor(t, `{"credential_ids":["cred-a"],"retries":0}`)
	assert.Equal(t, 0, run.options.Retries)
}

func TestSNMPProbeParseFallsBackOnAnOutOfBoundsKnob(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want connectivity.SNMPOptions
	}{
		{"port too large", `{"credential_ids":["cred-a"],"port":70000}`, connectivity.SNMPOptions{Port: defaultSNMPPort, TimeoutMs: defaultSNMPTimeoutMs, Retries: defaultSNMPRetries}},
		{"port negative", `{"credential_ids":["cred-a"],"port":-1}`, connectivity.SNMPOptions{Port: defaultSNMPPort, TimeoutMs: defaultSNMPTimeoutMs, Retries: defaultSNMPRetries}},
		{"timeout too large", `{"credential_ids":["cred-a"],"timeout_ms":100000000}`, connectivity.SNMPOptions{Port: defaultSNMPPort, TimeoutMs: defaultSNMPTimeoutMs, Retries: defaultSNMPRetries}},
		{"timeout negative", `{"credential_ids":["cred-a"],"timeout_ms":-1}`, connectivity.SNMPOptions{Port: defaultSNMPPort, TimeoutMs: defaultSNMPTimeoutMs, Retries: defaultSNMPRetries}},
		{"retries too large", `{"credential_ids":["cred-a"],"retries":500}`, connectivity.SNMPOptions{Port: defaultSNMPPort, TimeoutMs: defaultSNMPTimeoutMs, Retries: defaultSNMPRetries}},
		{"retries negative", `{"credential_ids":["cred-a"],"retries":-5}`, connectivity.SNMPOptions{Port: defaultSNMPPort, TimeoutMs: defaultSNMPTimeoutMs, Retries: defaultSNMPRetries}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, snmpRunFor(t, tt.raw).options)
		})
	}
}

func TestSNMPProbeParseRejectsAnEmptyCredentialList(t *testing.T) {
	p, _ := newTestSNMPProbe(t)

	_, err := p.parse(json.RawMessage(`{"port":161}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "credential_ids")
}

func TestSNMPProbeParseRejectsAMalformedBlock(t *testing.T) {
	p, _ := newTestSNMPProbe(t)

	_, err := p.parse(json.RawMessage(`5`))
	require.Error(t, err)
}

func TestSNMPProbeParseIgnoresAnUnknownField(t *testing.T) {
	run := snmpRunFor(t, `{"credential_ids":["cred-a"],"not_a_knob":true}`)
	assert.Equal(t, defaultSNMPPort, run.options.Port)
}

func TestSNMPProbePrepareResolvesCredentialsEachTime(t *testing.T) {
	p, store := newTestSNMPProbe(t)
	cfg, err := p.parse(json.RawMessage(`{"credential_ids":["cred-a"]}`))
	require.NoError(t, err)

	_, err = cfg.prepare()
	require.NoError(t, err)
	_, err = cfg.prepare()
	require.NoError(t, err)

	assert.Equal(t, 2, store.loads(), "a rotation lands without a restart")
}

func TestSNMPProbePrepareFailsOnAMissingCredential(t *testing.T) {
	p, store := newTestSNMPProbe(t)
	cfg, err := p.parse(json.RawMessage(`{"credential_ids":["cred-z"]}`))
	require.NoError(t, err)

	_, err = cfg.prepare()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cred-z")
	assert.NotContains(t, err.Error(), "public", "an error must never carry credential material")
	assert.GreaterOrEqual(t, store.loads(), 1)
}

func TestSNMPRunApplyAppendsItsCheckAndOptions(t *testing.T) {
	run := snmpRunFor(t, `{"credential_ids":["cred-a"],"port":1161}`)

	req := connectivity.Request{Checks: []string{connectivity.CheckPing}}
	run.apply(&req)

	assert.Equal(t, []string{connectivity.CheckPing, connectivity.CheckSNMP}, req.Checks)
	require.NotNil(t, req.SNMPOptions)
	assert.Equal(t, 1161, req.SNMPOptions.Port)
	require.Len(t, req.Credentials, 1)
	assert.Equal(t, "cred-a", req.Credentials[0].ID)
}

func TestSNMPRunFingerprintCoversTheOptionsAndTheSecrets(t *testing.T) {
	base := snmpRunFor(t, `{"credential_ids":["cred-a"],"port":161}`).fingerprint()

	assert.Equal(t, base, snmpRunFor(t, `{"credential_ids":["cred-a"],"port":161}`).fingerprint())
	assert.NotEqual(t, base, snmpRunFor(t, `{"credential_ids":["cred-a"],"port":1161}`).fingerprint())

	rotated := &snmpRun{
		options:     connectivity.SNMPOptions{Port: 161, TimeoutMs: defaultSNMPTimeoutMs, Retries: defaultSNMPRetries},
		credentials: []connectivity.SNMPCredential{{ID: "cred-a", Version: "2c", Community: "rotated"}},
	}
	assert.NotEqual(t, base, rotated.fingerprint())
	assert.NotContains(t, rotated.fingerprint(), "rotated", "the fingerprint is a hash, not the secret")
	assert.True(t, strings.HasPrefix(base, "snmp:"), "a fingerprint names its own kind")
}

func TestSNMPRunFingerprintIgnoresCredentialOrder(t *testing.T) {
	a := &snmpRun{credentials: []connectivity.SNMPCredential{{ID: "cred-a"}, {ID: "cred-b"}}}
	b := &snmpRun{credentials: []connectivity.SNMPCredential{{ID: "cred-b"}, {ID: "cred-a"}}}
	assert.Equal(t, a.fingerprint(), b.fingerprint())
}

func TestSNMPRunRead(t *testing.T) {
	rtt := int64(7)
	run := snmpRunFor(t, `{"credential_ids":["cred-a"]}`)

	answered := run.read(connectivity.DeviceResult{
		IPAddress: "10.0.0.1",
		SNMPResult: &connectivity.SNMPResult{
			CheckResult:   connectivity.CheckResult{Success: true, RttMs: &rtt},
			FailureReason: connectivity.FailureNone,
			CredID:        "cred-a",
			SysName:       "router-1",
		},
	})
	require.NotNil(t, answered)
	assert.Equal(t, metadata.ProbeResult{Kind: "snmp", Status: statusReachable, CredID: "cred-a", RttMs: &rtt}, answered.Result)
	assert.Equal(t, "router-1", answered.Name)

	silent := run.read(connectivity.DeviceResult{
		IPAddress: "10.0.0.2",
		SNMPResult: &connectivity.SNMPResult{
			CheckResult:   connectivity.CheckResult{Success: false},
			FailureReason: connectivity.FailureTimeout,
		},
	})
	require.NotNil(t, silent)
	assert.Equal(t, metadata.ProbeResult{Kind: "snmp", Status: statusUnreachable, FailureReason: connectivity.FailureTimeout}, silent.Result)
	assert.Empty(t, silent.Name)

	assert.Nil(t, run.read(connectivity.DeviceResult{IPAddress: "10.0.0.3"}), "no snmp answer is no snmp reading")
}
