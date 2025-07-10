// Copyright The OpenTelemetry Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package metrics provides runtime metric mappings.
package metrics

// runtimeMetricPrefixLanguageMap defines the runtime metric prefixes and which languages they map to
var runtimeMetricPrefixLanguageMap = map[string]string{
	"process.runtime.go":     "go",
	"process.runtime.dotnet": "dotnet",
	"process.runtime.jvm":    "jvm",
	"jvm":                    "jvm",
}

// runtimeMetricMapping defines the fields needed to map OTel runtime metrics to their equivalent
// Datadog runtime metrics
type runtimeMetricMapping struct {
	mappedName string                   // the Datadog runtime metric name
	attributes []runtimeMetricAttribute // the attribute(s) this metric originates from
}

// runtimeMetricAttribute defines the structure for an attribute in regard to mapping runtime metrics.
// The presence of a runtimeMetricAttribute means that a metric must be mapped from a data point
// with the given attribute(s).
type runtimeMetricAttribute struct {
	key    string   // the attribute name
	values []string // the attribute value, or multiple values if there is more than one value for the same mapping
}

// runtimeMetricMappingList defines the structure for a list of runtime metric mappings where the key
// represents the OTel metric name and the runtimeMetricMapping contains the Datadog metric name
type runtimeMetricMappingList map[string][]runtimeMetricMapping

var goRuntimeMetricsMappings = runtimeMetricMappingList{
	"process.runtime.go.goroutines":        {{mappedName: "runtime.go.num_goroutine"}},
	"process.runtime.go.cgo.calls":         {{mappedName: "runtime.go.num_cgo_call"}},
	"process.runtime.go.lookups":           {{mappedName: "runtime.go.mem_stats.lookups"}},
	"process.runtime.go.mem.heap_alloc":    {{mappedName: "runtime.go.mem_stats.heap_alloc"}},
	"process.runtime.go.mem.heap_sys":      {{mappedName: "runtime.go.mem_stats.heap_sys"}},
	"process.runtime.go.mem.heap_idle":     {{mappedName: "runtime.go.mem_stats.heap_idle"}},
	"process.runtime.go.mem.heap_inuse":    {{mappedName: "runtime.go.mem_stats.heap_inuse"}},
	"process.runtime.go.mem.heap_released": {{mappedName: "runtime.go.mem_stats.heap_released"}},
	"process.runtime.go.mem.heap_objects":  {{mappedName: "runtime.go.mem_stats.heap_objects"}},
	"process.runtime.go.gc.pause_total_ns": {{mappedName: "runtime.go.mem_stats.pause_total_ns"}},
	"process.runtime.go.gc.count":          {{mappedName: "runtime.go.mem_stats.num_gc"}},
}

var dotnetRuntimeMetricsMappings = runtimeMetricMappingList{
	"process.runtime.dotnet.monitor.lock_contention.count": {{mappedName: "runtime.dotnet.threads.contention_count"}},
	"process.runtime.dotnet.exceptions.count":              {{mappedName: "runtime.dotnet.exceptions.count"}},
	"process.runtime.dotnet.gc.heap.size": {{
		mappedName: "runtime.dotnet.gc.size.gen0",
		attributes: []runtimeMetricAttribute{{
			key:    "generation",
			values: []string{"gen0"},
		}},
	}, {
		mappedName: "runtime.dotnet.gc.size.gen1",
		attributes: []runtimeMetricAttribute{{
			key:    "generation",
			values: []string{"gen1"},
		}},
	}, {
		mappedName: "runtime.dotnet.gc.size.gen2",
		attributes: []runtimeMetricAttribute{{
			key:    "generation",
			values: []string{"gen2"},
		}},
	}, {
		mappedName: "runtime.dotnet.gc.size.loh",
		attributes: []runtimeMetricAttribute{{
			key:    "generation",
			values: []string{"loh"},
		}},
	}},
	"process.runtime.dotnet.gc.collections.count": {{
		mappedName: "runtime.dotnet.gc.count.gen0",
		attributes: []runtimeMetricAttribute{{
			key:    "generation",
			values: []string{"gen0"},
		}},
	}, {
		mappedName: "runtime.dotnet.gc.count.gen1",
		attributes: []runtimeMetricAttribute{{
			key:    "generation",
			values: []string{"gen1"},
		}},
	}, {
		mappedName: "runtime.dotnet.gc.count.gen2",
		attributes: []runtimeMetricAttribute{{
			key:    "generation",
			values: []string{"gen2"},
		}},
	}},
}

