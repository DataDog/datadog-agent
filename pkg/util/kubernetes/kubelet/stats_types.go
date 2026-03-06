// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

package kubelet

// This file contains local copies of the Kubernetes kubelet stats/summary API types.
// Copied from k8s.io/kubelet/pkg/apis/stats/v1alpha1/types.go (Apache 2.0 licensed).
// The only change is replacing metav1.Time with the local Time type already defined
// in kubelet.go.
//
// Upstream source: https://github.com/kubernetes/kubelet/blob/master/pkg/apis/stats/v1alpha1/types.go

// Summary is a top-level container for holding NodeStats and PodStats.
type Summary struct {
	// Overall node stats.
	Node NodeStats `json:"node"`
	// Per-pod stats.
	Pods []PodStats `json:"pods"`
}

// NodeStats holds node-level unprocessed sample stats.
type NodeStats struct {
	// Reference to the measured Node.
	NodeName string `json:"nodeName"`
	// Stats of system daemons tracked as raw containers.
	SystemContainers []ContainerStats `json:"systemContainers,omitempty"`
	// The time at which data collection for the node-scoped stats was (re)started.
	StartTime Time `json:"startTime"`
	// Stats pertaining to CPU resources.
	CPU *CPUStats `json:"cpu,omitempty"`
	// Stats pertaining to memory (RAM) resources.
	Memory *MemoryStats `json:"memory,omitempty"`
	// Stats pertaining to IO resources.
	IO *IOStats `json:"io,omitempty"`
	// Stats pertaining to network resources.
	Network *NetworkStats `json:"network,omitempty"`
	// Stats pertaining to total usage of filesystem resources on the rootfs used by node k8s components.
	Fs *FsStats `json:"fs,omitempty"`
	// Stats about the underlying container runtime.
	Runtime *RuntimeStats `json:"runtime,omitempty"`
	// Stats about the rlimit of system.
	Rlimit *RlimitStats `json:"rlimit,omitempty"`
	// Stats pertaining to swap resources.
	Swap *SwapStats `json:"swap,omitempty"`
}

// RlimitStats are stats rlimit of OS.
type RlimitStats struct {
	Time                  Time   `json:"time"`
	MaxPID                *int64 `json:"maxpid,omitempty"`
	NumOfRunningProcesses *int64 `json:"curproc,omitempty"`
}

// RuntimeStats are stats pertaining to the underlying container runtime.
type RuntimeStats struct {
	ImageFs     *FsStats `json:"imageFs,omitempty"`
	ContainerFs *FsStats `json:"containerFs,omitempty"`
}

// ProcessStats are stats pertaining to processes.
type ProcessStats struct {
	ProcessCount *uint64 `json:"process_count,omitempty"`
}

// PodStats holds pod-level unprocessed sample stats.
type PodStats struct {
	// Reference to the measured Pod.
	PodRef PodReference `json:"podRef"`
	// The time at which data collection for the pod-scoped stats was (re)started.
	StartTime Time `json:"startTime"`
	// Stats of containers in the measured pod.
	Containers []ContainerStats `json:"containers"`
	// Stats pertaining to CPU resources consumed by pod cgroup.
	CPU *CPUStats `json:"cpu,omitempty"`
	// Stats pertaining to memory resources consumed by pod cgroup.
	Memory *MemoryStats `json:"memory,omitempty"`
	// Stats pertaining to IO resources consumed by pod cgroup.
	IO *IOStats `json:"io,omitempty"`
	// Stats pertaining to network resources.
	Network *NetworkStats `json:"network,omitempty"`
	// Stats pertaining to volume usage of filesystem resources.
	VolumeStats []VolumeStats `json:"volume,omitempty"`
	// EphemeralStorage reports the total filesystem usage for containers and emptyDir-backed volumes.
	EphemeralStorage *FsStats `json:"ephemeral-storage,omitempty"`
	// ProcessStats pertaining to processes.
	ProcessStats *ProcessStats `json:"process_stats,omitempty"`
	// Stats pertaining to swap resources.
	Swap *SwapStats `json:"swap,omitempty"`
}

// ContainerStats holds container-level unprocessed sample stats.
type ContainerStats struct {
	// Reference to the measured container.
	Name string `json:"name"`
	// The time at which data collection for this container was (re)started.
	StartTime Time `json:"startTime"`
	// Stats pertaining to CPU resources.
	CPU *CPUStats `json:"cpu,omitempty"`
	// Stats pertaining to memory (RAM) resources.
	Memory *MemoryStats `json:"memory,omitempty"`
	// Stats pertaining to IO resources.
	IO *IOStats `json:"io,omitempty"`
	// Metrics for Accelerators.
	Accelerators []AcceleratorStats `json:"accelerators,omitempty"`
	// Stats pertaining to container rootfs usage of filesystem resources.
	Rootfs *FsStats `json:"rootfs,omitempty"`
	// Stats pertaining to container logs usage of filesystem resources.
	Logs *FsStats `json:"logs,omitempty"`
	// User defined metrics that are exposed by containers in the pod.
	UserDefinedMetrics []UserDefinedMetric `json:"userDefinedMetrics,omitempty"`
	// Stats pertaining to swap resources.
	Swap *SwapStats `json:"swap,omitempty"`
}

