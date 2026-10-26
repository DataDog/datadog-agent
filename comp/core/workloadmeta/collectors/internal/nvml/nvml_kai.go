// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

package nvml

import (
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/api/resource"

	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/gpu/config/consts"
	gpucontainers "github.com/DataDog/datadog-agent/pkg/gpu/containers"
	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// kaiGPUGroupLabel is carried by both a KAI fractional pod and the
	// reservation pod holding the GPU it shares; its value is the GPU group.
	// A pod asking for fractions of several GPUs carries one
	// kaiGPUGroupLabelPrefix label per GPU instead of this one.
	kaiGPUGroupLabel = "runai-gpu-group"
	// kaiGPUGroupLabelPrefix prefixes the per-GPU form of the group label,
	// "runai-gpu-group/<group>=<group>", used on multi-GPU fractions.
	kaiGPUGroupLabelPrefix = kaiGPUGroupLabel + "/"
	// kaiReservationGPUAnnotation holds the UUID of the GPU a reservation pod
	// reserves. It is what distinguishes a reservation pod from the fractional
	// workloads sharing its GPU: both carry the group label, only the
	// reservation pod carries this.
	kaiReservationGPUAnnotation = "run.ai/reserve_for_gpu_index"
	// kaiContainerAnnotationPrefix prefixes the per-container annotations KAI's
	// binder writes, e.g.
	// "nvidia.com/container.<name>.gpu-memory.request". The name embedded in
	// the key is how KAI marks which container of the pod gets the GPU.
	kaiContainerAnnotationPrefix = "nvidia.com/container."
	// kaiFractionContainerNameAnnotation names the container that gets the GPU
	// when the pod author sets it explicitly.
	kaiFractionContainerNameAnnotation = "gpu-fraction-container-name"
	// kaiGPUFractionAnnotation requests the same fraction of each GPU the pod
	// gets, e.g. "0.5".
	kaiGPUFractionAnnotation = "gpu-fraction"
	// kaiGPUMemoryAnnotation requests the same amount of memory, in MiB, on
	// each GPU the pod gets, e.g. "4096". It is the alternative to
	// kaiGPUFractionAnnotation.
	kaiGPUMemoryAnnotation = "gpu-memory"
	// kaiGPUMemoryRequestSuffix ends the per-container annotation holding the
	// memory the container gets on each of its GPUs, as a Kubernetes quantity,
	// e.g. "nvidia.com/container.<name>.gpu-memory.request: 7680Mi". Since
	// v0.18, KAI's admission webhook writes it from gpu-memory, and its binder
	// from the fraction it allocated when the pod requested gpu-fraction.
	kaiGPUMemoryRequestSuffix = ".gpu-memory.request"
)

// Terminal pod phases, as the kubelet reports them in KubernetesPod.Phase.
// These mirror corev1.PodSucceeded and corev1.PodFailed, deliberately
// duplicated rather than imported: this file is built without the kubelet tag
// too, where the package's dependency closure has no k8s.io/api, and two frozen
// API constants do not justify widening it.
const (
	podPhaseSucceeded = "Succeeded"
	podPhaseFailed    = "Failed"
)

// podPhaseRunning mirrors corev1.PodRunning.
const podPhaseRunning = "Running"

const kaiTelemetrySubsystem = consts.GpuTelemetryModule + "__kai"

// Values of the reason tag of the assignment_failed counter.
const (
	kaiReasonReservationWithoutGroup    = "reservation_without_group"
	kaiReasonReservationNotFound        = "reservation_not_found"
	kaiReasonMultipleFractionContainers = "multiple_fraction_containers"
	kaiReasonNamedContainerNotFound     = "named_container_not_found"
	kaiReasonUnidentifiedContainer      = "unidentified_container"
)

// newKAIAssignmentFailedCounter registers the gpu__kai__assignment_failed
// counter. A failure is counted on every pull for as long as its cause persists.
func newKAIAssignmentFailedCounter(tm telemetry.Component) telemetry.Counter {
	return tm.NewCounter(kaiTelemetrySubsystem, "assignment_failed", []string{"reason"},
		"Number of KAI fractional GPU assignment failures, by reason")
}

