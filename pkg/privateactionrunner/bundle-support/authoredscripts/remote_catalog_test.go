// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package authoredscripts

import (
	"encoding/json"
	"runtime"
	"testing"

	fleetcatalog "github.com/DataDog/datadog-agent/pkg/fleet/catalog"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/stretchr/testify/require"
)

const (
	testCatalogProduct = "UPDATER_CATALOG_DD"
	testCatalogFQN     = "com.datadoghq.authoredscripts.helm.addRepo"
	testCatalogPackage = "com.datadoghq.authoredscripts.helm.addrepo"
	testCatalogDigest  = "ea7829a6ebdaa464eb4fbfff4c72e6e63176df58a430a4b0b8dfb66f0e57149c"
)

type testCatalogRCClient struct {
	product        string
	handler        func(map[string]state.RawConfig, func(string, state.ApplyStatus))
	subscribeCount int
}

func (c *testCatalogRCClient) Subscribe(product string, handler func(map[string]state.RawConfig, func(string, state.ApplyStatus))) {
	c.product = product
	c.handler = handler
	c.subscribeCount++
}

func (*testCatalogRCClient) GetConfigTUFProof(string) (state.ConfigTUFProof, bool) {
	return state.ConfigTUFProof{}, false
}

func TestNewRemoteCatalogSubscribesAndFailsClosed(t *testing.T) {
	client := &testCatalogRCClient{}
	catalog, err := NewRemoteCatalog(client, testCatalogProduct)
	require.NoError(t, err)
	require.Equal(t, 1, client.subscribeCount)
	require.Equal(t, testCatalogProduct, client.product)
	require.NotNil(t, client.handler)
	_, err = catalog.Lookup(testCatalogFQN)
	require.ErrorIs(t, err, ErrPackageNotConfigured)
}

func TestNewRemoteCatalogValidatesDependencies(t *testing.T) {
	_, err := NewRemoteCatalog(nil, testCatalogProduct)
	require.Error(t, err)
	_, err = NewRemoteCatalog(&testCatalogRCClient{}, "")
	require.Error(t, err)
}

func TestRemoteCatalogAppliesAndReplacesSnapshot(t *testing.T) {
	client := &testCatalogRCClient{}
	catalog, err := NewRemoteCatalog(client, testCatalogProduct)
	require.NoError(t, err)

	pkg := validRemotePackage()
	status := applyRemoteCatalog(t, client, fleetcatalog.Catalog{Packages: []fleetcatalog.Package{
		pkg,
		{
			Name:    "com.datadoghq.other.package",
			Version: "1.0.0",
			SHA256:  testCatalogDigest,
			URL:     "oci://registry.example.test/other@sha256:" + testCatalogDigest,
		},
		{
			Name:     "com.datadoghq.authoredscripts.otheraction",
			Version:  "1.0.0",
			SHA256:   testCatalogDigest,
			URL:      "oci://registry.example.test/other-action@sha256:" + testCatalogDigest,
			Platform: runtime.GOOS,
			Arch:     "not-" + runtime.GOARCH,
		},
	}})
	require.Equal(t, state.ApplyStateAcknowledged, status.State)

	descriptor, err := catalog.Lookup(testCatalogFQN)
	require.NoError(t, err)
	want := Descriptor{FQN: testCatalogFQN, Package: pkg.Name, Version: pkg.Version, URL: pkg.URL, SHA256: pkg.SHA256}
	require.Equal(t, want, descriptor)
	_, err = catalog.Lookup("com.datadoghq.authoredscripts.otherAction")
	require.ErrorIs(t, err, ErrPackageNotConfigured)

	status = applyRemoteCatalog(t, client, fleetcatalog.Catalog{})
	require.Equal(t, state.ApplyStateAcknowledged, status.State)
	_, err = catalog.Lookup(testCatalogFQN)
	require.ErrorIs(t, err, ErrPackageNotConfigured)
}

func TestRemoteCatalogRejectsInvalidSnapshotWithoutReplacing(t *testing.T) {
	client := &testCatalogRCClient{}
	catalog, err := NewRemoteCatalog(client, testCatalogProduct)
	require.NoError(t, err)
	valid := validRemotePackage()
	status := applyRemoteCatalog(t, client, fleetcatalog.Catalog{Packages: []fleetcatalog.Package{valid}})
	require.Equal(t, state.ApplyStateAcknowledged, status.State)

	invalid := valid
	invalid.URL = "oci://registry.example.test/authored-script@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	status = applyRemoteCatalog(t, client, fleetcatalog.Catalog{Packages: []fleetcatalog.Package{invalid}})
	require.Equal(t, state.ApplyStateError, status.State)
	require.NotEmpty(t, status.Error)

	descriptor, err := catalog.Lookup(testCatalogFQN)
	require.NoError(t, err)
	require.Equal(t, valid.Version, descriptor.Version)
}

func TestRemoteCatalogRejectsMultipleCompatiblePackages(t *testing.T) {
	client := &testCatalogRCClient{}
	_, err := NewRemoteCatalog(client, testCatalogProduct)
	require.NoError(t, err)
	pkg := validRemotePackage()
	duplicate := pkg
	duplicate.Version = "2.0.0"

	status := applyRemoteCatalog(t, client, fleetcatalog.Catalog{Packages: []fleetcatalog.Package{pkg, duplicate}})
	require.Equal(t, state.ApplyStateError, status.State)
	require.NotEmpty(t, status.Error)
}

func validRemotePackage() fleetcatalog.Package {
	return fleetcatalog.Package{
		Name:     testCatalogPackage,
		Version:  "1.2.3",
		SHA256:   testCatalogDigest,
		URL:      "oci://registry.example.test/authored-script@sha256:" + testCatalogDigest,
		Platform: runtime.GOOS,
		Arch:     runtime.GOARCH,
	}
}

func applyRemoteCatalog(t *testing.T, client *testCatalogRCClient, catalog fleetcatalog.Catalog) state.ApplyStatus {
	t.Helper()
	payload, err := json.Marshal(catalog)
	require.NoError(t, err)

	var status state.ApplyStatus
	client.handler(
		map[string]state.RawConfig{"catalog": {Config: payload}},
		func(_ string, applied state.ApplyStatus) { status = applied },
	)
	return status
}
