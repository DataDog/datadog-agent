// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package bundle

import (
	"errors"
	"regexp"
	"slices"
	"time"

	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/schema"
)

var (
	commitPattern      = regexp.MustCompile(`^[0-9a-f]{40}$`)
	versionPattern     = regexp.MustCompile(`^[0-9][a-zA-Z0-9.+_-]{0,127}$`)
	opaqueIDPattern    = regexp.MustCompile(`^[a-zA-Z0-9_-]{16,64}$`)
	destinationPattern = regexp.MustCompile(`^(vector/)?(primary(-[1-9][0-9]*)?|(additional|local|failover)-[1-9][0-9]*)(/[1-9][0-9]*)?$`)
)

func validCommit(commit string) bool { return commitPattern.MatchString(commit) }

func validBuild(build BuildIdentity) bool {
	return versionPattern.MatchString(build.Version) && validCommit(build.Commit)
}

type cycleKey struct {
	producer string
	cycle    uint64
}

type cycleEvidence struct {
	stream   schema.Stream
	offset   time.Duration
	sequence uint64
	count    int
	next     int
}

func (m *Manifest) validateProvenance() (map[schema.Stream]int, error) {
	if !validBuild(m.CaptureTool) || !opaqueIDPattern.MatchString(m.SessionID) {
		return nil, errors.New("invalid capture-tool or session identity; recapture")
	}
	if len(m.Producers) == 0 || len(m.Producers) > 3 {
		return nil, errors.New("capture requires a participating producer inventory")
	}
	producers := map[string]Producer{}
	roles := map[string]bool{}
	owners := map[schema.Stream]string{}
	var firstStart, lastStart time.Duration
	for i, producer := range m.Producers {
		if !opaqueIDPattern.MatchString(producer.InstanceID) || producers[producer.InstanceID].InstanceID != "" || roles[producer.Role] ||
			!validBuild(BuildIdentity{Version: producer.Version, Commit: producer.Commit}) || producer.ProtocolVersion != 1 {
			return nil, errors.New("invalid or incompatible capture producer identity; recapture")
		}
		if !producer.Stopped || producer.Failures != 0 || producer.Drops != 0 || producer.FinalSequence == 0 || producer.FinalSequence != producer.AcknowledgedSequence {
			return nil, errors.New("capture producer lacks complete stopped and drained acknowledgement")
		}
		if producer.StartOffset < 0 || producer.StopOffset < producer.StartOffset || producer.StopOffset > m.Duration {
			return nil, errors.New("invalid acknowledged producer boundaries")
		}
		if i == 0 || producer.StartOffset < firstStart {
			firstStart = producer.StartOffset
		}
		if producer.StartOffset > lastStart {
			lastStart = producer.StartOffset
		}
		if len(producer.Streams) == 0 {
			return nil, errors.New("capture producer has no participating stream")
		}
		for _, stream := range producer.Streams {
			if !producerOwns(producer.Role, stream) || !slices.Contains(m.Profile.Streams, stream) || owners[stream] != "" {
				return nil, errors.New("capture stream has an invalid or competing producer")
			}
			owners[stream] = producer.InstanceID
		}
		producers[producer.InstanceID], roles[producer.Role] = producer, true
	}
	if lastStart-firstStart > 5*time.Second {
		return nil, errors.New("capture producer activation spread exceeds five seconds")
	}
	if len(owners) != len(m.Profile.Streams) {
		return nil, errors.New("capture stream inventory differs from producer participation")
	}
	cycles := map[cycleKey]*cycleEvidence{}
	sequences := map[string]map[uint64]uint64{}
	counts := map[schema.Stream]int{}
	var previous cycleKey
	for _, ref := range m.Samples {
		producer, ok := producers[ref.ProducerID]
		if !ok || owners[ref.Stream] != ref.ProducerID || ref.CycleID == 0 || ref.Sequence == 0 || ref.Sequence > producer.FinalSequence {
			return nil, errors.New("sample has invalid producer, cycle, or sequence evidence")
		}
		if ref.Offset < producer.StartOffset || ref.Offset > producer.StopOffset || ref.ChunkCount <= 0 || ref.ChunkCount > len(m.Samples) || ref.ChunkIndex < 0 || ref.ChunkIndex >= ref.ChunkCount {
			return nil, errors.New("sample lies outside producer boundaries or its complete group")
		}
		if ref.Stream != schema.Processes && ref.Stream != schema.Connections && (ref.ChunkIndex != 0 || ref.ChunkCount != 1) {
			return nil, errors.New("non-group stream has chunk evidence")
		}
		key := cycleKey{producer: ref.ProducerID, cycle: ref.CycleID}
		cycle := cycles[key]
		if cycle == nil {
			if old := cycles[previous]; old != nil && old.next != old.count {
				return nil, errors.New("capture group was interrupted before its final chunk")
			}
			if sequences[ref.ProducerID] == nil {
				sequences[ref.ProducerID] = map[uint64]uint64{}
			}
			if sequences[ref.ProducerID][ref.Sequence] != 0 {
				return nil, errors.New("accepted producer sequence identifies multiple cycles")
			}
			sequences[ref.ProducerID][ref.Sequence] = ref.CycleID
			cycle = &cycleEvidence{stream: ref.Stream, offset: ref.Offset, sequence: ref.Sequence, count: ref.ChunkCount}
			cycles[key] = cycle
			counts[ref.Stream]++
		}
		if cycle.stream != ref.Stream || cycle.offset != ref.Offset || cycle.sequence != ref.Sequence || cycle.count != ref.ChunkCount || ref.ChunkIndex != cycle.next {
			return nil, errors.New("capture group is incomplete, reordered, or inconsistent")
		}
		cycle.next++
		previous = key
		if err := validateRoutes(ref, m.Duration); err != nil {
			return nil, err
		}
	}
	for _, cycle := range cycles {
		if cycle.next != cycle.count {
			return nil, errors.New("capture group is missing chunks")
		}
	}
	return counts, nil
}