// kaiWarnLimiter is separate from the collector-wide logLimiter so that a pod
// warned about on every pull cannot starve the driver, device and MIG warnings
// that the rest of this collector depends on.
var kaiWarnLimiter = log.NewLogLimit(20, 10*time.Minute)

// kaiShareWarnLimiter is separate from kaiWarnLimiter for the same reason: a
// pod whose share cannot be read is warned about on every pull, and must not
// starve the warnings about GPUs that cannot be attributed at all.
var kaiShareWarnLimiter = log.NewLogLimit(20, 10*time.Minute)

// kaiFractions is what one pull could establish about the KAI Scheduler
// fractional GPUs on this node.
type kaiFractions struct {
	// uuidsByContainer holds the GPU UUIDs resolved for each fractional
	// container, keyed by container ID.
	uuidsByContainer map[string][]string
	// sharesByContainer holds the share of each GPU of each fractional
	// container, keyed by container ID and then by GPU UUID.
	sharesByContainer map[string]map[string]workloadmeta.GPUShare
	// sharedGPUs holds the UUIDs of the GPUs that a live reservation pod holds
	// for KAI fractions, whether or not the workloads sharing them could be
	// attributed.
	sharedGPUs map[string]struct{}
	// claiming holds every container that still has a fraction to account for:
	// one whose GPUs resolved, and one still waiting for a reservation pod. A
	// container whose reservations were all seen and whose GPUs have all left
	// NVML is absent, so that the caller retracts its stale mapping.
	claiming map[string]struct{}
	// partial holds the containers that share more GPUs than this pull could
	// account for, so that the caller can protect their existing mapping from
	// being replaced by the smaller set. Only groups whose reservation pod has
	// not been seen count: a group whose reservation names a device NVML does
	// not have is accounted for, because waiting longer cannot change it.
	partial map[string]struct{}
}

// kaiReservation is the GPU one KAI reservation pod holds. An empty uuid means
// the reservation was seen but its device is not attributable on this node.
type kaiReservation struct {
	uuid string
	// createdAt breaks the tie when two live reservation pods claim one group.
	createdAt time.Time
}

// reservationCandidate is a reservation pod before its GPU has been validated.
type reservationCandidate struct {
	kaiReservation
	group string
}

// resolveKAIFractions returns the KAI Scheduler fractional GPUs on this node,
// or none when KAI support is disabled.
func (c *collector) resolveKAIFractions(deviceCache ddnvml.DeviceCache) kaiFractions {
	if !c.kaiSchedulerEnabled {
		return kaiFractions{}
	}
	return c.kaiFractionalGPUs(deviceCache)
}

