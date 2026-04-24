// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package main

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	traceconfig "github.com/DataDog/datadog-agent/pkg/trace/config"
)

// fakeInnerLoad is a trace.Load stub that returns a caller-supplied
// AgentConfig/error pair. It lets the wrapper tests exercise Load without
// touching the real file-based loader.
type fakeInnerLoad struct {
	cfg *traceconfig.AgentConfig
	err error
}

func (f *fakeInnerLoad) Load() (*traceconfig.AgentConfig, error) {
	return f.cfg, f.err
}

func TestSpanDerivedPrimaryTagsLoader_ValidJSON(t *testing.T) {
	t.Setenv(spanDerivedPrimaryTagsEnvVar, `["region","version","tier"]`)

	inner := &fakeInnerLoad{cfg: &traceconfig.AgentConfig{}}
	loader := &spanDerivedPrimaryTagsLoader{inner: inner}

	tc, err := loader.Load()
	require.NoError(t, err)
	require.NotNil(t, tc)
	assert.Equal(t, []string{"region", "version", "tier"}, tc.SpanDerivedPrimaryTagKeys)
}

func TestSpanDerivedPrimaryTagsLoader_EnvUnset(t *testing.T) {
	// Explicitly clear the env var in case the test host has it set.
	t.Setenv(spanDerivedPrimaryTagsEnvVar, "")

	inner := &fakeInnerLoad{cfg: &traceconfig.AgentConfig{}}
	loader := &spanDerivedPrimaryTagsLoader{inner: inner}

	tc, err := loader.Load()
	require.NoError(t, err)
	require.NotNil(t, tc)
	assert.Nil(t, tc.SpanDerivedPrimaryTagKeys)
}

func TestSpanDerivedPrimaryTagsLoader_MalformedJSON(t *testing.T) {
	t.Setenv(spanDerivedPrimaryTagsEnvVar, `not a json array`)

	inner := &fakeInnerLoad{cfg: &traceconfig.AgentConfig{}}
	loader := &spanDerivedPrimaryTagsLoader{inner: inner}

	tc, err := loader.Load()
	// Malformed JSON must not turn into a Load error; the wrapper logs a
	// warning and leaves the field untouched.
	require.NoError(t, err)
	require.NotNil(t, tc)
	assert.Nil(t, tc.SpanDerivedPrimaryTagKeys)
}

func TestSpanDerivedPrimaryTagsLoader_InnerErrorPropagated(t *testing.T) {
	// Even with a valid env var, an inner Load error must propagate and the
	// wrapper must not mutate the returned config.
	t.Setenv(spanDerivedPrimaryTagsEnvVar, `["region"]`)

	innerErr := errors.New("inner load failed")
	inner := &fakeInnerLoad{cfg: &traceconfig.AgentConfig{}, err: innerErr}
	loader := &spanDerivedPrimaryTagsLoader{inner: inner}

	tc, err := loader.Load()
	require.ErrorIs(t, err, innerErr)
	require.NotNil(t, tc)
	assert.Nil(t, tc.SpanDerivedPrimaryTagKeys)
}
