// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"context"
	"fmt"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/collector/python"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/logs/status"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/util/installinfo"
)

func TestOTLPEnabled(t *testing.T) {
	defer cache.Cache.Delete(hostCacheKey)

	ctx := context.Background()
	conf := config.Mock(t)

	defer func(orig func(cfg config.Reader) bool) { otlpIsEnabled = orig }(otlpIsEnabled)

	otlpIsEnabled = func(config.Reader) bool { return false }
	p := GetPayload(ctx, conf)
	assert.False(t, p.OtlpMeta.Enabled)

	otlpIsEnabled = func(config.Reader) bool { return true }
	p = GetPayload(ctx, conf)
	assert.True(t, p.OtlpMeta.Enabled)
}

func TestGetNetworkMeta(t *testing.T) {
	ctx := context.Background()
	cloudproviders.MockNetworkID(t, "test networkID")

	m := getNetworkMeta(ctx)
	assert.Equal(t, "test networkID", m.ID)
}

func TestGetLogsMeta(t *testing.T) {
	conf := config.Mock(t)
	defer status.SetCurrentTransport("")

	status.SetCurrentTransport("")
	meta := getLogsMeta(conf)
	assert.Equal(t, &LogsMeta{Transport: "", AutoMultilineEnabled: false}, meta)

	status.SetCurrentTransport(status.TransportTCP)
	meta = getLogsMeta(conf)
	assert.Equal(t, &LogsMeta{Transport: "TCP", AutoMultilineEnabled: false}, meta)

	conf.Set("logs_config.auto_multi_line_detection", true)
	meta = getLogsMeta(conf)
	assert.Equal(t, &LogsMeta{Transport: "TCP", AutoMultilineEnabled: true}, meta)
}

func TestGetInstallMethod(t *testing.T) {
	conf := config.Mock(t)
	defer func(orig func(conf config.Reader) (*installinfo.InstallInfo, error)) {
		installinfoGet = orig
	}(installinfoGet)

	installinfoGet = func(config.Reader) (*installinfo.InstallInfo, error) { return nil, fmt.Errorf("an error") }

	installMethod := getInstallMethod(conf)
	assert.Equal(t, "undefined", installMethod.ToolVersion)
	assert.Nil(t, installMethod.Tool)
	assert.Nil(t, installMethod.InstallerVersion)

	installinfoGet = func(config.Reader) (*installinfo.InstallInfo, error) {
		return &installinfo.InstallInfo{
			ToolVersion:      "chef-15",
			Tool:             "chef",
			InstallerVersion: "datadog-cookbook-4.2.1",
		}, nil
	}

	installMethod = getInstallMethod(conf)
	assert.Equal(t, "chef-15", installMethod.ToolVersion)
	assert.Equal(t, "chef", *installMethod.Tool)
	assert.Equal(t, "datadog-cookbook-4.2.1", *installMethod.InstallerVersion)
}

func TestGetProxyMeta(t *testing.T) {
	conf := config.Mock(t)
	httputils.MockWarnings(t, nil, nil, nil)

	conf.Set("no_proxy_nonexact_match", false)
	meta := getProxyMeta(conf)
	assert.Equal(t, meta.NoProxyNonexactMatch, false)
	assert.Equal(t, meta.ProxyBehaviorChanged, false)
	assert.Equal(t, meta.NoProxyNonexactMatchExplicitlySet, true)

	conf.Set("no_proxy_nonexact_match", true)
	meta = getProxyMeta(conf)
	assert.Equal(t, meta.NoProxyNonexactMatch, true)
	assert.Equal(t, meta.ProxyBehaviorChanged, false)
	assert.Equal(t, meta.NoProxyNonexactMatchExplicitlySet, true)

	httputils.MockWarnings(t, nil, nil, []string{"a", "b", "c"})
	meta = getProxyMeta(conf)
	assert.Equal(t, meta.NoProxyNonexactMatch, true)
	assert.Equal(t, meta.ProxyBehaviorChanged, true)
	assert.Equal(t, meta.NoProxyNonexactMatchExplicitlySet, true)
}

func TestGetPayload(t *testing.T) {
	defer cache.Cache.Delete(hostCacheKey)

	ctx := context.Background()
	conf := config.Mock(t)

	_, found := cache.Cache.Get(hostCacheKey)
	assert.False(t, found)

	p := GetPayload(ctx, conf)
	if runtime.GOOS == "windows" {
		assert.Equal(t, "win32", p.Os)
	} else {
		assert.Equal(t, runtime.GOOS, p.Os)
	}

	assert.Equal(t, flavor.GetFlavor(), p.AgentFlavor)
	assert.Equal(t, python.GetPythonVersion(), p.PythonVersion)
	assert.NotNil(t, p.SystemStats)
	assert.NotNil(t, p.Meta)
	assert.NotNil(t, p.HostTags)
	assert.NotNil(t, p.ContainerMeta)
	assert.NotNil(t, p.LogsMeta)
	assert.NotNil(t, p.InstallMethod)
	assert.NotNil(t, p.ProxyMeta)
	assert.NotNil(t, p.OtlpMeta)

	_, found = cache.Cache.Get(hostCacheKey)
	assert.True(t, found)
}

func TestGetFromCache(t *testing.T) {
	defer cache.Cache.Delete(hostCacheKey)

	ctx := context.Background()
	conf := config.Mock(t)

	cache.Cache.Set(hostCacheKey, &Payload{Os: "testOS"}, cache.NoExpiration)
	p := GetFromCache(ctx, conf)
	require.NotNil(t, p)
	assert.Equal(t, "testOS", p.Os)
}