// kaiFractionalGPUs resolves which GPU each KAI Scheduler fractional container
// is using.
//
// KAI hands out part of a GPU without the pod ever requesting nvidia.com/gpu,
// so the kubelet PodResources API holds no allocation for it and the regular
// device-plugin path cannot attribute the device. The UUID lives on the
// reservation pod KAI's binder creates per shared GPU, linked to the workload
// by a shared GPU group label. This walks that link: group -> reservation pod
// -> UUID.
//
// The claim set is deliberately wider than the resolved map: a container claims
// while any of its groups is still waiting for a reservation pod, whether or not
// this pull could resolve it. Retraction keys off the claim set, so a
// reservation pod that is not in the store yet -- a new pod's binder has not
// finished, or the kubelet collector has not caught up -- does not unset the
// mapping and set it again on the next pull, flapping the pod tags on the
// device's metrics. It stops claiming once every group is accounted for and
// none of them yields a device, which is what lets a permanently removed GPU
// leave the container's mapping instead of pinning it.
//
// Note that unlike the DRA claim set, this one is derived from pods rather than
// from the container store, so it can name a container the caller's loop over
// ListContainers never visits. Such a container is recorded as claiming without
// a mapping ever being published for it, and produces an unset once its pod
// goes away; workloadmeta drops an unset for an entity it does not hold, so
// this is inert.
func (c *collector) kaiFractionalGPUs(deviceCache ddnvml.DeviceCache) kaiFractions {
	pods := c.store.ListKubernetesPods()
	reservations := c.kaiGPUGroupReservations(pods, deviceCache)

	fractions := kaiFractions{
		uuidsByContainer:  make(map[string][]string),
		sharesByContainer: make(map[string]map[string]workloadmeta.GPUShare),
		sharedGPUs:        make(map[string]struct{}),
		claiming:          make(map[string]struct{}),
		partial:           make(map[string]struct{}),
	}
	for _, reservation := range reservations {
		if reservation.uuid != "" {
			fractions.sharedGPUs[reservation.uuid] = struct{}{}
		}
	}
	for _, pod := range pods {
		// A reservation pod carries a group label too, but it is KAI's
		// placeholder rather than a workload: it holds the whole GPU from
		// Kubernetes' point of view and never touches it. Attributing the
		// device to it is the behaviour this collector exists to correct, so it
		// must not be treated as a fractional workload here, not even before it
		// has written its GPU annotation.
		if gpucontainers.IsKAIReservationPod(pod) {
			continue
		}
		// A finished workload no longer uses its fraction -- KAI only counts
		// Running and Pending pods as holding one -- but the kubelet keeps
		// reporting it for a while, with its container ID and group labels.
		// Skipping it stops it from claiming, so that its mapping is retracted
		// instead of keeping its tags on a GPU that others may still be using.
		if pod.Phase == podPhaseSucceeded || pod.Phase == podPhaseFailed {
			continue
		}

		groups := kaiGPUGroups(pod)
		if len(groups) == 0 {
			continue
		}

		containerID := c.kaiFractionContainerID(pod)
		if containerID == "" {
			// The container that gets the GPU is not in the store yet, or the
			// pod does not say which one it is, or names several. Nothing to
			// key a mapping on, and nothing to retract either. A container that
			// is not in the store yet is picked up on a later pull; the other
			// two cases are warned about by kaiFractionContainerID.
			continue
		}

		var uuids []string
		accountedFor := 0
		for _, group := range groups {
			reservation, found := reservations[group]
			if !found {
				// No reservation pod for this group yet. Genuinely pending, so
				// the container counts as partial below and keeps whatever it
				// published before.
				continue
			}
			// Seen, so it is accounted for even when the device turned out not
			// to be attributable. Counting that as pending would keep the
			// container partial on every future pull, pinning its mapping for
			// as long as the pod lives.
			accountedFor++
			if reservation.uuid == "" {
				continue
			}
			if !slices.Contains(uuids, reservation.uuid) {
				uuids = append(uuids, reservation.uuid)
			}
		}
		if accountedFor < len(groups) {
			fractions.partial[containerID] = struct{}{}
			// A settled pod is already bound, so its reservation pod should
			// exist by now.
			if kaiPodSettled(pod) {
				log.Debugf("KAI: running pod %s/%s has no live reservation pod for one of its GPU groups on this node", pod.Namespace, pod.Name)
				c.kaiAssignmentFailed.Inc(kaiReasonReservationNotFound)
			}
		}
		// Claim while something resolved, or while a group is still pending.
		//
		// A container whose every group is accounted for and which still
		// resolves to nothing -- every GPU it shares has left NVML -- has
		// nothing left to wait for, so it deliberately does not claim: the
		// caller's retraction then clears the mapping published while those
		// devices were still there. Keeping the claim would hold a GPUDeviceIDs
		// entry that no GPU entity matches for as long as the pod lives, and
		// the GPU check would count it as a missing container mapping on every
		// run.
		if len(uuids) > 0 || accountedFor < len(groups) {
			fractions.claiming[containerID] = struct{}{}
		}
		if len(uuids) > 0 {
			fractions.uuidsByContainer[containerID] = uuids
			containerName := kaiContainerNameByID(slices.Concat(pod.Containers, pod.InitContainers), containerID)
			fractions.sharesByContainer[containerID] = kaiGPUShares(pod, containerName, uuids, deviceCache)
		}
	}

	return fractions
}

