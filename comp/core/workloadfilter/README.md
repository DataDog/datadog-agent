# Workload Filter Component

The `workloadfilter` component provides a unified filtering mechanism for determining which containers, pods, services, endpoints, etc. should be included or excluded from various Datadog Agent operations like metrics collection, log collection, autodiscovery, and so on.

## Overview

The workload filter component uses a precedence-based filtering system where filters are organized into groups. If a filter group produces an Include or Exclude result, subsequent groups are not evaluated. This allows for fine-grained control over which workloads are processed by different Agent features.

### Key Features

- **Multi-resource support**: Filters containers, pods, services, endpoints, etc.
- **Precedence-based evaluation**: Filter groups are evaluated in order, with lower-indexed groups taking precedence
- **CEL-based expressions**: Uses Google's Common Expression Language for powerful filtering logic
- **Legacy configuration support**: Maintains compatibility with existing container filtering configurations

## Component Interface

The main component interface provides methods to check if workloads should be excluded:

```go
type Component interface {
	// GetContainerFilters retrieves the selected container FilterBundle
	GetContainerFilters(containerFilters [][]ContainerFilter) FilterBundle
	// GetPodFilters retrieves the selected container FilterBundle
	GetPodFilters(podFilters [][]PodFilter) FilterBundle
    // GetKubeServiceFilters retrieves the selected container FilterBundle
	GetKubeServiceFilters(serviceFilters [][]KubeServiceFilter) FilterBundle
    // GetKubeEndpointFilters retrieves the selected container FilterBundle
	GetKubeEndpointFilters(endpointFilters [][]KubeEndpointFilter) FilterBundle
    ... 
}
```

When using the component, you must first convert your workload entity (container, pod, endpoint, etc.) into the `workloadfilter` version of the object which implements the `Filterable` interface. Second, you must select which filters should be used in your evaluation to make a filter bundle. With the `FilterBundle` you've generated, you will then be able to get the result of the filter by providing your `Filterable` object into the `IsExcluded` method.

## Usage Examples

### Basic Filtering

```go
import (
    workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
    workloadmetafilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/util/workloadmeta"
)

// Create a filterable container object
filterableContainer := workloadmetafilter.CreateContainer(wmetaContainer)

// Define filter groups example (in order of precedence)
selectedFilterGroups := [][]workloadfilter.ContainerFilter{
    // High precedence: Check autodiscovery annotations first
    {workloadfilter.ContainerADAnnotations},
    
    // Medium precedence: Check legacy AC filters
    {workloadfilter.LegacyContainerACInclude, workloadfilter.LegacyContainerACExclude},
    
    // Low precedence: Check global filters
    {workloadfilter.LegacyContainerGlobal},
}
filterBundle := filterStore.GetContainerFilters(selectedFilterGroups)

// Check if container should be excluded
if filterBundle.IsExcluded(filterableContainer) {
    // Container is excluded, skip processing
    return
}

// Container is included, proceed with processing
```

## Filter Precedence

Filters are evaluated in the order they appear in filter priority groups. The evaluation stops as soon as a priority group produces a definitive result (Include or Exclude). 
Within the same priority level, inclusion takes precedence over exclusion.

### Evaluation Logic

For each filter group:
1. If any filter in the group returns `Include` → immediately return `Include`
2. If any filter in the group returns `Exclude` → record group result as `Exclude`, proceed evaluating other filters in group
3. If group result is `Exclude` → return `Exclude`
4. If group result is `Unknown` → continue to next group
5. If all groups return `Unknown` → return `Unknown` (typically treated as not excluded)

### Example In Practice

Datadog Agent configuration `DD_CONTAINER_INCLUDE: "name:nginx"` maps to the `ContainerLegacyGlobal` filter. Meanwhile the value from the pod annotation `ad.datadoghq.com/<container_name>.exclude` is used for `ContainerADAnnotations`.

Pod and Container Definition:
```
kind: Pod
metadata:
  name: web-app
  namespace: production
  annotations:
    ad.datadoghq.com/nginx.exclude: "true"
...
spec:
  containers:
  - name: nginx
    image: nginx:latest
```

With the configuration and workload defined above, the queries below would have the following results:

