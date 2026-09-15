// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package ndm

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/types"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

func TestParseDocument(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		wantKeys []string
		wantErr  bool
	}{
		{name: "an snmp document", raw: `{"snmp":{"instances":[]}}`, wantKeys: []string{"snmp"}},
		{name: "two feature keys", raw: `{"snmp":{},"autodiscovery":{}}`, wantKeys: []string{"autodiscovery", "snmp"}},
		{name: "an empty object", raw: `{}`, wantKeys: nil},
		{name: "not valid json", raw: `{`, wantErr: true},
		{name: "an empty payload", raw: ``, wantErr: true},
		{name: "a json array", raw: `[1,2]`, wantErr: true},
		{name: "a json string", raw: `"snmp"`, wantErr: true},
		{name: "a json number", raw: `7`, wantErr: true},
		{name: "json null", raw: `null`, wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			keys, err := parseDocument([]byte(tc.raw))
			if tc.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)

			got := make([]string, 0, len(keys))
			for k := range keys {
				got = append(got, k)
			}
			assert.ElementsMatch(t, tc.wantKeys, got)
		})
	}
}

func TestParseDocumentKeepsTheFeatureValueUntouched(t *testing.T) {
	keys, err := parseDocument([]byte(`{"snmp":{"instances":[{"ip_address":"10.0.0.1"}]}}`))
	require.NoError(t, err)
	assert.JSONEq(t, `{"instances":[{"ip_address":"10.0.0.1"}]}`, string(keys["snmp"]))
}

func TestDispatchCallsOnlyTheHandlersWhoseKeyIsPresent(t *testing.T) {
	snmp := &fakeHandler{key: "snmp", configs: map[string][]integration.Config{
		"path-a": {{Name: "snmp", Source: "ndm-remote-config:snmp"}},
	}}
	ad := &fakeHandler{key: "autodiscovery"}
	p := newTestProvider(t, snmp, ad)

	keys, err := parseDocument([]byte(`{"snmp":{},"unknown":{}}`))
	require.NoError(t, err)

	configsByKey, errsByKey, owned := p.dispatch("path-a", keys)

	assert.True(t, owned, "a registered key is present, so the document is ours")
	assert.Empty(t, errsByKey)
	assert.Len(t, configsByKey["snmp"], 1)
	assert.Equal(t, []string{"path-a"}, snmp.renderedPaths)
	assert.Empty(t, ad.renderedPaths, "a registered key absent from the document is not dispatched")
}

func TestDispatchDoesNotOwnADocumentWithNoRegisteredKey(t *testing.T) {
	snmp := &fakeHandler{key: "snmp"}
	p := newTestProvider(t, snmp)

	for _, raw := range []string{`{}`, `{"unknown":{}}`, `{"debug-config-pct":1}`} {
		keys, err := parseDocument([]byte(raw))
		require.NoError(t, err)

		_, _, owned := p.dispatch("path-a", keys)
		assert.False(t, owned, "document %s carries no registered key", raw)
	}
	assert.Empty(t, snmp.renderedPaths)
}

func TestDispatchKeepsOneHandlersFailureFromStoppingAnother(t *testing.T) {
	snmp := &fakeHandler{key: "snmp", configs: map[string][]integration.Config{
		"path-a": {{Name: "snmp"}},
	}}
	ad := &fakeHandler{key: "autodiscovery", err: errors.New("range 10.0.0.0/8 is too large")}
	p := newTestProvider(t, snmp, ad)

	keys, err := parseDocument([]byte(`{"snmp":{},"autodiscovery":{}}`))
	require.NoError(t, err)

	configsByKey, errsByKey, owned := p.dispatch("path-a", keys)

	assert.True(t, owned)
	assert.Len(t, configsByKey["snmp"], 1, "the succeeding key still yields its configs")
	require.Contains(t, errsByKey, "autodiscovery")
	assert.EqualError(t, errsByKey["autodiscovery"], "range 10.0.0.0/8 is too large")
}

func TestApplyStatusIsAcknowledgedOnlyWhenEveryDispatchedKeySucceeded(t *testing.T) {
	keys := []string{"autodiscovery", "snmp"}

	got := applyStatus(nil, keys)
	assert.Equal(t, state.ApplyStateAcknowledged, got.State)
	assert.Empty(t, got.Error)

	got = applyStatus(map[string]error{"snmp": errors.New("credential \"c1\" is not available")}, keys)
	assert.Equal(t, state.ApplyStateError, got.State)
	assert.Equal(t, `snmp: credential "c1" is not available`, got.Error)
}

func TestApplyStatusJoinsEveryFailingKeyInRegistrationOrder(t *testing.T) {
	got := applyStatus(map[string]error{
		"snmp":          errors.New("second"),
		"autodiscovery": errors.New("first"),
	}, []string{"autodiscovery", "snmp"})

	assert.Equal(t, state.ApplyStateError, got.State)
	assert.Equal(t, "autodiscovery: first; snmp: second", got.Error)
}

func TestErrorSetPrefixesEveryMessageWithItsKey(t *testing.T) {
	got := errorSet(map[string]error{
		"snmp":          errors.New("credential \"c1\" is not available"),
		"autodiscovery": errors.New("range is too large"),
	}, []string{"autodiscovery", "snmp"})

	assert.Equal(t, types.ErrorMsgSet{
		"autodiscovery: range is too large":      {},
		`snmp: credential "c1" is not available`: {},
	}, got)
}

func TestErrorSetOfNoErrorsIsNil(t *testing.T) {
	assert.Nil(t, errorSet(nil, []string{"snmp"}))
	assert.Nil(t, errorSet(map[string]error{}, []string{"snmp"}))
}