// kaiGPUShares returns the share of each of the given GPUs that KAI assigned
// to the pod's fractional container, named containerName, keyed by GPU UUID.
//
// KAI assigns the same share of every GPU a pod gets. The fraction is read
// from the gpu-fraction annotation when the pod requested one, as that is the
// fraction the pod asked for and KAI accounts for. Otherwise the pod requested an amount of memory,
// which is turned into a fraction of each GPU's memory, capped at the whole
// GPU: from the per-container gpu-memory.request annotation, or from the
// gpu-memory annotation on KAI versions that do not write the former. An
// invalid value is skipped in favour of the next source. A GPU whose share
// cannot be determined still gets an entry, with a zero fraction, so that the
// container is not taken for having it whole.
func kaiGPUShares(pod *workloadmeta.KubernetesPod, containerName string, uuids []string, deviceCache ddnvml.DeviceCache) map[string]workloadmeta.GPUShare {
	shares := make(map[string]workloadmeta.GPUShare, len(uuids))
	for _, uuid := range uuids {
		shares[uuid] = workloadmeta.GPUShare{}
	}

	if raw := pod.Annotations[kaiGPUFractionAnnotation]; raw != "" {
		fraction, err := strconv.ParseFloat(raw, 64)
		if err == nil && fraction > 0 && fraction <= 1 {
			for _, uuid := range uuids {
				shares[uuid] = workloadmeta.GPUShare{Fraction: fraction}
			}
			return shares
		}
		warnInvalidKAIShare(pod, kaiGPUFractionAnnotation, raw)
	}

	requested, found := kaiRequestedGPUMemory(pod, containerName)
	if !found {
		return shares
	}
	for _, uuid := range uuids {
		// The UUIDs were validated against NVML by kaiGPUGroupReservations, so
		// a lookup failure here means the device left between the two calls.
		device, err := deviceCache.GetByUUID(uuid)
		if err != nil {
			log.Debugf("KAI: cannot get GPU %s to compute the share of pod %s/%s: %v", uuid, pod.Namespace, pod.Name, err)
			continue
		}
		total := device.GetDeviceInfo().Memory
		if total == 0 {
			log.Debugf("KAI: GPU %s reports no memory, so the share of pod %s/%s on it cannot be computed", uuid, pod.Namespace, pod.Name)
			continue
		}
		shares[uuid] = workloadmeta.GPUShare{Fraction: min(1, float64(requested)/float64(total))}
	}
	return shares
}

// kaiRequestedGPUMemory returns the memory, in bytes, that the pod's
// fractional container requests on each of its GPUs, and whether it could be
// read: from the per-container gpu-memory.request annotation, else from the
// gpu-memory annotation.
func kaiRequestedGPUMemory(pod *workloadmeta.KubernetesPod, containerName string) (uint64, bool) {
	key := kaiContainerAnnotationPrefix + containerName + kaiGPUMemoryRequestSuffix
	if raw := pod.Annotations[key]; raw != "" {
		if requested, ok := parseMemoryQuantity(raw); ok {
			return requested, true
		}
		warnInvalidKAIShare(pod, key, raw)
	}

	if raw := pod.Annotations[kaiGPUMemoryAnnotation]; raw != "" {
		mib, err := strconv.ParseUint(raw, 10, 64)
		if err == nil && mib > 0 && mib <= math.MaxUint64>>20 {
			return mib << 20, true
		}
		warnInvalidKAIShare(pod, kaiGPUMemoryAnnotation, raw)
	}

	return 0, false
}

// parseMemoryQuantity parses a positive memory amount written as a Kubernetes
// quantity, e.g. "7680Mi", into bytes.
func parseMemoryQuantity(raw string) (uint64, bool) {
	quantity, err := resource.ParseQuantity(raw)
	if err != nil {
		return 0, false
	}
	// Value rounds a fraction of a byte up, the way Kubernetes does.
	bytes := quantity.Value()
	if bytes <= 0 {
		return 0, false
	}
	return uint64(bytes), true
}