func producerOwns(role string, stream schema.Stream) bool {
	switch role {
	case "core-agent":
		return stream == schema.Metrics || stream == schema.HostMetadata || stream == schema.AgentInventory || stream == schema.HostInventory || stream == schema.HostSystemInfo || stream == schema.Software
	case "process-agent":
		return stream == schema.Processes || stream == schema.Connections
	case "system-probe":
		return stream == schema.Connections
	default:
		return false
	}
}

func routeEndpoint(stream schema.Stream, protocol string) string {
	if (stream == schema.AgentInventory || stream == schema.HostInventory || stream == schema.HostSystemInfo) && protocol == "inventory-v1" {
		return "/api/v1/metadata"
	}
	if stream == schema.Metrics {
		switch protocol {
		case "v1":
			return "/api/v1/series"
		case "v2":
			return "/api/v2/series"
		case "v3":
			return "/api/intake/metrics/v3/series"
		case "v3beta":
			return "/api/intake/metrics/v3beta/series"
		}
	}
	if stream == schema.HostMetadata {
		switch protocol {
		case "metadata-v1":
			return "/intake/"
		case "metadata-v2":
			return "/api/v2/host_metadata"
		}
	}
	return ""
}

// ValidateRoutes checks the privacy and correlation contract for observed routes.
// Coordinators also apply it to consumed cycles omitted from the final bundle.
func ValidateRoutes(ref SampleRef, duration time.Duration) error {
	return validateRoutes(ref, duration)
}

func validateRoutes(ref SampleRef, duration time.Duration) error {
	if (ref.Stream == schema.Metrics || ref.Stream == schema.HostMetadata || ref.Stream == schema.AgentInventory || ref.Stream == schema.HostInventory || ref.Stream == schema.HostSystemInfo) && len(ref.Routes) == 0 {
		return errors.New("sample lacks initial forwarding route evidence")
	}
	type routeKey struct {
		payload     uint64
		destination string
	}
	seen := map[routeKey]bool{}
	membership := map[uint64][]uint64{}
	protocols := map[uint64]string{}
	for _, route := range ref.Routes {
		endpoint := routeEndpoint(ref.Stream, route.Protocol)
		if route.PayloadID == 0 || endpoint == "" || route.Endpoint != endpoint || len(route.Destination) > 64 || !destinationPattern.MatchString(route.Destination) ||
			route.EnqueueOffset < ref.Offset || route.EnqueueOffset > duration {
			return errors.New("invalid or unsafe forwarding route evidence")
		}
		if previous := protocols[route.PayloadID]; previous != "" && previous != route.Protocol {
			return errors.New("payload fanout has inconsistent wire protocol evidence")
		}
		protocols[route.PayloadID] = route.Protocol
		if (ref.Stream == schema.AgentInventory || ref.Stream == schema.HostInventory || ref.Stream == schema.HostSystemInfo) && len(route.Ordinals) != 0 {
			return errors.New("inventory route cannot contain metric membership")
		}
		if ref.Stream == schema.Metrics && len(route.Ordinals) == 0 {
			return errors.New("metric route lacks payload membership evidence")
		}
		for i, ordinal := range route.Ordinals {
			if ordinal == 0 || (i > 0 && ordinal <= route.Ordinals[i-1]) {
				return errors.New("invalid metric payload membership evidence")
			}
		}
		key := routeKey{payload: route.PayloadID, destination: route.Destination}
		if seen[key] {
			return errors.New("duplicate initial forwarding route evidence")
		}
		seen[key] = true
		if previous, present := membership[route.PayloadID]; present && !slices.Equal(previous, route.Ordinals) {
			return errors.New("payload fanout has inconsistent semantic membership")
		}
		membership[route.PayloadID] = route.Ordinals
	}
	return nil
}