```
// 1. The container's pod annotations excludes the container.
filterBundle := filterStore.GetContainerFilters({{ContainerADAnnotations}})
filterBundle.IsExcluded(filterableContainer) == true

// 2.The legacy global container filter includes containers named like `nginx`.
filterBundle := filterStore.GetContainerFilters({{ContainerLegacyGlobal}})
filterStore.IsExcluded(filterableContainer) == false

// 3. The container's pod annotations excludes the container. ADAnnotations filter is higher precedence so excluded.
filterBundle := filterStore.GetContainerFilters({{ContainerADAnnotations}, {ContainerLegacyGlobal}})
filterBundle.IsExcluded(filterableContainer) == true

// 4. The legacy global container filter includes containers named like `nginx`. LegacyGlobal is higher precedence so included.
filterBundle := filterStore.GetContainerFilters({{ContainerLegacyGlobal}, {ContainerADAnnotations}})
filterBundle.IsExcluded(filterableContainer) == false

// 5. The ContainerADAnnotations filter evaluates to exclude, however, ContainerLegacyGlobal evalutes to include.
//    Within the same group, inclusion takes higher precedence over exclusion, and thus the container is not excluded.
filterBundle := filterStore.GetContainerFilters({{ContainerADAnnotations, ContainerLegacyGlobal}})
filterBundle.IsExcluded(filterableContainer) == false

// 6. There are no filters defined. Requires explicit exclusion to be excluded. Thus container is included.
filterBundle := filterStore.GetContainerFilters({{}})
filterBundle.IsExcluded(filterableContainer, nil) == false
```

## Error Handling

The component provides methods to check for filter initialization errors:

```go
// Check for filter initialization errors
filterSelection := []workloadfilter.ContainerFilter{
    workloadfilter.LegacyContainerMetrics,
    workloadfilter.LegacyContainerLogs,
}
filterBundle := filterStore.GetContainerFilters(filterSelection)

errors := filterBundle.GetErrors()
for _, err := range errors {
    log.Warnf("Filter initialization error: %v", err)
}
```

Common error scenarios:
- Invalid regex patterns in filter configurations
- CEL expression compilation errors

## Defining a New Filter

Adding a new filter to the workloadfilter system involves several steps.

### Step 1: Define the Filter Type

Each exposed filter is specific to a particular resource type. This will be the filter key identifier which clients will use to request that specific filter to be used in their query. For example:

```go
// In comp/core/workloadfilter/def/types.go
type ContainerFilter int

const (
    // ...existing filters...
    LegacyContainerGlobal ContainerFilter = iota
    ContainerADAnnotations
    
    // Add your new filter here
    MyCustomContainerFilter
)

...
// Example usage:
selectedFilterGroups := [][]workloadfilter.ContainerFilter{
    {workloadfilter.ContainerADAnnotations},
    {workloadfilter.LegacyContainerGlobal, workloadfilter.MyCustomContainerFilter},
}
filterBundle := filterStore.GetContainerFilters(selectedFilterGroups)
filterBundle.IsExcluded(container)
```

### Step 2: Define the Filter Program in the Catalog

Create a new filter program that implements the filtering logic. For example:

```go
// In comp/core/workloadfilter/catalog/container.go

import (
    "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
    "github.com/DataDog/datadog-agent/comp/core/workloadfilter/impl/filter"
    "github.com/DataDog/datadog-agent/pkg/config"
)

// MyCustomFilterProgram implements filtering based on custom logic.
// This implements the `FilterProgram` interface with `Evaluate` and `GetInitializationErrors`
type MyCustomFilterProgram struct {
    // Add any configuration fields needed
    patterns []string
    enabled  bool
}

// NewMyCustomFilterProgram creates a new custom filter program
func NewMyCustomFilterProgram(cfg config.Component) (FilterProgram, error) {
    patterns := cfg.GetStringSlice("my_custom_filter.patterns")
    enabled := cfg.GetBool("my_custom_filter.enabled")
    
    return &MyCustomFilterProgram{
        patterns: patterns,
        enabled:  enabled,
    }, nil
}
```

### Step 3: Register the Filter from the Catalog

Add your filter into the store from the filter catalog:

```go
// In comp/core/workloadfilter/impl/filter.go
func newFilter(config config.Component, logger log.Component) (workloadfilter.Component, error) {
	filter := &filter{
		config:    config,
		log:       logger,
		prgs:      make(map[workloadfilter.ResourceType]map[int]program.FilterProgram),
	}

	// Register your custom container filter
	filter.registerProgram(
        workloadfilter.ContainerType,
        int(workloadfilter.MyCustomContainerFilter),
        catalog.LegacyContainerMetricsProgram(config)
    )
    ...
}
```