// warnInvalidKAIShare warns that an annotation holding a pod's share of its
// GPUs cannot be read.
func warnInvalidKAIShare(pod *workloadmeta.KubernetesPod, annotation, value string) {
	if kaiShareWarnLimiter.ShouldLog() {
		log.Warnf("KAI: pod %s/%s has an invalid %s annotation %q, ignoring it for its GPU share", pod.Namespace, pod.Name, annotation, value)
	}
}

// kaiGPUGroupReservations maps each KAI GPU group to the GPU its reservation
// pod holds. A group whose reservation names a device NVML does not report on
// this node is present with an empty UUID: the distinction between "not seen"
// and "seen but not attributable" is what keeps a permanently missing device
// from pinning a workload's mapping forever.
func (c *collector) kaiGPUGroupReservations(pods []*workloadmeta.KubernetesPod, deviceCache ddnvml.DeviceCache) map[string]kaiReservation {
	// Collect first and validate against NVML afterwards: the device list is
	// only worth building once a reservation pod is actually present, so a node
	// without KAI pays nothing for this.
	var candidates []reservationCandidate
	for _, pod := range pods {
		uuid := pod.Annotations[kaiReservationGPUAnnotation]
		if uuid == "" {
			continue
		}
		// A reservation pod that has finished no longer holds its GPU, but the
		// kubelet keeps reporting it for a while. Letting it compete with the
		// live reservation for the same group would point every workload in
		// that group at a GPU nobody holds. A terminating-but-Running pod is
		// deliberately kept: it still holds its GPU.
		if pod.Phase == podPhaseSucceeded || pod.Phase == podPhaseFailed {
			continue
		}
		// Reservation pods use the plain group label: each reserves exactly one
		// GPU, so the per-GPU form never applies to them.
		group := pod.Labels[kaiGPUGroupLabel]
		if group == "" {
			log.Debugf("KAI: reservation pod %s/%s reserves GPU %s but carries no %s label, so no workload can be matched to it", pod.Namespace, pod.Name, uuid, kaiGPUGroupLabel)
			c.kaiAssignmentFailed.Inc(kaiReasonReservationWithoutGroup)
			continue
		}
		candidates = append(candidates, reservationCandidate{
			kaiReservation: kaiReservation{uuid: uuid, createdAt: pod.CreationTimestamp},
			group:          group,
		})
	}
	if len(candidates) == 0 {
		return nil
	}

	// Resolve collisions before validating, so that a stale reservation cannot
	// win a group merely because the live one's device is momentarily absent.
	winners := make(map[string]reservationCandidate, len(candidates))
	for _, candidate := range candidates {
		if current, taken := winners[candidate.group]; taken && !kaiReservationSupersedes(candidate.kaiReservation, current.kaiReservation) {
			continue
		}
		winners[candidate.group] = candidate
	}

	// GetByUUID below cannot tell a device NVML does not report from a device
	// list that cannot be read, so check the latter first.
	if _, err := deviceCache.All(); err != nil {
		// Without the device list nothing can be validated, and guessing would
		// publish device IDs that no GPU entity matches. Resolving nothing
		// leaves the mappings from earlier pulls in place: every fractional
		// container stays in the claim set, so none of them is retracted.
		// Pull lists the devices before reaching this collector and fails the
		// whole pull if it cannot, so reaching this means something is wrong.
		log.Errorf("KAI: cannot list GPU devices to validate %d reservation pod(s): %v", len(candidates), err)
		return nil
	}

	reservations := make(map[string]kaiReservation, len(winners))
	for group, winner := range winners {
		if _, err := deviceCache.GetByUUID(winner.uuid); err != nil {
			// The reservation names a GPU this node's NVML does not report: the
			// device is in gpu.excluded_devices, or it disappeared between the
			// pod's binding and this pull. Debug rather than warn, because the
			// excluded-device case is a supported configuration that would
			// otherwise log on every pull forever.
			log.Debugf("KAI: ignoring the reservation of GPU %s for group %s: NVML does not report that device on this node", winner.uuid, group)
			reservations[group] = kaiReservation{}
			continue
		}
		reservations[group] = winner.kaiReservation
	}
	return reservations
}

