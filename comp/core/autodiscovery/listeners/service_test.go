// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package listeners

import (
	"testing"

	"github.com/stretchr/testify/assert"

	adtypes "github.com/DataDog/datadog-agent/comp/core/autodiscovery/common/types"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/names"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

// filterConfigsDropped applies the given filter to the given configs, and
// returns the configs that the filter dropped.
func filterConfigsDropped(filter func(map[string]adtypes.InternalConfig), configs ...adtypes.InternalConfig) (dropped []adtypes.InternalConfig) {
	byDigest := map[string]adtypes.InternalConfig{}
	for _, c := range configs {
		if _, found := byDigest[c.Digest()]; found {
			panic("duplicate digest") // easy mistake to make with fake templates
		}
		byDigest[c.Digest()] = c
	}

	filter(byDigest)

	dropped = []adtypes.InternalConfig{}
	for _, c := range configs {
		if _, found := byDigest[c.Digest()]; !found {
			dropped = append(dropped, c)
		}
	}
	return
}

func TestServiceFilterTemplatesEmptyOverrides(t *testing.T) {
	filterDrops := func(svc *WorkloadService, configs ...adtypes.InternalConfig) (dropped []adtypes.InternalConfig) {
		return filterConfigsDropped(svc.filterTemplatesEmptyOverrides, configs...)
	}

	entity := &workloadmeta.Container{EntityID: workloadmeta.EntityID{Kind: "container", ID: "testy"}}
	fileTpl := adtypes.InternalConfig{Config: integration.Config{Provider: names.File, LogsConfig: []byte(`{"source":"file"}`)}}
	nonFileTpl := adtypes.InternalConfig{Config: integration.Config{Provider: "something-else", LogsConfig: []byte(`{"source":"nonfile"}`)}}
	nothingDropped := []adtypes.InternalConfig{}

	t.Run("nil checkNames", func(t *testing.T) {
		assert.Equal(t, nothingDropped,
			filterDrops(&WorkloadService{entity: entity, checkNames: nil}, fileTpl))
	})

	t.Run("one checkName", func(t *testing.T) {
		assert.Equal(t, nothingDropped,
			filterDrops(&WorkloadService{entity: entity, checkNames: []string{"foo"}}, fileTpl))
	})

	t.Run("some checkNames", func(t *testing.T) {
		assert.Equal(t, nothingDropped,
			filterDrops(&WorkloadService{entity: entity, checkNames: []string{"foo", "bar"}}, fileTpl))
	})

	t.Run("zero checkNames", func(t *testing.T) {
		assert.Equal(t, []adtypes.InternalConfig{fileTpl}, // fileTpl gets dropped, but not non-file
			filterDrops(&WorkloadService{entity: entity, checkNames: []string{}}, fileTpl, nonFileTpl))
	})

	t.Run("one empty checkName", func(t *testing.T) {
		assert.Equal(t, []adtypes.InternalConfig{fileTpl}, // fileTpl gets dropped, but not non-file
			filterDrops(&WorkloadService{entity: entity, checkNames: []string{""}}, fileTpl, nonFileTpl))
	})
}

func TestServiceFilterTemplatesOverriddenChecks(t *testing.T) {
	filterDrops := func(svc *WorkloadService, configs ...adtypes.InternalConfig) (dropped []adtypes.InternalConfig) {
		return filterConfigsDropped(svc.filterTemplatesOverriddenChecks, configs...)
	}

	entity := &workloadmeta.Container{EntityID: workloadmeta.EntityID{Kind: "container", ID: "testy"}}
	fooTpl := adtypes.InternalConfig{Config: integration.Config{Name: "foo", Provider: names.File, LogsConfig: []byte(`{"source":"foo"}`)}}
	barTpl := adtypes.InternalConfig{Config: integration.Config{Name: "bar", Provider: names.File, LogsConfig: []byte(`{"source":"bar"}`)}}
	fooNonFileTpl := adtypes.InternalConfig{Config: integration.Config{Name: "foo", Provider: "xxx", LogsConfig: []byte(`{"source":"foo-nf"}`)}}
	barNonFileTpl := adtypes.InternalConfig{Config: integration.Config{Name: "bar", Provider: "xxx", LogsConfig: []byte(`{"source":"bar-nf"}`)}}
	nothingDropped := []adtypes.InternalConfig{}

	t.Run("nil checkNames", func(t *testing.T) {
		assert.Equal(t, nothingDropped,
			filterDrops(&WorkloadService{entity: entity, checkNames: nil}, fooTpl, barTpl))
	})

	t.Run("one checkName", func(t *testing.T) {
		assert.Equal(t, []adtypes.InternalConfig{fooTpl},
			filterDrops(&WorkloadService{entity: entity, checkNames: []string{"foo"}}, fooTpl, barTpl, fooNonFileTpl))
	})

	t.Run("some checkNames", func(t *testing.T) {
		assert.Equal(t, []adtypes.InternalConfig{fooTpl, barTpl},
			filterDrops(&WorkloadService{entity: entity, checkNames: []string{"foo", "bar"}}, fooTpl, barTpl, fooNonFileTpl, barNonFileTpl))
	})

	t.Run("some checkNames, partial match", func(t *testing.T) {
		assert.Equal(t, []adtypes.InternalConfig{barTpl},
			filterDrops(&WorkloadService{entity: entity, checkNames: []string{"bing", "bar"}}, fooTpl, barTpl, fooNonFileTpl, barNonFileTpl))
	})
}

func TestServiceFilterTemplatesCCA(t *testing.T) {
	filterDrops := func(svc *WorkloadService, configs ...adtypes.InternalConfig) (dropped []adtypes.InternalConfig) {
		return filterConfigsDropped(svc.filterTemplatesContainerCollectAll, configs...)
	}

	// this should match what's given in comp/core/autodiscovery/common/utils/container_collect_all.go
	ccaTpl := adtypes.InternalConfig{Config: integration.Config{Name: "container_collect_all", LogsConfig: []byte("{}")}}
	noLogsTpl := adtypes.InternalConfig{Config: integration.Config{Name: "foo"}}
	logsTpl := adtypes.InternalConfig{Config: integration.Config{Name: "foo", LogsConfig: []byte(`{"source":"foo"}`)}}
	nothingDropped := []adtypes.InternalConfig{}

	t.Run("no CCA config", func(t *testing.T) {
		mockConfig := configmock.New(t)
		mockConfig.SetWithoutSource("logs_config.container_collect_all", true)

		assert.Equal(t, nothingDropped,
			filterDrops(&WorkloadService{}, logsTpl, noLogsTpl))
	})

	t.Run("no other logs config", func(t *testing.T) {
		mockConfig := configmock.New(t)
		mockConfig.SetWithoutSource("logs_config.container_collect_all", true)

		assert.Equal(t, nothingDropped,
			filterDrops(&WorkloadService{}, noLogsTpl, ccaTpl))
	})

	t.Run("other logs config", func(t *testing.T) {
		mockConfig := configmock.New(t)
		mockConfig.SetWithoutSource("logs_config.container_collect_all", true)

		assert.Equal(t, []adtypes.InternalConfig{ccaTpl},
			filterDrops(&WorkloadService{}, noLogsTpl, logsTpl, ccaTpl))
	})

	t.Run("other logs config, CCA disabled", func(t *testing.T) {
		mockConfig := configmock.New(t)
		mockConfig.SetWithoutSource("logs_config.container_collect_all", false)

		assert.Equal(t, nothingDropped,
			filterDrops(&WorkloadService{}, noLogsTpl, logsTpl, ccaTpl))
	})
}
