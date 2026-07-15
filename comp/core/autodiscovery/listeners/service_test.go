// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package listeners

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/names"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
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

// neverMatchProgram is a MatchingProgram that always returns false, used in
// tests to simulate a config that does not match the current service's entity.
type neverMatchProgram struct{}

func (neverMatchProgram) IsMatched(workloadfilter.Filterable) bool { return false }
func (neverMatchProgram) GetTargetType() workloadfilter.ResourceType {
	return workloadfilter.ContainerType
}

func TestServiceFilterTemplatesEmptyOverrides(t *testing.T) {
	filterDrops := func(svc *WorkloadService, configs ...integration.Config) (dropped []integration.Config) {
		return filterConfigsDropped(svc.filterTemplatesEmptyOverrides, configs...)
	}

	entity := &workloadmeta.Container{EntityID: workloadmeta.EntityID{Kind: "container", ID: "testy"}}
	fileTpl := integration.Config{Provider: names.File, LogsConfig: []byte(`{"source":"file"}`)}
	nonFileTpl := integration.Config{Provider: "something-else", LogsConfig: []byte(`{"source":"nonfile"}`)}
	nothingDropped := []integration.Config{}

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
		assert.Equal(t, []integration.Config{fileTpl}, // fileTpl gets dropped, but not non-file
			filterDrops(&WorkloadService{entity: entity, checkNames: []string{}}, fileTpl, nonFileTpl))
	})

	t.Run("one empty checkName", func(t *testing.T) {
		assert.Equal(t, []integration.Config{fileTpl}, // fileTpl gets dropped, but not non-file
			filterDrops(&WorkloadService{entity: entity, checkNames: []string{""}}, fileTpl, nonFileTpl))
	})
}

func TestServiceFilterTemplatesOverriddenChecks(t *testing.T) {
	filterDrops := func(svc *WorkloadService, configs ...integration.Config) (dropped []integration.Config) {
		return filterConfigsDropped(svc.filterTemplatesOverriddenChecks, configs...)
	}

	entity := &workloadmeta.Container{EntityID: workloadmeta.EntityID{Kind: "container", ID: "testy"}}
	fooTpl := integration.Config{Name: "foo", Provider: names.File, LogsConfig: []byte(`{"source":"foo"}`)}
	barTpl := integration.Config{Name: "bar", Provider: names.File, LogsConfig: []byte(`{"source":"bar"}`)}
	fooInstrTpl := integration.Config{Name: "foo", Provider: names.InstrumentationChecks, LogsConfig: []byte(`{"source":"foo-instr"}`)}
	fooNonFileTpl := integration.Config{Name: "foo", Provider: "xxx", LogsConfig: []byte(`{"source":"foo-nf"}`)}
	barNonFileTpl := integration.Config{Name: "bar", Provider: "xxx", LogsConfig: []byte(`{"source":"bar-nf"}`)}
	nothingDropped := []integration.Config{}

	t.Run("nil checkNames", func(t *testing.T) {
		assert.Equal(t, nothingDropped,
			filterDrops(&WorkloadService{entity: entity, checkNames: nil}, fooTpl, barTpl))
	})

	t.Run("one checkName", func(t *testing.T) {
		assert.Equal(t, []integration.Config{fooTpl},
			filterDrops(&WorkloadService{entity: entity, checkNames: []string{"foo"}}, fooTpl, barTpl, fooNonFileTpl))
	})

	t.Run("some checkNames", func(t *testing.T) {
		assert.Equal(t, []integration.Config{fooTpl, barTpl},
			filterDrops(&WorkloadService{entity: entity, checkNames: []string{"foo", "bar"}}, fooTpl, barTpl, fooNonFileTpl, barNonFileTpl))
	})

	t.Run("some checkNames, partial match", func(t *testing.T) {
		assert.Equal(t, []integration.Config{barTpl},
			filterDrops(&WorkloadService{entity: entity, checkNames: []string{"bing", "bar"}}, fooTpl, barTpl, fooNonFileTpl, barNonFileTpl))
	})

	t.Run("annotation overrides instrumentation check", func(t *testing.T) {
		assert.Equal(t, []integration.Config{fooInstrTpl},
			filterDrops(&WorkloadService{entity: entity, checkNames: []string{"foo"}}, fooInstrTpl, barTpl))
	})

	t.Run("annotation overrides both file and instrumentation check", func(t *testing.T) {
		assert.Equal(t, []integration.Config{fooTpl, fooInstrTpl},
			filterDrops(&WorkloadService{entity: entity, checkNames: []string{"foo"}}, fooTpl, fooInstrTpl, barTpl))
	})
}