// kaiReservationSupersedes reports whether a reservation should replace the one
// already recorded for its group. Two live reservation pods for one group means
// a GPU was re-bound and the previous pod has not gone away yet, so the newer
// pod is the one holding it. Equal timestamps fall back to comparing the UUID:
// ListKubernetesPods iterates a map, so an arbitrary winner would alternate
// between pulls and make every pull look like a change.
func kaiReservationSupersedes(candidate, current kaiReservation) bool {
	if !candidate.createdAt.Equal(current.createdAt) {
		return candidate.createdAt.After(current.createdAt)
	}
	return candidate.uuid < current.uuid
}

// kaiGPUGroups returns the KAI GPU groups a pod belongs to, sorted so that the
// UUIDs derived from them keep a stable order across pulls. Label maps iterate
// in random order, and an unstable GPUDeviceIDs order would make every pull
// look like a change to workloadmeta.
func kaiGPUGroups(pod *workloadmeta.KubernetesPod) []string {
	var groups []string

	if group := pod.Labels[kaiGPUGroupLabel]; group != "" {
		groups = append(groups, group)
	}
	for key, value := range pod.Labels {
		if !strings.HasPrefix(key, kaiGPUGroupLabelPrefix) {
			continue
		}
		// KAI writes the group as both the key suffix and the value, one label per
		// shared GPU:
		//
		//	runai-gpu-group/<group-a>=<group-a>
		//	runai-gpu-group/<group-b>=<group-b>
		if value != "" && !slices.Contains(groups, value) {
			groups = append(groups, value)
		}
	}

	slices.Sort(groups)
	return groups
}

// kaiFractionContainerID returns the ID of the container that gets the GPU,
// following the same order as KAI's own GetFractionContainerRef: a container
// named in a "nvidia.com/container.<name>.*" annotation, else the one named in
// the gpu-fraction-container-name annotation, else the pod's only container
// when nothing names one. The GPU container can be an init container, so both
// lists are searched.
//
// Returns "" when a named container is not in the store yet, when a pod with
// several containers says nothing about which of them uses the GPU, and when
// the per-container annotations name more than one container.
func (c *collector) kaiFractionContainerID(pod *workloadmeta.KubernetesPod) string {
	containers := slices.Concat(pod.Containers, pod.InitContainers)

	names := kaiAnnotatedContainerNames(pod.Annotations)
	if len(names) > 1 {
		// KAI does not support several fractional containers in one pod: since
		// v0.18 its admission webhook rejects such a pod and its binder fails to
		// bind it (GetNvFractionsContainerName), so a pod seen on this node
		// cannot be in this state unless something is wrong. Older versions do
		// not use these annotations at all, so only another tool could have
		// written them. Picking one would be a guess KAI never makes, so warn
		// instead of attributing the GPU.
		if kaiWarnLimiter.ShouldLog() {
			log.Warnf("KAI: pod %s/%s has fractional GPU annotations for several containers %v, which KAI does not support, so its GPU cannot be attributed", pod.Namespace, pod.Name, names)
		}
		c.kaiAssignmentFailed.Inc(kaiReasonMultipleFractionContainers)
		return ""
	}
	if len(names) == 0 {
		// Reached only when no per-container annotation exists at all. KAI's
		// rule is "the annotated container, else this one", so a per-container
		// annotation naming a container that is not in the store yet must be
		// waited for rather than passed over in favour of this one.
		if name := pod.Annotations[kaiFractionContainerNameAnnotation]; name != "" {
			names = []string{name}
		}
	}
	for _, name := range names {
		if id := kaiContainerIDByName(containers, name); id != "" {
			return id
		}
	}
	if len(names) > 0 {
		// KAI named a container that workloadmeta does not have: the runtime has
		// not created it, so the kubelet collector dropped it from the pod.
		// Waiting for it is right -- falling through to another container would
		// attribute the GPU to a sidecar, publish that, and retract it a pull
		// later.
		//
		// A container that is not created yet still has a status, so a settled
		// pod with no status for that name names a container it does not have.
		if kaiPodSettled(pod) && !kaiHasContainerStatus(pod, names[0]) {
			log.Debugf("KAI: running pod %s/%s names %s as its GPU container, but has no such container", pod.Namespace, pod.Name, names[0])
			c.kaiAssignmentFailed.Inc(kaiReasonNamedContainerNotFound)
		}
		return ""
	}

	// Nothing names a container. KAI falls back to the pod's first regular
	// container, which is only identifiable here when there is exactly one: a
	// pod's container list is built from the kubelet's status array, which is
	// not kept in spec order. Since v0.18, KAI's binder writes a per-container
	// annotation before it binds a fractional pod to the node, so reaching this
	// with several containers is an anomaly there; older KAI versions do not
	// write it, so a pod with a sidecar and no gpu-fraction-container-name ends
	// up here routinely. Either way the GPU stays unattributed, hence the
	// warning, which says how to fix it.
	if len(pod.Containers) == 1 {
		return pod.Containers[0].ID
	}
	if len(pod.Containers) > 1 {
		if kaiWarnLimiter.ShouldLog() {
			log.Warnf("KAI: pod %s/%s shares a GPU but no annotation says which of its %d containers uses it, so its GPU cannot be attributed; set the %s annotation to the name of that container in the pod spec, before the pod is scheduled", pod.Namespace, pod.Name, len(pod.Containers), kaiFractionContainerNameAnnotation)
		}
		c.kaiAssignmentFailed.Inc(kaiReasonUnidentifiedContainer)
	}
	return ""
}

