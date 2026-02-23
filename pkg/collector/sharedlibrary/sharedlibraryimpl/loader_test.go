// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build sharedlibrarycheck

package sharedlibrarycheck

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	workloadfilterfxmock "github.com/DataDog/datadog-agent/comp/core/workloadfilter/fx-mock"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	nooptagger "github.com/DataDog/datadog-agent/comp/core/tagger/impl-noop"
	integrations "github.com/DataDog/datadog-agent/comp/logs/integrations/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/sharedlibrary/ffi"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

func TestLoad_FakeCheck(t *testing.T) {
	conf := integration.Config{
		Name:      "fake_check",
		Instances: []integration.Data{integration.Data("{\"value\": 1}")},
		Source:    "fake_check:/path/to/conf/fake_check.yaml",
	}

	senderManager := mocksender.CreateDefaultDemultiplexer()
	logReceiver := option.None[integrations.Component]()
	tagger := nooptagger.NewComponent()
	filterStore := workloadfilterfxmock.SetupMockFilter(t)
	sharedLibraryLoader := &ffi.NoopSharedLibraryLoader{}

	loader, err := newCheckLoader(senderManager, logReceiver, tagger, filterStore, sharedLibraryLoader)
	require.NoError(t, err)

	check, err := loader.Load(senderManager, conf, conf.Instances[0], 1)
	require.NoError(t, err)

	assert.Equal(t, "fake_check", check.(*Check).name)
	assert.Equal(t, "noop_version", check.(*Check).version)
	assert.Equal(t, "fake_check:/path/to/conf/fake_check.yaml", check.(*Check).source)

	// Remove check finalizer that may trigger race condition while testing
	runtime.SetFinalizer(check, nil)
}

func TestLoad_WithoutLibrary(t *testing.T) {
	conf := integration.Config{
		Name:      "foo",
		Instances: []integration.Data{{}},
	}

	senderManager := mocksender.CreateDefaultDemultiplexer()
	logReceiver := option.None[integrations.Component]()
	tagger := nooptagger.NewComponent()
	filterStore := workloadfilterfxmock.SetupMockFilter(t)
	sharedLibraryLoader, err := ffi.NewSharedLibraryLoader("/library/folder/path/")
	require.NoError(t, err)

	loader, err := newCheckLoader(senderManager, logReceiver, tagger, filterStore, sharedLibraryLoader)
	require.NoError(t, err)

	_, err = loader.Load(senderManager, conf, conf.Instances[0], 1)
	assert.Error(t, err)
}
