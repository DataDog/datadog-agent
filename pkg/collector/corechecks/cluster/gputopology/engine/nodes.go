// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package engine

import (
	"slices"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/gpu"
)

// labelGPUProduct is set by NVIDIA GPU Feature Discovery. The MIG rule
// deliberately ignores nvidia.com/mig.strategy: GFD sets it on every GPU
// node, to "none" when MIG is off.
const (
	labelGPUProduct         = "nvidia.com/gpu.product"
	labelGPUReplicas        = "nvidia.com/gpu.replicas"
	labelGPUSharingStrategy = "nvidia.com/gpu.sharing-strategy"
	migResourcePfx          = string(gpu.GpuNvidiaMigPrefix) + "-"
)

// wholeGPUResources are the extended resources the engine models: one unit
// is one whole GPU.
var wholeGPUResources = []string{
	string(gpu.GpuNvidiaGeneric),
	string(gpu.GpuAMD),
	string(gpu.GpuIntelXe),
	string(gpu.GpuInteli915),
}

// presenceLabels are node labels (set to "true") that show a node has GPUs
// even when the device plugin is gone: kubelet drops the resource from
// capacity and allocatable about 5 minutes after the plugin dies. Each maps
// to the resource the node is attributed to.
var presenceLabels = []struct{ label, resource string }{
	{"nvidia.com/gpu.present", string(gpu.GpuNvidiaGeneric)},           // NVIDIA GFD
	{"feature.node.kubernetes.io/amd-gpu", string(gpu.GpuAMD)},         // AMD GPU operator NFD rule
	{"intel.feature.node.kubernetes.io/gpu", string(gpu.GpuInteli915)}, // Intel device plugins NFD rule
}

// isGPUResourceName reports whether name is any GPU extended resource,
// modelled or not (MIG slices, shared GPUs).
func isGPUResourceName(name string) bool {
	return slices.Contains(wholeGPUResources, name) || gpu.IsNvidiaKubernetesResource(name)
}

// NodeState is the state of a node in the topology. Each node has exactly
// one, see doc.go for the precedence.
type NodeState int

// Node states, in reporting order.
const (
	NodeInTopology NodeState = iota
	NodeUnavailable
	NodeTainted
	NodeNoGPUAllocatable
	NodeMissingLabels
	NodeUnsupported
)

// NodeStates returns all node states in reporting order.
func NodeStates() []NodeState {
	return []NodeState{NodeInTopology, NodeUnavailable, NodeTainted, NodeNoGPUAllocatable, NodeMissingLabels, NodeUnsupported}
}

func (s NodeState) String() string {
	switch s {
	case NodeInTopology:
		return "in_topology"
	case NodeUnavailable:
		return "unavailable"
	case NodeTainted:
		return "tainted"
	case NodeNoGPUAllocatable:
		return "no_gpu_allocatable"
	case NodeMissingLabels:
		return "missing_labels"
	case NodeUnsupported:
		return "unsupported"
	}
	return "unknown"
}

// Modes of unsupported nodes, in NodeCount.SlicingMode and SharingMode.
const (
	SlicingModeMIG         = "mig"          // any MIG strategy
	SharingModeTimeSlicing = "time_slicing" // incl. the nvidia.com/gpu.shared rename
	SharingModeMPS         = "mps"

	// sharedResource is the resource the NVIDIA device plugin advertises
	// shared GPUs under with renameByDefault.
	sharedResource = string(gpu.GpuNvidiaGenericShared)
)

const (
	// GPUTypeUnknown is the GPU type of nodes without a parseable GPU type
	// label (see gpuTypeLabels).
	GPUTypeUnknown = "unknown"
	// GPUResourceNone is not produced by the engine: the check tags its zero
	// node counts with it when Result.Nodes is empty.
	GPUResourceNone = "none"
)

// classified is a node after classification.
type classified struct {
	node Node
	// gpuTypes holds the GPU type per key of states.
	gpuTypes map[string]string
	// states holds the node's state per attributed resource. It only differs
	// between resources for no_gpu_allocatable (e.g. one vendor's plugin
	// down). An unsupported node has a single entry.
	states map[string]NodeState
	// slicing and sharing are the modes of an unsupported node.
	slicing, sharing string
	path             []string // label values for every level, nil if missing_labels or unsupported
}

