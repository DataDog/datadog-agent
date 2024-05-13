// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func parseServiceCheck(t *testing.T, rawServiceCheck []byte) (dogstatsdServiceCheck, error) {
	deps := newServerDeps(t)
	parser := newParser(deps.Config, newFloat64ListPool(), 1, deps.WMeta)
	return parser.parseServiceCheck(rawServiceCheck)
}

func TestServiceCheckMinimal(t *testing.T) {
	sc, err := parseServiceCheck(t, []byte("_sc|agent.up|0"))

	assert.Nil(t, err)
	assert.Equal(t, "agent.up", sc.name)
	assert.Equal(t, int64(0), sc.timestamp)
	assert.Equal(t, serviceCheckStatusOk, sc.status)
	assert.Equal(t, "", sc.message)
	assert.Equal(t, []string(nil), sc.tags)
}

func TestServiceCheckError(t *testing.T) {
	// not enough information
	_, err := parseServiceCheck(t, []byte("_sc|agent.up"))
	assert.Error(t, err)

	_, err = parseServiceCheck(t, []byte("_sc|agent.up|"))
	assert.Error(t, err)

	_, err = parseServiceCheck(t, []byte("_sc||"))
	assert.Error(t, err)

	_, err = parseServiceCheck(t, []byte("_sc|"))
	assert.Error(t, err)

	// not invalid status
	_, err = parseServiceCheck(t, []byte("_sc|agent.up|OK"))
	assert.Error(t, err)

	// not unknown status
	_, err = parseServiceCheck(t, []byte("_sc|agent.up|21"))
	assert.Error(t, err)

	// invalid timestamp
	_, err = parseServiceCheck(t, []byte("_sc|agent.up|0|d:some_time"))
	assert.NoError(t, err)

	// unknown metadata
	_, err = parseServiceCheck(t, []byte("_sc|agent.up|0|u:unknown"))
	assert.NoError(t, err)
}
func TestServiceCheckMetadataTimestamp(t *testing.T) {
	sc, err := parseServiceCheck(t, []byte("_sc|agent.up|0|d:21"))

	require.Nil(t, err)
	assert.Equal(t, "agent.up", sc.name)
	assert.Equal(t, int64(21), sc.timestamp)
	assert.Equal(t, serviceCheckStatusOk, sc.status)
	assert.Equal(t, "", sc.message)
	assert.Equal(t, []string(nil), sc.tags)
}

func TestServiceCheckMetadataHostname(t *testing.T) {
	sc, err := parseServiceCheck(t, []byte("_sc|agent.up|0|h:localhost"))

	require.Nil(t, err)
	assert.Equal(t, "agent.up", sc.name)
	assert.Equal(t, "localhost", sc.hostname)
	assert.Equal(t, int64(0), sc.timestamp)
	assert.Equal(t, serviceCheckStatusOk, sc.status)
	assert.Equal(t, "", sc.message)
	assert.Equal(t, []string(nil), sc.tags)
}

func TestServiceCheckMetadataTags(t *testing.T) {
	sc, err := parseServiceCheck(t, []byte("_sc|agent.up|0|#tag1,tag2:test,tag3"))

	require.Nil(t, err)
	assert.Equal(t, "agent.up", sc.name)
	assert.Equal(t, int64(0), sc.timestamp)
	assert.Equal(t, serviceCheckStatusOk, sc.status)
	assert.Equal(t, "", sc.message)
	assert.Equal(t, []string{"tag1", "tag2:test", "tag3"}, sc.tags)
}

func TestServiceCheckMetadataMessage(t *testing.T) {
	sc, err := parseServiceCheck(t, []byte("_sc|agent.up|0|m:this is fine"))

	require.Nil(t, err)
	assert.Equal(t, "agent.up", sc.name)
	assert.Equal(t, int64(0), sc.timestamp)
	assert.Equal(t, serviceCheckStatusOk, sc.status)
	assert.Equal(t, "this is fine", sc.message)
	assert.Equal(t, []string(nil), sc.tags)
}

func TestServiceCheckMetadataMultiple(t *testing.T) {
	// all type
	sc, err := parseServiceCheck(t, []byte("_sc|agent.up|0|d:21|h:localhost|#tag1:test,tag2|m:this is fine"))
	require.Nil(t, err)
	assert.Equal(t, "agent.up", sc.name)
	assert.Equal(t, "localhost", sc.hostname)
	assert.Equal(t, int64(21), sc.timestamp)
	assert.Equal(t, serviceCheckStatusOk, sc.status)
	assert.Equal(t, "this is fine", sc.message)
	assert.Equal(t, []string{"tag1:test", "tag2"}, sc.tags)

	// multiple time the same tag
	sc, err = parseServiceCheck(t, []byte("_sc|agent.up|0|d:21|h:localhost|h:localhost2|d:22"))
	require.Nil(t, err)
	assert.Equal(t, "agent.up", sc.name)
	assert.Equal(t, "localhost2", sc.hostname)
	assert.Equal(t, int64(22), sc.timestamp)
	assert.Equal(t, serviceCheckStatusOk, sc.status)
	assert.Equal(t, "", sc.message)
	assert.Equal(t, []string(nil), sc.tags)
}
