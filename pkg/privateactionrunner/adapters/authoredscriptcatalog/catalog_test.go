// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package authoredscriptcatalog

import (
	"encoding/json"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	installercatalog "github.com/DataDog/datadog-agent/pkg/fleet/installer/catalog"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/authoredscripts"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

const (
	testFQN     = "com.datadoghq.authoredscripts.helm.addRepo"
	testPackage = "com.datadoghq.authoredscripts.helm.addrepo"
	testDigest  = "ea7829a6ebdaa464eb4fbfff4c72e6e63176df58a430a4b0b8dfb66f0e57149c"
)

type testRCClient struct {
	product string
	handler func(map[string]state.RawConfig, func(string, state.ApplyStatus))
}

func (c *testRCClient) Subscribe(product string, handler func(map[string]state.RawConfig, func(string, state.ApplyStatus))) {
	c.product = product
	c.handler = handler
}

func TestNewUsesStaticCatalogWhenRemoteCatalogIsUnavailable(t *testing.T) {
	for _, test := range []struct {
		name    string
		enabled bool
		product string
	}{
		{name: "disabled", product: "UPDATER_CATALOG_DD"},
		{name: "product unavailable", enabled: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := &testRCClient{}
			catalog := New(test.enabled, test.product, client)

			_, ok := catalog.(*authoredscripts.StaticCatalog)
			require.True(t, ok)
			require.Empty(t, client.product)
			require.Nil(t, client.handler)
		})
	}
}

func TestNewSubscribesAndAppliesRemoteCatalog(t *testing.T) {
	client := &testRCClient{}
	catalog := New(true, "UPDATER_CATALOG_DD", client)

	require.Equal(t, "UPDATER_CATALOG_DD", client.product)
	require.NotNil(t, client.handler)
	_, err := catalog.Lookup(testFQN)
	require.ErrorIs(t, err, authoredscripts.ErrPackageNotConfigured)

	remotePackage := installercatalog.Package{
		Name:     testPackage,
		Version:  "1.2.3",
		SHA256:   testDigest,
		URL:      "oci://registry.example.test/authored-scripts@sha256:" + testDigest,
		Platform: runtime.GOOS,
		Arch:     runtime.GOARCH,
	}
	payload, err := json.Marshal(installercatalog.Catalog{Packages: []installercatalog.Package{remotePackage}})
	require.NoError(t, err)

	statuses := make(map[string]state.ApplyStatus)
	client.handler(
		map[string]state.RawConfig{"catalog": {Config: payload}},
		func(path string, status state.ApplyStatus) { statuses[path] = status },
	)

	require.Equal(t, state.ApplyStateAcknowledged, statuses["catalog"].State)
	descriptor, err := catalog.Lookup(testFQN)
	require.NoError(t, err)
	require.Equal(t, authoredscripts.Descriptor{
		FQN:     testFQN,
		Package: testPackage,
		Version: remotePackage.Version,
		URL:     remotePackage.URL,
		SHA256:  testDigest,
	}, descriptor)
}

func TestRemoteCatalogRejectsMismatchedDigest(t *testing.T) {
	catalog := &remoteCatalog{}
	err := catalog.replace(installercatalog.Catalog{Packages: []installercatalog.Package{{
		Name:    testPackage,
		Version: "1.2.3",
		SHA256:  testDigest,
		URL:     "oci://registry.example.test/authored-scripts@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
	}}})

	require.ErrorContains(t, err, "does not match expected digest")
}
