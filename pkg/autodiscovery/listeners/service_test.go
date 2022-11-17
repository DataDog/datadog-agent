// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package listeners

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/providers/names"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

// filterConfigsDropped applies the given filter to the given configs, and
// returns the configs that the filter dropped.
func filterConfigsDropped(filter func(map[string]integration.Config), configs ...integration.Config) (dropped []integration.Config) {
	byDigest := map[string]integration.Config{}
	for _, c := range configs {
		if _, found := byDigest[c.Digest()]; found {
			panic("duplicate digest") // easy mistake to make with fake templates
		}
		byDigest[c.Digest()] = c
	}

	filter(byDigest)

	dropped = []integration.Config{}
	for _, c := range configs {
		if _, found := byDigest[c.Digest()]; !found {
			dropped = append(dropped, c)
		}
	}
	return
}

func TestServiceFilterTemplatesEmptyOverrides(t *testing.T) {
	filterDrops := func(svc *service, configs ...integration.Config) (dropped []integration.Config) {
		return filterConfigsDropped(svc.filterTemplatesEmptyOverrides, configs...)
	}

	entity := &workloadmeta.Container{EntityID: workloadmeta.EntityID{Kind: "container", ID: "testy"}}
	fileTpl := integration.Config{Provider: names.File, LogsConfig: []byte(`{"source":"file"}`)}
	nonFileTpl := integration.Config{Provider: "something-else", LogsConfig: []byte(`{"source":"nonfile"}`)}
	nothingDropped := []integration.Config{}

	t.Run("nil checkNames", func(t *testing.T) {
		assert.Equal(t, nothingDropped,
			filterDrops(&service{entity: entity, checkNames: nil}, fileTpl))
	})

	t.Run("one checkName", func(t *testing.T) {
		assert.Equal(t, nothingDropped,
			filterDrops(&service{entity: entity, checkNames: []string{"foo"}}, fileTpl))
	})

	t.Run("some checkNames", func(t *testing.T) {
		assert.Equal(t, nothingDropped,
			filterDrops(&service{entity: entity, checkNames: []string{"foo", "bar"}}, fileTpl))
	})

	t.Run("zero checkNames", func(t *testing.T) {
		assert.Equal(t, []integration.Config{fileTpl}, // fileTpl gets dropped, but not non-file
			filterDrops(&service{entity: entity, checkNames: []string{}}, fileTpl, nonFileTpl))
	})

	t.Run("one empty checkName", func(t *testing.T) {
		assert.Equal(t, []integration.Config{fileTpl}, // fileTpl gets dropped, but not non-file
			filterDrops(&service{entity: entity, checkNames: []string{""}}, fileTpl, nonFileTpl))
	})
}

func TestServiceFilterTemplatesOverriddenChecks(t *testing.T) {
	filterDrops := func(svc *service, configs ...integration.Config) (dropped []integration.Config) {
		return filterConfigsDropped(svc.filterTemplatesOverriddenChecks, configs...)
	}

	entity := &workloadmeta.Container{EntityID: workloadmeta.EntityID{Kind: "container", ID: "testy"}}
	fooTpl := integration.Config{Name: "foo", Provider: names.File, LogsConfig: []byte(`{"source":"foo"}`)}
	barTpl := integration.Config{Name: "bar", Provider: names.File, LogsConfig: []byte(`{"source":"bar"}`)}
	fooNonFileTpl := integration.Config{Name: "foo", Provider: "xxx", LogsConfig: []byte(`{"source":"foo-nf"}`)}
	barNonFileTpl := integration.Config{Name: "bar", Provider: "xxx", LogsConfig: []byte(`{"source":"bar-nf"}`)}
	nothingDropped := []integration.Config{}

	t.Run("nil checkNames", func(t *testing.T) {
		assert.Equal(t, nothingDropped,
			filterDrops(&service{entity: entity, checkNames: nil}, fooTpl, barTpl))
	})

	t.Run("one checkName", func(t *testing.T) {
		assert.Equal(t, []integration.Config{fooTpl},
			filterDrops(&service{entity: entity, checkNames: []string{"foo"}}, fooTpl, barTpl, fooNonFileTpl))
	})

	t.Run("some checkNames", func(t *testing.T) {
		assert.Equal(t, []integration.Config{fooTpl, barTpl},
			filterDrops(&service{entity: entity, checkNames: []string{"foo", "bar"}}, fooTpl, barTpl, fooNonFileTpl, barNonFileTpl))
	})

	t.Run("some checkNames, partial match", func(t *testing.T) {
		assert.Equal(t, []integration.Config{barTpl},
			filterDrops(&service{entity: entity, checkNames: []string{"bing", "bar"}}, fooTpl, barTpl, fooNonFileTpl, barNonFileTpl))
	})
}

func TestServiceFilterTemplatesCCA(t *testing.T) {
	filterDrops := func(svc *service, configs ...integration.Config) (dropped []integration.Config) {
		return filterConfigsDropped(svc.filterTemplatesContainerCollectAll, configs...)
	}

	// this should match what's given in pkg/autodiscovery/common/utils/container_collect_all.go
	ccaTpl := integration.Config{Name: "container_collect_all", LogsConfig: []byte("{}")}
	noLogsTpl := integration.Config{Name: "foo"}
	logsTpl := integration.Config{Name: "foo", LogsConfig: []byte(`{"source":"foo"}`)}
	nothingDropped := []integration.Config{}

	t.Run("no CCA config", func(t *testing.T) {
		mockConfig := config.Mock(t)
		mockConfig.Set("logs_config.container_collect_all", true)

		assert.Equal(t, nothingDropped,
			filterDrops(&service{}, logsTpl, noLogsTpl))
	})

	t.Run("no other logs config", func(t *testing.T) {
		mockConfig := config.Mock(t)
		mockConfig.Set("logs_config.container_collect_all", true)

		assert.Equal(t, nothingDropped,
			filterDrops(&service{}, noLogsTpl, ccaTpl))
	})

	t.Run("other logs config", func(t *testing.T) {
		mockConfig := config.Mock(t)
		mockConfig.Set("logs_config.container_collect_all", true)

		assert.Equal(t, []integration.Config{ccaTpl},
			filterDrops(&service{}, noLogsTpl, logsTpl, ccaTpl))
	})

	t.Run("other logs config, CCA disabled", func(t *testing.T) {
		mockConfig := config.Mock(t)
		mockConfig.Set("logs_config.container_collect_all", false)

		assert.Equal(t, nothingDropped,
			filterDrops(&service{}, noLogsTpl, logsTpl, ccaTpl))
	})
}
