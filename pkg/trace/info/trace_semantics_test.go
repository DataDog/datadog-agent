// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package info

import (
	"expvar"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/semantics"
)

// restoreEmbeddedRegistry installs a t.Cleanup that resets the process-global
// registry to the embedded mappings.json, since the registry is shared across
// the process.
func restoreEmbeddedRegistry(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		r, err := semantics.NewEmbeddedRegistry()
		require.NoError(t, err)
		semantics.UpdateRegistry(r)
	})
}

func TestPublishTraceSemanticsInfo_Embedded(t *testing.T) {
	restoreEmbeddedRegistry(t)

	embedded, err := semantics.NewEmbeddedRegistry()
	require.NoError(t, err)
	semantics.UpdateRegistry(embedded)

	info, ok := publishTraceSemanticsInfo().(TraceSemanticsInfo)
	require.True(t, ok)
	require.Equal(t, semantics.SourceEmbedded, info.Source)
	require.Equal(t, embedded.ContentHash(), info.ContentHash)
	require.Equal(t, embedded.Version(), info.Version)
}

func TestPublishTraceSemanticsInfo_RemoteConfig(t *testing.T) {
	restoreEmbeddedRegistry(t)

	const rcJSON = `{"version":"rc-1.0","metadata":{"content_hash":"hash-rc"},"concepts":{"db.statement":{"canonical":"db.statement","fallbacks":[{"name":"db.statement","provider":"datadog","type":"string"}]}}}`
	reg, err := semantics.NewRegistryFromJSON([]byte(rcJSON))
	require.NoError(t, err)
	semantics.UpdateRegistry(reg)

	info, ok := publishTraceSemanticsInfo().(TraceSemanticsInfo)
	require.True(t, ok)
	require.Equal(t, semantics.SourceRemoteConfig, info.Source)
	require.Equal(t, "hash-rc", info.ContentHash)
	require.Equal(t, "rc-1.0", info.Version)
}

// TestPublishTraceSemanticsInfo_RemoteConfigMatchingEmbeddedHash guards the
// case an RC payload carries the same content_hash as the embedded registry:
// the source must still be reported as remote-config (provenance is tracked on
// the registry, not inferred from the hash).
func TestPublishTraceSemanticsInfo_RemoteConfigMatchingEmbeddedHash(t *testing.T) {
	restoreEmbeddedRegistry(t)

	embedded, err := semantics.NewEmbeddedRegistry()
	require.NoError(t, err)

	rcJSON := `{"version":"rc-same","metadata":{"content_hash":"` + embedded.ContentHash() + `"},"concepts":{"db.statement":{"canonical":"db.statement","fallbacks":[{"name":"db.statement","provider":"datadog","type":"string"}]}}}`
	reg, err := semantics.NewRegistryFromJSON([]byte(rcJSON))
	require.NoError(t, err)
	semantics.UpdateRegistry(reg)

	info, ok := publishTraceSemanticsInfo().(TraceSemanticsInfo)
	require.True(t, ok)
	require.Equal(t, semantics.SourceRemoteConfig, info.Source)
	require.Equal(t, embedded.ContentHash(), info.ContentHash)
}

func TestTraceSemanticsExpvarRegistered(t *testing.T) {
	require.NoError(t, InitInfo(config.New()))
	require.NotNil(t, expvar.Get("trace_semantics"))
}