// classify returns the node's states, or false for a node with no GPU at all.
func classify(n Node, levels []string, implicit bool, tols []Toleration) (classified, bool) {
	c := classified{node: n, states: map[string]NodeState{}, gpuTypes: map[string]string{}}
	if r, slicing, sharing := unsupported(n); r != "" {
		c.states[r] = NodeUnsupported
		c.gpuTypes[r] = gpuType(n, r)
		c.slicing, c.sharing = slicing, sharing
		return c, true
	}
	resources := attributedResources(n)
	if len(resources) == 0 {
		return c, false
	}
	path, labelled := nodePath(n, levels, implicit)
	if labelled {
		c.path = path
	}
	ok := tolerated(n.Taints, tols)
	for _, r := range resources {
		c.gpuTypes[r] = gpuType(n, r)
		switch {
		case !labelled:
			c.states[r] = NodeMissingLabels
		case !n.Ready || n.Unschedulable:
			c.states[r] = NodeUnavailable
		case n.Allocatable[r] <= 0:
			c.states[r] = NodeNoGPUAllocatable
		case !ok:
			c.states[r] = NodeTainted
		default:
			c.states[r] = NodeInTopology
		}
	}
	return c, true
}

// unsupported returns, for a node whose GPUs v1 doesn't model, the resource
// it is attributed to and its slicing or sharing mode; or "" for other nodes.
// MIG is checked first.
//   - MIG is active: any nvidia.com/mig-* capacity (mixed strategy), or a
//     -MIG- product label (single strategy, incl. -MIG-INVALID).
//   - GPUs are shared (time-slicing or MPS): nvidia.com/gpu.shared in
//     capacity (renameByDefault), or the GFD labels gpu.replicas > 1,
//     gpu.sharing-strategy time-slicing/mps, or a -SHARED product suffix. By
//     default the plugin advertises nvidia.com/gpu = GPUs x replicas, which
//     would inflate capacity, free GPUs and slots.
//   - MIG nodes are attributed to nvidia.com/gpu, with slicing mode "mig".
//   - Shared nodes are attributed to nvidia.com/gpu.shared when renamed,
//     else nvidia.com/gpu, with sharing mode "mps" when the strategy label
//     says so, else "time_slicing".
func unsupported(n Node) (resource, slicing, sharing string) {
	nvidia := string(gpu.GpuNvidiaGeneric)
	for r, c := range n.Capacity {
		if c > 0 && strings.HasPrefix(r, migResourcePfx) {
			return nvidia, SlicingModeMIG, ""
		}
	}
	product := n.Labels[labelGPUProduct]
	if strings.Contains(product, "-MIG-") {
		return nvidia, SlicingModeMIG, ""
	}
	strategy := n.Labels[labelGPUSharingStrategy]
	replicas, err := strconv.Atoi(n.Labels[labelGPUReplicas])
	_, renamed := n.Capacity[sharedResource]
	if !renamed && !(err == nil && replicas > 1) && strategy != "time-slicing" && strategy != "mps" && !strings.HasSuffix(product, "-SHARED") {
		return "", "", ""
	}
	resource, sharing = nvidia, SharingModeTimeSlicing
	if renamed {
		resource = sharedResource
	}
	if strategy == "mps" {
		sharing = SharingModeMPS
	}
	return resource, "", sharing
}

// GPU vendors, to pick the GPU type labels of a resource.
const (
	vendorNVIDIA = "nvidia"
	vendorAMD    = "amd"
)

// gpuTypeLabels are the node labels the GPU type is read from, in order: the
// first one of the resource's vendor present with a parseable value wins.
// Each value is turned into a device name that gpu.ExtractGPUType parses, so
// the type matches the gpu_type tag of the node GPU check. There is no Intel
// label: Intel resources are always "unknown".
var gpuTypeLabels = []struct {
	vendor   string
	label    string
	toDevice func(value string, labels map[string]string) string
}{
	// NVIDIA GPU Feature Discovery, e.g. NVIDIA-H100-80GB-HBM3.
	{vendorNVIDIA, labelGPUProduct, func(v string, _ map[string]string) string { return gpu.GFDLabelToGPUDeviceName(v) }},
	// GKE without the GPU Operator, e.g. nvidia-tesla-a100, nvidia-h100-80gb.
	{vendorNVIDIA, "cloud.google.com/gke-accelerator", func(v string, _ map[string]string) string {
		v = strings.TrimPrefix(v, "nvidia-tesla-")
		return "nvidia " + strings.TrimPrefix(v, "nvidia-")
	}},
	// EKS with Karpenter, e.g. h100, a10g, only when the manufacturer label
	// says nvidia.
	{vendorNVIDIA, "karpenter.k8s.aws/instance-gpu-name", func(v string, labels map[string]string) string {
		if !strings.EqualFold(labels["karpenter.k8s.aws/instance-gpu-manufacturer"], "nvidia") {
			return ""
		}
		return "nvidia " + v
	}},
	// AMD GPU node labeller, e.g. AMD_Instinct_MI300X or Instinct_MI300X.
	{vendorAMD, "amd.com/gpu.product-name", func(v string, _ map[string]string) string {
		if strings.HasPrefix(strings.ToLower(v), "amd") {
			return v
		}
		return "amd " + v
	}},
}