var stableJavaRuntimeMetricsMappings = runtimeMetricMappingList{
	"jvm.thread.count":           {{mappedName: "jvm.thread_count"}},
	"jvm.class.count":            {{mappedName: "jvm.loaded_classes"}},
	"jvm.system.cpu.utilization": {{mappedName: "jvm.cpu_load.system"}},
	"jvm.cpu.recent_utilization": {{mappedName: "jvm.cpu_load.process"}},
	"jvm.memory.used": {{
		mappedName: "jvm.heap_memory",
		attributes: []runtimeMetricAttribute{{
			key:    "jvm.memory.type",
			values: []string{"heap"},
		}},
	}, {
		mappedName: "jvm.non_heap_memory",
		attributes: []runtimeMetricAttribute{{
			key:    "jvm.memory.type",
			values: []string{"non_heap"},
		}},
	}, {
		mappedName: "jvm.gc.old_gen_size",
		attributes: []runtimeMetricAttribute{{
			key:    "jvm.memory.pool.name",
			values: []string{"G1 Old Gen", "Tenured Gen", "PS Old Gen"},
		}, {
			key:    "jvm.memory.type",
			values: []string{"heap"},
		}},
	}, {
		mappedName: "jvm.gc.eden_size",
		attributes: []runtimeMetricAttribute{{
			key:    "jvm.memory.pool.name",
			values: []string{"G1 Eden Space", "Eden Space", "Par Eden Space", "PS Eden Space"},
		}, {
			key:    "jvm.memory.type",
			values: []string{"heap"},
		}},
	}, {
		mappedName: "jvm.gc.survivor_size",
		attributes: []runtimeMetricAttribute{{
			key:    "jvm.memory.pool.name",
			values: []string{"G1 Survivor Space", "Survivor Space", "Par Survivor Space", "PS Survivor Space"},
		}, {
			key:    "jvm.memory.type",
			values: []string{"heap"},
		}},
	}, {
		mappedName: "jvm.gc.metaspace_size",
		attributes: []runtimeMetricAttribute{{
			key:    "jvm.memory.pool.name",
			values: []string{"Metaspace"},
		}, {
			key:    "jvm.memory.type",
			values: []string{"non_heap"},
		}},
	}},
	"jvm.memory.committed": {{
		mappedName: "jvm.heap_memory_committed",
		attributes: []runtimeMetricAttribute{{
			key:    "jvm.memory.type",
			values: []string{"heap"},
		}},
	}, {
		mappedName: "jvm.non_heap_memory_committed",
		attributes: []runtimeMetricAttribute{{
			key:    "jvm.memory.type",
			values: []string{"non_heap"},
		}},
	}},
	"jvm.memory.init": {{
		mappedName: "jvm.heap_memory_init",
		attributes: []runtimeMetricAttribute{{
			key:    "jvm.memory.type",
			values: []string{"heap"},
		}},
	}, {
		mappedName: "jvm.non_heap_memory_init",
		attributes: []runtimeMetricAttribute{{
			key:    "jvm.memory.type",
			values: []string{"non_heap"},
		}},
	}},
	"jvm.memory.limit": {{
		mappedName: "jvm.heap_memory_max",
		attributes: []runtimeMetricAttribute{{
			key:    "jvm.memory.type",
			values: []string{"heap"},
		}},
	}, {
		mappedName: "jvm.non_heap_memory_max",
		attributes: []runtimeMetricAttribute{{
			key:    "jvm.memory.type",
			values: []string{"non_heap"},
		}},
	}},
	"jvm.buffer.memory.usage": {{
		mappedName: "jvm.buffer_pool.direct.used",
		attributes: []runtimeMetricAttribute{{
			key:    "jvm.buffer.pool.name",
			values: []string{"direct"},
		}},
	}, {
		mappedName: "jvm.buffer_pool.mapped.used",
		attributes: []runtimeMetricAttribute{{
			key:    "jvm.buffer.pool.name",
			values: []string{"mapped"},
		}},
	}},
	"jvm.buffer.count": {{
		mappedName: "jvm.buffer_pool.direct.count",
		attributes: []runtimeMetricAttribute{{
			key:    "jvm.buffer.pool.name",
			values: []string{"direct"},
		}},
	}, {
		mappedName: "jvm.buffer_pool.mapped.count",
		attributes: []runtimeMetricAttribute{{
			key:    "jvm.buffer.pool.name",
			values: []string{"mapped"},
		}},
	}},
	"jvm.buffer.memory.limit": {{
		mappedName: "jvm.buffer_pool.direct.limit",
		attributes: []runtimeMetricAttribute{{
			key:    "jvm.buffer.pool.name",
			values: []string{"direct"},
		}},
	}, {
		mappedName: "jvm.buffer_pool.mapped.limit",
		attributes: []runtimeMetricAttribute{{
			key:    "jvm.buffer.pool.name",
			values: []string{"mapped"},
		}},
	}},
}