func TestServiceFilterTemplatesInstrumentationOverFile(t *testing.T) {
	filterDrops := func(svc *WorkloadService, configs ...integration.Config) (dropped []integration.Config) {
		return filterConfigsDropped(svc.filterTemplatesInstrumentationOverFile, configs...)
	}

	entity := &workloadmeta.Container{EntityID: workloadmeta.EntityID{Kind: "container", ID: "testy"}}
	fooFileTpl := integration.Config{Name: "foo", Provider: names.File, LogsConfig: []byte(`{"source":"foo-file"}`)}
	fooInstrTpl := integration.Config{Name: "foo", Provider: names.InstrumentationChecks, LogsConfig: []byte(`{"source":"foo-instr"}`)}
	barFileTpl := integration.Config{Name: "bar", Provider: names.File, LogsConfig: []byte(`{"source":"bar-file"}`)}
	nothingDropped := []integration.Config{}

	t.Run("file dropped when instrumentation check has same name", func(t *testing.T) {
		assert.Equal(t, []integration.Config{fooFileTpl},
			filterDrops(&WorkloadService{entity: entity}, fooFileTpl, fooInstrTpl))
	})

	t.Run("instrumentation check is kept", func(t *testing.T) {
		assert.NotContains(t,
			filterDrops(&WorkloadService{entity: entity}, fooFileTpl, fooInstrTpl),
			fooInstrTpl)
	})

	t.Run("file kept when no instrumentation check exists", func(t *testing.T) {
		assert.Equal(t, nothingDropped,
			filterDrops(&WorkloadService{entity: entity}, fooFileTpl, barFileTpl))
	})

	t.Run("file kept when instrumentation check has different name", func(t *testing.T) {
		assert.Equal(t, nothingDropped,
			filterDrops(&WorkloadService{entity: entity}, barFileTpl, fooInstrTpl))
	})

	t.Run("multiple files, only matching one dropped", func(t *testing.T) {
		assert.Equal(t, []integration.Config{fooFileTpl},
			filterDrops(&WorkloadService{entity: entity}, fooFileTpl, barFileTpl, fooInstrTpl))
	})
}

func TestServiceFilterTemplatesDiscovery(t *testing.T) {
	entity := &workloadmeta.Container{EntityID: workloadmeta.EntityID{Kind: "container", ID: "redis-1"}}

	mkSvc := func(idx *StaticConfigIndex) *WorkloadService {
		return &WorkloadService{entity: entity, staticConfigIndex: idx}
	}

	discoveryTpl := integration.Config{
		Name:          "redis",
		Provider:      names.File,
		ADIdentifiers: []string{"redis"},
		Discovery:     &integration.DiscoveryConfig{},
		Source:        "file:redis/auto_conf.yaml",
	}
	siblingTpl := integration.Config{
		Name:          "redis",
		Provider:      names.File,
		ADIdentifiers: []string{"redis"},
		Instances:     []integration.Data{[]byte("port: 6379")},
		Source:        "file:redis/auto_conf.yaml",
	}
	unrelatedTpl := integration.Config{
		Name:          "nginx",
		Provider:      names.File,
		ADIdentifiers: []string{"nginx"},
		Instances:     []integration.Data{[]byte("port: 80")},
		Source:        "file:nginx/auto_conf.yaml",
	}

	containsDigests := func(configs map[string]integration.Config, want ...integration.Config) []string {
		t.Helper()
		got := []string{}
		for _, c := range want {
			if _, found := configs[c.Digest()]; found {
				got = append(got, c.Name)
			}
		}
		return got
	}

	t.Run("discovery dropped when sibling template matches same service", func(t *testing.T) {
		configs := map[string]integration.Config{
			discoveryTpl.Digest(): discoveryTpl,
			siblingTpl.Digest():   siblingTpl,
		}
		mkSvc(NewStaticConfigIndex()).FilterTemplates(configs)
		assert.NotContains(t, configs, discoveryTpl.Digest(), "discovery template should be dropped")
		assert.Contains(t, configs, siblingTpl.Digest(), "non-discovery sibling should be kept")
	})

	t.Run("discovery dropped when static config of same name exists", func(t *testing.T) {
		idx := NewStaticConfigIndex()
		idx.Add("redis")

		configs := map[string]integration.Config{
			discoveryTpl.Digest(): discoveryTpl,
			unrelatedTpl.Digest(): unrelatedTpl,
		}
		mkSvc(idx).FilterTemplates(configs)
		assert.NotContains(t, configs, discoveryTpl.Digest(), "discovery template should be dropped")
		assert.Contains(t, configs, unrelatedTpl.Digest(), "unrelated template should be kept")
	})

	t.Run("discovery kept when no sibling and no static config", func(t *testing.T) {
		configs := map[string]integration.Config{
			discoveryTpl.Digest(): discoveryTpl,
			unrelatedTpl.Digest(): unrelatedTpl,
		}
		mkSvc(NewStaticConfigIndex()).FilterTemplates(configs)
		assert.Equal(t, []string{"redis", "nginx"}, containsDigests(configs, discoveryTpl, unrelatedTpl))
	})

	t.Run("discovery kept when static config is for a different integration", func(t *testing.T) {
		idx := NewStaticConfigIndex()
		idx.Add("postgres")

		configs := map[string]integration.Config{
			discoveryTpl.Digest(): discoveryTpl,
		}
		mkSvc(idx).FilterTemplates(configs)
		assert.Contains(t, configs, discoveryTpl.Digest(), "discovery template should be kept when only an unrelated static config exists")
	})

	t.Run("discovery kept when sibling template has only logs config", func(t *testing.T) {
		logsOnlySibling := integration.Config{
			Name:          "redis",
			Provider:      names.File,
			ADIdentifiers: []string{"redis"},
			LogsConfig:    []byte(`{"source":"redis"}`),
			Source:        "file:redis/auto_conf.yaml",
		}
		configs := map[string]integration.Config{
			discoveryTpl.Digest():    discoveryTpl,
			logsOnlySibling.Digest(): logsOnlySibling,
		}
		mkSvc(NewStaticConfigIndex()).FilterTemplates(configs)
		assert.Contains(t, configs, discoveryTpl.Digest(),
			"discovery template should be kept when the sibling is logs-only")
		assert.Contains(t, configs, logsOnlySibling.Digest(),
			"logs-only sibling should be kept")
	})

	t.Run("instrumentation check overrides matched file check", func(t *testing.T) {
		fileTpl := integration.Config{
			Name:      "redis",
			Provider:  names.File,
			Instances: []integration.Data{[]byte("port: 6379")},
			Source:    "file:redis/conf.yaml",
		}
		instrTpl := integration.Config{
			Name:      "redis",
			Provider:  names.InstrumentationChecks,
			Instances: []integration.Data{[]byte("{}")},
			Source:    "instrumentation:redis",
		}
		configs := map[string]integration.Config{
			fileTpl.Digest():  fileTpl,
			instrTpl.Digest(): instrTpl,
		}
		mkSvc(NewStaticConfigIndex()).FilterTemplates(configs)
		assert.NotContains(t, configs, fileTpl.Digest(), "file template should be dropped in favour of the instrumentation check")
		assert.Contains(t, configs, instrTpl.Digest(), "instrumentation template should be kept")
	})

	t.Run("file kept when instrumentation check does not match service", func(t *testing.T) {
		fileTpl := integration.Config{
			Name:      "redis",
			Provider:  names.File,
			Instances: []integration.Data{[]byte("port: 6379")},
			Source:    "file:redis/conf.yaml",
		}
		instrTpl := integration.Config{
			Name:      "redis",
			Provider:  names.InstrumentationChecks,
			Instances: []integration.Data{[]byte("{}")},
			Source:    "instrumentation:redis",
		}
		instrTpl.SetMatchingPrograms(map[workloadfilter.ResourceType]integration.MatchingProgram{
			workloadfilter.ContainerType: neverMatchProgram{},
		})
		configs := map[string]integration.Config{
			fileTpl.Digest():  fileTpl,
			instrTpl.Digest(): instrTpl,
		}
		mkSvc(NewStaticConfigIndex()).FilterTemplates(configs)
		assert.Contains(t, configs, fileTpl.Digest(), "file template should be kept when instrumentation check does not match the service")
		assert.NotContains(t, configs, instrTpl.Digest(), "non-matching instrumentation template should be dropped")
	})
}