// vendorOf returns the vendor of a resource or unsupported attribution.
func vendorOf(resource string) string {
	switch {
	case strings.HasPrefix(resource, "nvidia.com/"):
		return vendorNVIDIA
	case strings.HasPrefix(resource, "amd.com/"):
		return vendorAMD
	}
	return ""
}

// gpuType returns the GPU type of a node's resource from the first parseable
// gpuTypeLabels label of the resource's vendor, or GPUTypeUnknown.
func gpuType(n Node, resource string) string {
	vendor := vendorOf(resource)
	for _, l := range gpuTypeLabels {
		v, ok := n.Labels[l.label]
		if l.vendor != vendor || !ok || v == "" {
			continue
		}
		if t := gpu.ExtractGPUType(l.toDevice(v, n.Labels)); t != "" {
			return t
		}
	}
	return GPUTypeUnknown
}

// attributedResources returns the modelled GPU resources present in the
// node's capacity at any value, or else those of its presence labels.
func attributedResources(n Node) []string {
	var res []string
	for _, r := range wholeGPUResources {
		if _, ok := n.Capacity[r]; ok {
			res = append(res, r)
		}
	}
	if len(res) == 0 {
		for _, p := range presenceLabels {
			if n.Labels[p.label] == "true" {
				res = append(res, p.resource)
			}
		}
	}
	slices.Sort(res)
	return slices.Compact(res)
}

// nodePath returns the node's label values for every level. The implicit
// host level (last) uses the node name.
func nodePath(n Node, levels []string, implicit bool) ([]string, bool) {
	path := make([]string, len(levels))
	for i, label := range levels {
		if implicit && i == len(levels)-1 {
			path[i] = n.Name
			continue
		}
		v, ok := n.Labels[label]
		if !ok {
			return nil, false
		}
		path[i] = v
	}
	return path, true
}

// Taint effects and toleration operators, with their Kubernetes values.
const (
	EffectNoSchedule       = "NoSchedule"
	EffectPreferNoSchedule = "PreferNoSchedule"
	EffectNoExecute        = "NoExecute"
	OpExists               = "Exists"
	OpEqual                = "Equal"
)

// Taint is a node taint.
type Taint struct {
	Key    string
	Value  string
	Effect string
}

// Toleration is a flavor or assumed toleration.
type Toleration struct {
	Key      string
	Operator string // OpExists or OpEqual (default)
	Value    string
	Effect   string // empty matches all effects
}

// tolerates follows Kubernetes ToleratesTaint.
func (t Toleration) tolerates(taint Taint) bool {
	if t.Effect != "" && t.Effect != taint.Effect {
		return false
	}
	if t.Key == "" {
		// The API rejects an empty key with Equal, so only Exists matches all.
		return t.Operator == OpExists
	}
	if t.Key != taint.Key {
		return false
	}
	switch t.Operator {
	case OpExists:
		return true
	case "", OpEqual:
		return t.Value == taint.Value
	}
	return false
}

// tolerated reports whether every NoSchedule/NoExecute taint is tolerated.
// A taint keyed on a GPU resource name is always tolerated: the
// ExtendedResourceToleration admission plugin adds that toleration to pods
// requesting the resource (GKE taints GPU pools nvidia.com/gpu=present).
func tolerated(taints []Taint, tols []Toleration) bool {
	for _, taint := range taints {
		if taint.Effect != EffectNoSchedule && taint.Effect != EffectNoExecute {
			continue
		}
		if isGPUResourceName(taint.Key) {
			continue
		}
		if !slices.ContainsFunc(tols, func(t Toleration) bool { return t.tolerates(taint) }) {
			return false
		}
	}
	return true
}