// kaiAnnotatedContainerNames returns the distinct container names KAI marked
// with a per-container annotation, sorted so that logs list them in a stable
// order.
func kaiAnnotatedContainerNames(annotations map[string]string) []string {
	var names []string
	for key := range annotations {
		rest, found := strings.CutPrefix(key, kaiContainerAnnotationPrefix)
		if !found {
			continue
		}
		// The key is "nvidia.com/container.<name>.<setting>"; a container name
		// cannot contain a dot, so the first one ends the name. Cutting on it
		// is also what keeps "gpu-workload" from matching an annotation for
		// "gpu-workload-sidecar".
		name, _, found := strings.Cut(rest, ".")
		if !found || name == "" || slices.Contains(names, name) {
			continue
		}
		names = append(names, name)
	}

	slices.Sort(names)
	return names
}

// kaiPodSettled reports whether the pod is Running and not terminating, so
// that KAI has finished binding it and nothing it is missing will still appear.
func kaiPodSettled(pod *workloadmeta.KubernetesPod) bool {
	return pod.Phase == podPhaseRunning && pod.DeletionTimestamp == nil
}

// kaiHasContainerStatus reports whether the pod has a status for the named
// container, created or not.
func kaiHasContainerStatus(pod *workloadmeta.KubernetesPod, name string) bool {
	return slices.ContainsFunc(slices.Concat(pod.ContainerStatuses, pod.InitContainerStatuses), func(status workloadmeta.KubernetesContainerStatus) bool {
		return status.Name == name
	})
}

// kaiContainerNameByID returns the name of the container with the given ID, or
// "" if the pod has no such container.
func kaiContainerNameByID(containers []workloadmeta.OrchestratorContainer, id string) string {
	for _, container := range containers {
		if container.ID == id {
			return container.Name
		}
	}
	return ""
}

// kaiContainerIDByName returns the ID of the named container, or "" if the pod
// has no such container.
func kaiContainerIDByName(containers []workloadmeta.OrchestratorContainer, name string) string {
	for _, container := range containers {
		if container.Name == name {
			return container.ID
		}
	}
	return ""
}