func TestServiceFilterTemplatesCCA(t *testing.T) {
	filterDrops := func(svc *WorkloadService, configs ...integration.Config) (dropped []integration.Config) {
		return filterConfigsDropped(svc.filterTemplatesContainerCollectAll, configs...)
	}

	// this should match what's given in comp/core/autodiscovery/common/utils/container_collect_all.go
	ccaTpl := integration.Config{Name: "container_collect_all", LogsConfig: []byte("{}")}
	noLogsTpl := integration.Config{Name: "foo"}
	logsTpl := integration.Config{Name: "foo", LogsConfig: []byte(`{"source":"foo"}`)}
	nothingDropped := []integration.Config{}

	t.Run("no CCA config", func(t *testing.T) {
		mockConfig := configmock.New(t)
		mockConfig.SetInTest("logs_config.container_collect_all", true)

		assert.Equal(t, nothingDropped,
			filterDrops(&WorkloadService{}, logsTpl, noLogsTpl))
	})

	t.Run("no other logs config", func(t *testing.T) {
		mockConfig := configmock.New(t)
		mockConfig.SetInTest("logs_config.container_collect_all", true)

		assert.Equal(t, nothingDropped,
			filterDrops(&WorkloadService{}, noLogsTpl, ccaTpl))
	})

	t.Run("other logs config", func(t *testing.T) {
		mockConfig := configmock.New(t)
		mockConfig.SetInTest("logs_config.container_collect_all", true)

		assert.Equal(t, []integration.Config{ccaTpl},
			filterDrops(&WorkloadService{}, noLogsTpl, logsTpl, ccaTpl))
	})

	t.Run("other logs config, CCA disabled", func(t *testing.T) {
		mockConfig := configmock.New(t)
		mockConfig.SetInTest("logs_config.container_collect_all", false)

		assert.Equal(t, nothingDropped,
			filterDrops(&WorkloadService{}, noLogsTpl, logsTpl, ccaTpl))
	})
}
