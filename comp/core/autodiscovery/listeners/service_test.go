// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package listeners

import (
	"fmt"
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
	unrelatedDiscoveryTpl := integration.Config{
		Name:          "nginx",
		Provider:      names.File,
		ADIdentifiers: []string{"nginx"},
		Discovery:     &integration.DiscoveryConfig{},
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

	t.Run("discovery dropped when generic integration sibling matches same service", func(t *testing.T) {
		openmetricsSibling := integration.Config{
			Name:          "openmetrics",
			Provider:      names.File,
			ADIdentifiers: []string{"redis"},
			Instances:     []integration.Data{[]byte("namespace: redis\nopenmetrics_endpoint: http://%%host%%:9121/metrics")},
			Source:        "file:openmetrics/conf.yaml",
		}
		configs := map[string]integration.Config{
			discoveryTpl.Digest():          discoveryTpl,
			openmetricsSibling.Digest():    openmetricsSibling,
			unrelatedDiscoveryTpl.Digest(): unrelatedDiscoveryTpl,
		}
		mkSvc(NewStaticConfigIndex()).FilterTemplates(configs)
		assert.NotContains(t, configs, discoveryTpl.Digest(),
			"discovery template should be dropped when an openmetrics config matching its namespace matches the same service, even under a different Name")
		assert.Contains(t, configs, openmetricsSibling.Digest(), "openmetrics config should be kept")
		assert.Contains(t, configs, unrelatedDiscoveryTpl.Digest(),
			"unrelated discovery template should be kept: the match is scoped to the redis namespace, not every integration")
	})

	t.Run("discovery dropped when generic integration is configured host-wide with a matching namespace root", func(t *testing.T) {
		// Simulates a static prometheus config with `namespace: redis`,
		// whose root ("redis") the config manager would add to the same
		// staticConfigIndex used for name-based matching.
		idx := NewStaticConfigIndex()
		idx.Add("redis")

		configs := map[string]integration.Config{
			discoveryTpl.Digest():          discoveryTpl,
			unrelatedDiscoveryTpl.Digest(): unrelatedDiscoveryTpl,
		}
		mkSvc(idx).FilterTemplates(configs)
		assert.NotContains(t, configs, discoveryTpl.Digest(),
			"discovery template should be dropped when a prometheus config with a matching namespace root is scheduled host-wide")
		assert.Contains(t, configs, unrelatedDiscoveryTpl.Digest(),
			"unrelated discovery template should be kept: the match is scoped to the redis namespace, not every integration")
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

	krakendDiscoveryTpl := integration.Config{
		Name:          "krakend",
		Provider:      names.File,
		ADIdentifiers: []string{"krakend"},
		Discovery:     &integration.DiscoveryConfig{},
		Source:        "file:krakend/auto_conf.yaml",
	}

	genericSiblingWithNamespace := func(namespace string) integration.Config {
		cfg := integration.Config{
			Name:          "openmetrics",
			Provider:      names.KubeContainer,
			ADIdentifiers: []string{"container-1"},
			Instances:     []integration.Data{[]byte(fmt.Sprintf("namespace: %s\nopenmetrics_endpoint: http://1.2.3.4:9091/metrics", namespace))},
			Source:        "container:docker://container-1",
		}
		return cfg
	}

	t.Run("discovery dropped when sibling generic integration matches expected namespace", func(t *testing.T) {
		sibling := genericSiblingWithNamespace("krakend.api")
		configs := map[string]integration.Config{
			krakendDiscoveryTpl.Digest(): krakendDiscoveryTpl,
			sibling.Digest():             sibling,
		}
		mkSvc(NewStaticConfigIndex()).FilterTemplates(configs)
		assert.NotContains(t, configs, krakendDiscoveryTpl.Digest(),
			"discovery template should be dropped when a sibling openmetrics config claims a rooted-in namespace")
		assert.Contains(t, configs, sibling.Digest(), "sibling should be kept")
	})

	t.Run("discovery kept when sibling generic integration namespace does not match", func(t *testing.T) {
		sibling := genericSiblingWithNamespace("myapp")
		configs := map[string]integration.Config{
			krakendDiscoveryTpl.Digest(): krakendDiscoveryTpl,
			sibling.Digest():             sibling,
		}
		mkSvc(NewStaticConfigIndex()).FilterTemplates(configs)
		assert.Contains(t, configs, krakendDiscoveryTpl.Digest(),
			"discovery template should be kept when the sibling's namespace doesn't match")
	})

	t.Run("discovery kept when generic sibling sets no namespace at all", func(t *testing.T) {
		sibling := integration.Config{
			Name:          "openmetrics",
			Provider:      names.KubeContainer,
			ADIdentifiers: []string{"container-1"},
			Instances:     []integration.Data{[]byte("openmetrics_endpoint: http://1.2.3.4:9091/metrics")},
			Source:        "container:docker://container-1",
		}
		configs := map[string]integration.Config{
			krakendDiscoveryTpl.Digest(): krakendDiscoveryTpl,
			sibling.Digest():             sibling,
		}
		mkSvc(NewStaticConfigIndex()).FilterTemplates(configs)
		assert.Contains(t, configs, krakendDiscoveryTpl.Digest(),
			"discovery template should be kept when the sibling has no explicit namespace to compare")
	})

	t.Run("discovery dropped when sibling has no namespace but renames a metric to the fully-qualified name", func(t *testing.T) {
		// With no namespace set, an explicit metrics: rename target is
		// submitted completely unprefixed, so a rename to
		// "krakend.api...." collides with krakend's own metrics exactly the
		// same way a matching namespace: field would.
		sibling := integration.Config{
			Name:          "openmetrics",
			Provider:      names.KubeContainer,
			ADIdentifiers: []string{"container-1"},
			Instances: []integration.Data{[]byte(
				"openmetrics_endpoint: http://1.2.3.4:9091/metrics\n" +
					"metrics:\n  - krakend_requests_total: krakend.api.requests_total",
			)},
			Source: "container:docker://container-1",
		}
		configs := map[string]integration.Config{
			krakendDiscoveryTpl.Digest(): krakendDiscoveryTpl,
			sibling.Digest():             sibling,
		}
		mkSvc(NewStaticConfigIndex()).FilterTemplates(configs)
		assert.NotContains(t, configs, krakendDiscoveryTpl.Digest(),
			"discovery template should be dropped when a namespace-less sibling renames a metric to krakend's own namespace")
	})

	t.Run("discovery kept when sibling has a namespace and also renames a metric to a matching name", func(t *testing.T) {
		// The rename target only matters when namespace is unset -- when
		// namespace is set, it's prepended on top of the rename target
		// regardless, so this isn't a real collision.
		sibling := integration.Config{
			Name:          "openmetrics",
			Provider:      names.KubeContainer,
			ADIdentifiers: []string{"container-1"},
			Instances: []integration.Data{[]byte(
				"namespace: myapp\nopenmetrics_endpoint: http://1.2.3.4:9091/metrics\n" +
					"metrics:\n  - krakend_requests_total: krakend.api.requests_total",
			)},
			Source: "container:docker://container-1",
		}
		configs := map[string]integration.Config{
			krakendDiscoveryTpl.Digest(): krakendDiscoveryTpl,
			sibling.Digest():             sibling,
		}
		mkSvc(NewStaticConfigIndex()).FilterTemplates(configs)
		assert.Contains(t, configs, krakendDiscoveryTpl.Digest(),
			"discovery template should be kept: sibling's namespace is prepended regardless of the rename target")
	})

	t.Run("discovery dropped when static config index has a rooted-in namespace match", func(t *testing.T) {
		// The config manager adds the *root* of a static generic-integration
		// config's namespace (see listeners.NamespaceRoot) to the same
		// StaticConfigIndex used for name-based matching, so simulate that
		// here by adding the root directly rather than the raw "krakend.api".
		idx := NewStaticConfigIndex()
		idx.Add("krakend")

		configs := map[string]integration.Config{
			krakendDiscoveryTpl.Digest(): krakendDiscoveryTpl,
		}
		mkSvc(idx).FilterTemplates(configs)
		assert.NotContains(t, configs, krakendDiscoveryTpl.Digest(),
			"discovery template should be dropped when a host-wide static generic-integration config claims a rooted-in namespace")
	})

	t.Run("discovery kept when static config index has no matching namespace root", func(t *testing.T) {
		idx := NewStaticConfigIndex()
		idx.Add("myapp")

		configs := map[string]integration.Config{
			krakendDiscoveryTpl.Digest(): krakendDiscoveryTpl,
		}
		mkSvc(idx).FilterTemplates(configs)
		assert.Contains(t, configs, krakendDiscoveryTpl.Digest(),
			"discovery template should be kept when no tracked namespace root matches")
	})

	t.Run("known gap: namespace divergent from check name is not detected without a map", func(t *testing.T) {
		// zk's own metrics use the "zookeeper" namespace, not "zk". Without a
		// hard-coded check-name-to-namespace override map (deliberately
		// dropped in favor of assuming an integration's namespace root equals
		// its check name), this divergence isn't detected: it's an accepted,
		// documented gap rather than a silent regression.
		zkDiscoveryTpl := integration.Config{
			Name:          "zk",
			Provider:      names.File,
			ADIdentifiers: []string{"zk"},
			Discovery:     &integration.DiscoveryConfig{},
			Source:        "file:zk/auto_conf.yaml",
		}
		sibling := genericSiblingWithNamespace("zookeeper")
		configs := map[string]integration.Config{
			zkDiscoveryTpl.Digest(): zkDiscoveryTpl,
			sibling.Digest():        sibling,
		}
		mkSvc(NewStaticConfigIndex()).FilterTemplates(configs)
		assert.Contains(t, configs, zkDiscoveryTpl.Digest(),
			"zk's discovery template is kept even though a sibling claims the 'zookeeper' namespace, since 'zk' != 'zookeeper' without an override map")
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