var javaRuntimeMetricsMappings = runtimeMetricMappingList{
	"process.runtime.jvm.threads.count":          {{mappedName: "jvm.thread_count"}},
	"process.runtime.jvm.classes.current_loaded": {{mappedName: "jvm.loaded_classes"}},
	"process.runtime.jvm.system.cpu.utilization": {{mappedName: "jvm.cpu_load.system"}},
	"process.runtime.jvm.cpu.utilization":        {{mappedName: "jvm.cpu_load.process"}},
	"process.runtime.jvm.memory.usage": {{
		mappedName: "jvm.heap_memory",
		attributes: []runtimeMetricAttribute{{
			key:    "type",
			values: []string{"heap"},
		}},
	}, {
		mappedName: "jvm.non_heap_memory",
		attributes: []runtimeMetricAttribute{{
			key:    "type",
			values: []string{"non_heap"},
		}},
	}, {
		mappedName: "jvm.gc.old_gen_size",
		attributes: []runtimeMetricAttribute{{
			key:    "pool",
			values: []string{"G1 Old Gen", "Tenured Gen", "PS Old Gen"},
		}, {
			key:    "type",
			values: []string{"heap"},
		}},
	}, {
		mappedName: "jvm.gc.eden_size",
		attributes: []runtimeMetricAttribute{{
			key:    "pool",
			values: []string{"G1 Eden Space", "Eden Space", "Par Eden Space", "PS Eden Space"},
		}, {
			key:    "type",
			values: []string{"heap"},
		}},
	}, {
		mappedName: "jvm.gc.survivor_size",
		attributes: []runtimeMetricAttribute{{
			key:    "pool",
			values: []string{"G1 Survivor Space", "Survivor Space", "Par Survivor Space", "PS Survivor Space"},
		}, {
			key:    "type",
			values: []string{"heap"},
		}},
	}, {
		mappedName: "jvm.gc.metaspace_size",
		attributes: []runtimeMetricAttribute{{
			key:    "pool",
			values: []string{"Metaspace"},
		}, {
			key:    "type",
			values: []string{"non_heap"},
		}},
	}},
	"process.runtime.jvm.memory.committed": {{
		mappedName: "jvm.heap_memory_committed",
		attributes: []runtimeMetricAttribute{{
			key:    "type",
			values: []string{"heap"},
		}},
	}, {
		mappedName: "jvm.non_heap_memory_committed",
		attributes: []runtimeMetricAttribute{{
			key:    "type",
			values: []string{"non_heap"},
		}},
	}},
	"process.runtime.jvm.memory.init": {{
		mappedName: "jvm.heap_memory_init",
		attributes: []runtimeMetricAttribute{{
			key:    "type",
			values: []string{"heap"},
		}},
	}, {
		mappedName: "jvm.non_heap_memory_init",
		attributes: []runtimeMetricAttribute{{
			key:    "type",
			values: []string{"non_heap"},
		}},
	}},
	"process.runtime.jvm.memory.limit": {{
		mappedName: "jvm.heap_memory_max",
		attributes: []runtimeMetricAttribute{{
			key:    "type",
			values: []string{"heap"},
		}},
	}, {
		mappedName: "jvm.non_heap_memory_max",
		attributes: []runtimeMetricAttribute{{
			key:    "type",
			values: []string{"non_heap"},
		}},
	}},
	"process.runtime.jvm.buffer.usage": {{
		mappedName: "jvm.buffer_pool.direct.used",
		attributes: []runtimeMetricAttribute{{
			key:    "pool",
			values: []string{"direct"},
		}},
	}, {
		mappedName: "jvm.buffer_pool.mapped.used",
		attributes: []runtimeMetricAttribute{{
			key:    "pool",
			values: []string{"mapped"},
		}},
	}},
	"process.runtime.jvm.buffer.count": {{
		mappedName: "jvm.buffer_pool.direct.count",
		attributes: []runtimeMetricAttribute{{
			key:    "pool",
			values: []string{"direct"},
		}},
	}, {
		mappedName: "jvm.buffer_pool.mapped.count",
		attributes: []runtimeMetricAttribute{{
			key:    "pool",
			values: []string{"mapped"},
		}},
	}},
	"process.runtime.jvm.buffer.limit": {{
		mappedName: "jvm.buffer_pool.direct.limit",
		attributes: []runtimeMetricAttribute{{
			key:    "pool",
			values: []string{"direct"},
		}},
	}, {
		mappedName: "jvm.buffer_pool.mapped.limit",
		attributes: []runtimeMetricAttribute{{
			key:    "pool",
			values: []string{"mapped"},
		}},
	}},
}

func getRuntimeMetricsMappings() runtimeMetricMappingList {
	res := runtimeMetricMappingList{}
	for k, v := range goRuntimeMetricsMappings {
		res[k] = v
	}
	for k, v := range dotnetRuntimeMetricsMappings {
		res[k] = v
	}
	for k, v := range javaRuntimeMetricsMappings {
		res[k] = v
	}
	for k, v := range stableJavaRuntimeMetricsMappings {
		res[k] = v
	}
	return res
}

// runtimeMetricsMappings defines the mappings from OTel runtime metric names to their
// equivalent Datadog runtime metric names
var runtimeMetricsMappings = getRuntimeMetricsMappings()