// PodReference contains enough information to locate the referenced pod.
type PodReference struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	UID       string `json:"uid"`
}

// InterfaceStats contains resource value data about interface.
type InterfaceStats struct {
	Name     string  `json:"name"`
	RxBytes  *uint64 `json:"rxBytes,omitempty"`
	RxErrors *uint64 `json:"rxErrors,omitempty"`
	TxBytes  *uint64 `json:"txBytes,omitempty"`
	TxErrors *uint64 `json:"txErrors,omitempty"`
}

// NetworkStats contains data about network resources.
type NetworkStats struct {
	// The time at which these stats were updated.
	Time Time `json:"time"`
	// Stats for the default interface, if found
	InterfaceStats `json:",inline"`
	Interfaces     []InterfaceStats `json:"interfaces,omitempty"`
}

// CPUStats contains data about CPU usage.
type CPUStats struct {
	Time                 Time    `json:"time"`
	UsageNanoCores       *uint64 `json:"usageNanoCores,omitempty"`
	UsageCoreNanoSeconds *uint64 `json:"usageCoreNanoSeconds,omitempty"`
	PSI                  *PSIStats `json:"psi,omitempty"`
}

// MemoryStats contains data about memory usage.
type MemoryStats struct {
	Time            Time    `json:"time"`
	AvailableBytes  *uint64 `json:"availableBytes,omitempty"`
	UsageBytes      *uint64 `json:"usageBytes,omitempty"`
	WorkingSetBytes *uint64 `json:"workingSetBytes,omitempty"`
	RSSBytes        *uint64 `json:"rssBytes,omitempty"`
	PageFaults      *uint64 `json:"pageFaults,omitempty"`
	MajorPageFaults *uint64 `json:"majorPageFaults,omitempty"`
	PSI             *PSIStats `json:"psi,omitempty"`
}

// IOStats contains data about IO usage.
type IOStats struct {
	Time Time      `json:"time"`
	PSI  *PSIStats `json:"psi,omitempty"`
}

// PSIStats contains PSI statistics for an individual resource.
type PSIStats struct {
	Full PSIData `json:"full"`
	Some PSIData `json:"some"`
}

// PSIData contains PSI data for an individual resource.
type PSIData struct {
	Total  uint64  `json:"total"`
	Avg10  float64 `json:"avg10"`
	Avg60  float64 `json:"avg60"`
	Avg300 float64 `json:"avg300"`
}

// SwapStats contains data about swap memory usage.
type SwapStats struct {
	Time               Time    `json:"time"`
	SwapAvailableBytes *uint64 `json:"swapAvailableBytes,omitempty"`
	SwapUsageBytes     *uint64 `json:"swapUsageBytes,omitempty"`
}

// AcceleratorStats contains stats for accelerators attached to the container.
type AcceleratorStats struct {
	Make       string `json:"make"`
	Model      string `json:"model"`
	ID         string `json:"id"`
	MemoryTotal uint64 `json:"memoryTotal"`
	MemoryUsed  uint64 `json:"memoryUsed"`
	DutyCycle   uint64 `json:"dutyCycle"`
}

// VolumeStats contains data about Volume filesystem usage.
type VolumeStats struct {
	FsStats `json:",inline"`
	Name    string        `json:"name,omitempty"`
	PVCRef  *PVCReference `json:"pvcRef,omitempty"`
	VolumeHealthStats *VolumeHealthStats `json:"volumeHealthStats,omitempty"`
}

// VolumeHealthStats contains data about volume health.
type VolumeHealthStats struct {
	Abnormal bool `json:"abnormal"`
}

// PVCReference contains enough information to describe the referenced PVC.
type PVCReference struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

// FsStats contains data about filesystem usage.
type FsStats struct {
	Time           Time    `json:"time"`
	AvailableBytes *uint64 `json:"availableBytes,omitempty"`
	CapacityBytes  *uint64 `json:"capacityBytes,omitempty"`
	UsedBytes      *uint64 `json:"usedBytes,omitempty"`
	InodesFree     *uint64 `json:"inodesFree,omitempty"`
	Inodes         *uint64 `json:"inodes,omitempty"`
	InodesUsed     *uint64 `json:"inodesUsed,omitempty"`
}

// UserDefinedMetricType defines how the metric should be interpreted by the user.
type UserDefinedMetricType string

const (
	// MetricGauge is an instantaneous value. May increase or decrease.
	MetricGauge UserDefinedMetricType = "gauge"
	// MetricCumulative is a counter-like value that is only expected to increase.
	MetricCumulative UserDefinedMetricType = "cumulative"
	// MetricDelta is a rate over a time period.
	MetricDelta UserDefinedMetricType = "delta"
)

// UserDefinedMetricDescriptor contains metadata that describes a user defined metric.
type UserDefinedMetricDescriptor struct {
	Name   string                `json:"name"`
	Type   UserDefinedMetricType `json:"type"`
	Units  string                `json:"units"`
	Labels map[string]string     `json:"labels,omitempty"`
}

// UserDefinedMetric represents a metric defined and generated by users.
type UserDefinedMetric struct {
	UserDefinedMetricDescriptor `json:",inline"`
	Time                        Time    `json:"time"`
	Value                       float64 `json:"value"`
}
