// Copyright 2025 Datadog, Inc.
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

/**
 * @file fgm_observer.h
 * @brief C FFI interface for the FGM observer library
 *
 * This header provides C-compatible function declarations for collecting
 * fine-grained container metrics from Linux cgroups and procfs.
 */

#ifndef FGM_OBSERVER_H
#define FGM_OBSERVER_H

#ifdef __cplusplus
extern "C" {
#endif

/**
 * @brief Callback function type for metric emission
 *
 * Called once for each metric sampled from a container.
 *
 * @param name Metric name (null-terminated string)
 * @param value Metric value (floating point)
 * @param tags_json JSON array of tags in "key:value" format (e.g., ["app:web", "env:prod"])
 * @param timestamp_ms Timestamp in milliseconds since Unix epoch
 * @param ctx Opaque context pointer (passed through from fgm_sample_container)
 */
typedef void (*fgm_metric_callback)(
    const char* name,
    double value,
    const char* tags_json,
    long long timestamp_ms,
    void* ctx
);

/**
 * @brief Initialize the FGM observer library
 *
 * Must be called before any sampling operations. Creates a Tokio runtime
 * for async operations.
 *
 * @return 0 on success, 1 if already initialized, -1 on failure
 */
int fgm_init(void);

/**
 * @brief Shutdown the FGM observer library
 *
 * Cleans up resources. No sampling operations should be performed after
 * calling this function.
 */
void fgm_shutdown(void);

/**
 * @brief Sample metrics for a single container
 *
 * Reads cgroup v2 and procfs metrics, calling the provided callback for
 * each metric. This is a blocking call.
 *
 * Metrics emitted include:
 * - Memory: container.memory.current, container.memory.anon, container.memory.file, etc.
 * - CPU: container.cpu.usage_usec, container.cpu.user_usec, container.cpu.system_usec, etc.
 * - PSI: container.memory.pressure.some.avg10, container.cpu.pressure.full.total, etc.
 * - Procfs: container.memory.pss, container.memory.rss, container.memory.swap (if pid > 0)
 *
 * @param cgroup_path Absolute path to container's cgroup directory (e.g., "/sys/fs/cgroup/system.slice/docker-abc123.scope")
 * @param pid Container's main PID (for procfs reads, 0 to skip)
 * @param callback Function to call for each metric
 * @param ctx Opaque context pointer passed to callback
 *
 * @return 0 on success, -1 if not initialized, -2 if invalid parameters, -3 if sampling failed
 */
int fgm_sample_container(
    const char* cgroup_path,
    int pid,
    fgm_metric_callback callback,
    void* ctx
);

#ifdef __cplusplus
}
#endif

#endif // FGM_OBSERVER_H
