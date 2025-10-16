// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package sharedlibrary

import (
	"runtime"
	"testing"

	workloadfilterfxmock "github.com/DataDog/datadog-agent/comp/core/workloadfilter/fx-mock"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	nooptagger "github.com/DataDog/datadog-agent/comp/core/tagger/impl-noop"
	integrations "github.com/DataDog/datadog-agent/comp/logs/integrations/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

func testCreateFakeCheck(t *testing.T) {
	conf := integration.Config{
		Name:       "fake_check",
		Instances:  []integration.Data{integration.Data("{\"value\": 1}")},
		InitConfig: integration.Data("{}"),
		Source:     "fake_check:/path/to/conf/fake_check.yaml",
	}

	senderManager := mocksender.CreateDefaultDemultiplexer()
	logReceiver := option.None[integrations.Component]()
	tagger := nooptagger.NewComponent()
	filterStore := workloadfilterfxmock.SetupMockFilter(t)

	loader, err := NewSharedLibraryCheckLoader(senderManager, logReceiver, tagger, filterStore, &mockSharedLibraryLoader{})
	assert.Nil(t, err)

	check, err := loader.Load(senderManager, conf, conf.Instances[0], 1)
	assert.Nil(t, err)

	// Remove check finalizer that may trigger race condition while testing
	runtime.SetFinalizer(check, nil)

	assert.Nil(t, err)
	assert.Equal(t, "fake_check", check.(*SharedLibraryCheck).name)
	assert.Equal(t, "unversioned", check.(*SharedLibraryCheck).version)
	assert.Equal(t, "fake_check:/path/to/conf/fake_check.yaml", check.(*SharedLibraryCheck).source)
}

func testLoadWithMissingLibrary(t *testing.T) {
	conf := integration.Config{
		Name:       "fake_check",
		Instances:  []integration.Data{integration.Data("{\"value\": 1}")},
		InitConfig: integration.Data("{}"),
		Source:     "fake_check:/path/to/conf/fake_check.yaml",
	}

	senderManager := mocksender.CreateDefaultDemultiplexer()
	logReceiver := option.None[integrations.Component]()
	tagger := nooptagger.NewComponent()
	filterStore := workloadfilterfxmock.SetupMockFilter(t)
	sharedLibraryLoader := newSharedLibraryLoader("folder/path/without/expected/library")

	loader, err := NewSharedLibraryCheckLoader(senderManager, logReceiver, tagger, filterStore, sharedLibraryLoader)
	assert.Nil(t, err)

	_, err = loader.Load(senderManager, conf, conf.Instances[0], 1)
	assert.Error(t, err)
}
