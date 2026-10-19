// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

package nvml

import (
	"slices"
	"strings"
	"time"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
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

// kaiFractions is what one pull could establish about the KAI Scheduler
// fractional GPUs on this node.
type kaiFractions struct {
	// uuidsByContainer holds the GPU UUIDs resolved for each fractional
	// container, keyed by container ID.
	uuidsByContainer map[string][]string
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
	reservations := kaiGPUGroupReservations(pods, deviceCache)

	fractions := kaiFractions{
		uuidsByContainer: make(map[string][]string),
		claiming:         make(map[string]struct{}),
		partial:          make(map[string]struct{}),
	}
	for _, pod := range pods {
		// A reservation pod carries a group label too, but it is KAI's
		// placeholder rather than a workload: it holds the whole GPU from
		// Kubernetes' point of view and never touches it. Attributing the
		// device to it is the behaviour this collector exists to correct, so it
		// must not be treated as a fractional workload here.
		if pod.Annotations[kaiReservationGPUAnnotation] != "" {
			continue
		}

		groups := kaiGPUGroups(pod)
		if len(groups) == 0 {
			continue
		}

		containerID := kaiFractionContainerID(pod)
		if containerID == "" {
			// The container that gets the GPU is not in the store yet, or the
			// pod does not say which one it is. Nothing to key a mapping on,
			// and nothing to retract either -- the next pull picks it up.
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
		}
	}

	return fractions
}

// kaiGPUGroupReservations maps each KAI GPU group to the GPU its reservation
// pod holds. A group whose reservation names a device NVML does not report on
// this node is present with an empty UUID: the distinction between "not seen"
// and "seen but not attributable" is what keeps a permanently missing device
// from pinning a workload's mapping forever.
func kaiGPUGroupReservations(pods []*workloadmeta.KubernetesPod, deviceCache ddnvml.DeviceCache) map[string]kaiReservation {
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

	knownUUIDs, err := knownGPUUUIDs(deviceCache)
	if err != nil {
		// Without the device list nothing can be validated, and guessing would
		// publish device IDs that no GPU entity matches. Resolving nothing
		// leaves the mappings from earlier pulls in place: every fractional
		// container stays in the claim set, so none of them is retracted.
		// Debug rather than warn: Pull lists the devices before reaching this
		// collector and fails the whole pull if it cannot, so this is defensive
		// against a future caller rather than a condition that occurs today.
		log.Debugf("KAI: cannot list GPU devices to validate %d reservation pod(s): %v", len(candidates), err)
		return nil
	}

	reservations := make(map[string]kaiReservation, len(winners))
	for group, winner := range winners {
		if _, known := knownUUIDs[winner.uuid]; !known {
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

// knownGPUUUIDs returns the set of GPU UUIDs NVML reports on this node,
// including MIG children. These are exactly the UUIDs the collector publishes
// GPU entities for, so a device ID outside this set would match no entity.
func knownGPUUUIDs(deviceCache ddnvml.DeviceCache) (map[string]struct{}, error) {
	devices, err := deviceCache.All()
	if err != nil {
		return nil, err
	}

	uuids := make(map[string]struct{}, len(devices))
	for _, device := range devices {
		uuids[device.GetDeviceInfo().UUID] = struct{}{}
	}
	return uuids, nil
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
// Returns "" when a named container is not in the store yet, and when a pod
// with several containers says nothing about which of them uses the GPU.
func kaiFractionContainerID(pod *workloadmeta.KubernetesPod) string {
	containers := slices.Concat(pod.Containers, pod.InitContainers)

	names := kaiAnnotatedContainerNames(pod.Annotations)
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
		return ""
	}

	// Nothing names a container. KAI falls back to the pod's first regular
	// container, which is only identifiable here when there is exactly one: a
	// pod's container list is built from the kubelet's status array, which is
	// not kept in spec order. In practice KAI's binder writes a per-container
	// annotation on every bound fractional pod, so this is a defensive path.
	if len(pod.Containers) == 1 {
		return pod.Containers[0].ID
	}
	if len(pod.Containers) > 1 {
		log.Debugf("KAI: pod %s/%s shares a GPU but no annotation says which of its %d containers uses it", pod.Namespace, pod.Name, len(pod.Containers))
	}
	return ""
}

// kaiAnnotatedContainerNames returns the container names KAI marked with a
// per-container annotation, sorted. Sorting rather than taking them in map
// order is what makes a pod carrying annotations for more than one container
// resolve to the same container on every pull.
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
