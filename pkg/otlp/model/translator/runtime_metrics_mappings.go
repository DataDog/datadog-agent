// Copyright The OpenTelemetry Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package translator

// runtimeMetricsMappings defines the mappings from OTel runtime metric names to their
// equivalent Datadog runtime metric names
var runtimeMetricsMappings = map[string]string{
	"process.runtime.go.goroutines":        "runtime.go.num_goroutine",
	"process.runtime.go.cgo.calls":         "runtime.go.num_cgo_call",
	"process.runtime.go.lookups":           "runtime.go.mem_stats.lookups",
	"process.runtime.go.mem.heap_alloc":    "runtime.go.mem_stats.heap_alloc",
	"process.runtime.go.mem.heap_sys":      "runtime.go.mem_stats.heap_sys",
	"process.runtime.go.mem.heap_idle":     "runtime.go.mem_stats.heap_idle",
	"process.runtime.go.mem.heap_inuse":    "runtime.go.mem_stats.heap_inuse",
	"process.runtime.go.mem.heap_released": "runtime.go.mem_stats.heap_released",
	"process.runtime.go.mem.heap_objects":  "runtime.go.mem_stats.heap_objects",
	"process.runtime.go.gc.pause_total_ns": "runtime.go.mem_stats.pause_total_ns",
	"process.runtime.go.gc.count":          "runtime.go.mem_stats.num_gc",
}
