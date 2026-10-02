// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package live

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/bundle"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/capture"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/output"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/safety"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/schema"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/telemetry"
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	softwareimpl "github.com/DataDog/datadog-agent/comp/softwareinventory/impl"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
	processapi "github.com/DataDog/datadog-agent/pkg/process/util/api"
	"github.com/DataDog/datadog-agent/pkg/process/util/api/headers"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	tc "github.com/DataDog/datadog-agent/pkg/telemetrycapture"
)

type evidencePipeline struct {
	pipeline *output.Pipeline
	recorder *output.Recorder
}

// Evidence serially sanitizes and persists complete observed cycles. Only
// sanitized data reaches its non-networking Agent delivery pipelines.
type Evidence struct {
	directory      string
	tool           bundle.BuildIdentity
	profile        schema.Profile
	session        Session
	sanitizer      *capture.Sanitizer
	writer         *bundle.Writer
	pipelines      map[string]evidencePipeline
	participants   map[string]Participant
	sequences      map[string]uint64
	cycles         map[string]map[uint64]bool
	offsets        map[schema.Stream][]time.Duration
	cadences       map[schema.Stream]time.Duration
	metricCadences map[string]time.Duration
	metricOffsets  map[string][]time.Duration
	closed         bool
}

var _ Sink = (*Evidence)(nil)

func NewEvidence(directory, platform, architecture string, tool bundle.BuildIdentity) *Evidence {
	return &Evidence{directory: directory, tool: tool, profile: schema.Profile{OS: platform, Architecture: architecture}}
}

func (e *Evidence) Start(ctx context.Context, session Session) error {
	if e.writer != nil || e.closed || session.Origin.IsZero() || len(session.Participants) == 0 ||
		(e.profile.OS != "macos" && e.profile.OS != "windows") || (e.profile.Architecture != "amd64" && e.profile.Architecture != "arm64") {
		return errors.New("invalid evidence capture session")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	e.participants = map[string]Participant{}
	e.sequences = map[string]uint64{}
	e.cycles = map[string]map[uint64]bool{}
	e.offsets = map[schema.Stream][]time.Duration{}
	e.cadences = map[schema.Stream]time.Duration{}
	e.metricCadences = map[string]time.Duration{}
	e.metricOffsets = map[string][]time.Duration{}
	e.pipelines = map[string]evidencePipeline{}
	owners := map[tc.Stream]bool{}
	for _, participant := range session.Participants {
		status := participant.Status
		if status.ProtocolVersion != tc.ProtocolVersion || status.SessionID != session.ID || status.State != tc.Active ||
			status.ActivatedAt.Before(session.Origin) || status.ActivatedAt.Sub(session.Origin) > 5*time.Second || len(participant.Streams) == 0 ||
			e.participants[status.Producer.InstanceID].Status.Producer.InstanceID != "" {
			return errors.New("invalid evidence producer activation")
		}
		for _, stream := range participant.Streams {
			mapped := evidenceStream(stream)
			if mapped == "" || owners[stream] {
				return errors.New("invalid evidence stream ownership")
			}
			owners[stream] = true
			e.profile.Streams = append(e.profile.Streams, mapped)
			for _, capability := range status.Capabilities {
				if capability.Stream == stream && capability.Cadence > 0 {
					e.cadences[mapped] = capability.Cadence
					if stream == tc.Metrics {
						if !validMetricSchedules(capability) {
							return errors.New("metric capture requires scheduled check cadences; reinstall compatible producers")
						}
						for _, schedule := range capability.MetricSchedules {
							e.metricCadences[schedule.Family] = schedule.Cadence
						}
					}
				}
			}
			if e.cadences[mapped] == 0 {
				return errors.New("evidence producer lacks an effective cadence")
			}
		}
		e.participants[status.Producer.InstanceID] = participant
		e.cycles[status.Producer.InstanceID] = map[uint64]bool{}
	}
	var err error
	e.sanitizer, err = capture.NewSanitizer()
	if err != nil {
		return errors.New("cannot initialize capture sanitizer")
	}
	e.writer, err = bundle.NewWriter(e.directory, bundle.Manifest{CaptureTool: e.tool, SessionID: session.ID, MetricCadences: e.metricCadences})
	if err != nil {
		return err
	}
	e.session = session
	return nil
}

func evidenceStream(stream tc.Stream) schema.Stream {
	switch stream {
	case tc.Metrics:
		return schema.Metrics
	case tc.Metadata:
		return schema.HostMetadata
	case tc.AgentInventory:
		return schema.AgentInventory
	case tc.HostInventory:
		return schema.HostInventory
	case tc.HostSystemInfo:
		return schema.HostSystemInfo
	case tc.Processes:
		return schema.Processes
	case tc.Connections:
		return schema.Connections
	case tc.Software:
		return schema.Software
	default:
		return ""
	}
}

func (e *Evidence) pipeline(ctx context.Context, protocol string) (evidencePipeline, error) {
	if existing, ok := e.pipelines[protocol]; ok {
		return existing, nil
	}
	destinations, err := (safety.Config{Site: safety.Site}).Resolve(func(string) string { return "" })
	if err != nil {
		return evidencePipeline{}, err
	}
	options := output.Options{MetricProtocol: "v2", QueueCapacity: 1}
	if strings.HasPrefix(protocol, "metadata-") {
		options.MetadataProtocol = protocol
	} else if protocol != "group" && protocol != "software" && protocol != "inventory-v1" {
		options.MetricProtocol = protocol
	}
	recorder := output.NewRecorder()
	pipeline, err := output.New(ctx, destinations, "recording-only-no-credential", recorder, options)
	if err != nil {
		return evidencePipeline{}, errors.New("cannot initialize observed wire protocol")
	}
	result := evidencePipeline{pipeline, recorder}
	e.pipelines[protocol] = result
	return result, nil
}

func (e *Evidence) Accept(ctx context.Context, record tc.Record) (err error) {
	defer func() {
		if err != nil {
			e.Close()
		}
	}()
	if e.writer == nil || e.closed {
		return errors.New("evidence session is not active")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	participant, ok := e.participants[record.Producer.InstanceID]
	if !ok || participant.Status.Producer != record.Producer || record.ProtocolVersion != tc.ProtocolVersion || record.SessionID != e.session.ID ||
		!slices.Contains(participant.Streams, record.Stream) || record.Sequence <= e.sequences[record.Producer.InstanceID] || record.CycleID == 0 || e.cycles[record.Producer.InstanceID][record.CycleID] ||
		record.CollectedAt.IsZero() || record.ObservedAt.Before(record.CollectedAt) || record.Cadence <= 0 {
		return errors.New("invalid evidence record provenance")
	}
	if len(e.cycles[record.Producer.InstanceID]) >= maxSessionCycles {
		return errors.New("capture cycle history exceeds memory limit")
	}
	e.sequences[record.Producer.InstanceID] = record.Sequence
	e.cycles[record.Producer.InstanceID][record.CycleID] = true
	if record.CollectedAt.Before(participant.Status.ActivatedAt) {
		return nil
	}
	stream := evidenceStream(record.Stream)
	ref := bundle.SampleRef{Stream: stream, ProducerID: record.Producer.InstanceID, CycleID: record.CycleID, Sequence: record.Sequence,
		Offset: record.CollectedAt.Sub(e.session.Origin), ChunkCount: 1}
	for _, route := range record.Payload.Routes {
		ref.Routes = append(ref.Routes, bundle.RoutingEvidence{PayloadID: route.PayloadID, Ordinals: slices.Clone(route.Ordinals),
			Endpoint: route.Endpoint, Protocol: route.Protocol, Destination: route.Destination, EnqueueOffset: route.EnqueuedAt.Sub(e.session.Origin)})
	}
	if err := bundle.ValidateRoutes(ref, time.Duration(1<<63-1)); err != nil {
		return err
	}
	var nonempty bool
	switch record.Stream {
	case tc.Metrics:
		nonempty, err = e.metrics(ctx, record, ref)
	case tc.Metadata:
		nonempty, err = e.metadata(ctx, record, ref)
	case tc.AgentInventory, tc.HostInventory, tc.HostSystemInfo:
		nonempty, err = e.inventory(ctx, record, ref)
	case tc.Software:
		nonempty, err = e.software(ctx, record, ref)
	case tc.Processes, tc.Connections:
		nonempty, err = e.group(ctx, record, ref)
	default:
		err = errors.New("unsupported evidence stream")
	}
	if err != nil {
		return err
	}
	if nonempty {
		e.offsets[stream] = append(e.offsets[stream], ref.Offset)
		e.cadences[stream] = record.Cadence
	}
	return nil
}

func (e *Evidence) metrics(ctx context.Context, record tc.Record, ref bundle.SampleRef) (bool, error) {
	if record.Payload.Metadata != nil || record.Payload.Inventory != nil || record.Payload.Software != nil || len(record.Payload.Chunks) != 0 {
		return false, errors.New("invalid metric evidence shape")
	}
	if len(record.Payload.Series) == 0 {
		return false, nil
	}
	var native []*metrics.Serie
	ordinals := map[uint64]int{}
	for _, serie := range record.Payload.Series {
		if serie.Ordinal == 0 || !tc.MetricAllowed(serie.Name) || uint32(metrics.MetricSource(serie.Source)) != serie.Source {
			return false, errors.New("invalid metric semantic evidence")
		}
		if _, exists := ordinals[serie.Ordinal]; exists {
			return false, errors.New("duplicate metric ordinal")
		}
		ordinals[serie.Ordinal] = len(native)
		value := &metrics.Serie{Name: serie.Name, Source: metrics.MetricSource(serie.Source), MType: metrics.APIMetricType(serie.Type), Interval: serie.Interval,
			Host: serie.Host, Device: serie.Device, Tags: tagset.CompositeTagsFromSlice(serie.Tags)}
		for _, point := range serie.Points {
			value.Points = append(value.Points, metrics.Point{Ts: point.Timestamp - float64(e.session.Origin.UnixNano())/1e9, Value: point.Value})
		}
		native = append(native, value)
	}
	source, err := e.sanitizer.Series(capture.NewSeriesSource(native))
	if err != nil {
		return false, errors.New("cannot sanitize metric evidence")
	}
	clean := source.(*capture.SeriesSource).Series
	typed, err := telemetry.NewMetricSample(clean)
	if err != nil {
		return false, errors.New("invalid sanitized metric evidence")
	}
	// Each protocol can cover a different filtered subset of the same cycle.
	memberships := map[string]map[uint64]bool{}
	observed := map[uint64]bool{}
	for _, route := range ref.Routes {
		if memberships[route.Protocol] == nil {
			memberships[route.Protocol] = map[uint64]bool{}
		}
		for _, ordinal := range route.Ordinals {
			if _, exists := ordinals[ordinal]; !exists {
				return false, errors.New("metric route references absent semantics")
			}
			memberships[route.Protocol][ordinal], observed[ordinal] = true, true
		}
	}
	if len(observed) != len(ordinals) {
		return false, errors.New("metric semantics lack forwarding evidence")
	}
	if !e.retainCycle(ref.Stream, ref.Offset) {
		return false, nil
	}
	var references []bundle.WireReference
	protocols := make([]string, 0, len(memberships))
	for protocol := range memberships {
		protocols = append(protocols, protocol)
	}
	sort.Strings(protocols)
	for _, protocol := range protocols {
		pipeline, err := e.pipeline(ctx, protocol)
		if err != nil {
			return false, err
		}
		var selected []*metrics.Serie
		for i, serie := range record.Payload.Series {
			if memberships[protocol][serie.Ordinal] {
				selected = append(selected, clean[i])
			}
		}
		// Reconstruct an owned iterator: Agent serialization may append tags.
		subset, err := telemetry.NewMetricSample(selected)
		if err != nil {
			return false, err
		}
		owned, err := subset.AgentSeries()
		if err != nil {
			return false, err
		}
		if err := pipeline.pipeline.Serializer.SendIterableSeries(capture.NewSeriesSource(owned)); err != nil {
			return false, errors.New("cannot regenerate metric wire evidence")
		}
		if err := pipeline.pipeline.Wait(ctx); err != nil {
			return false, errors.New("metric wire evidence did not complete")
		}
		references = append(references, pipeline.recorder.Drain()...)
	}
	if err := e.writer.Append(ref, typed, references); err != nil {
		return false, err
	}
	families := map[string]bool{}
	for _, serie := range clean {
		family := tc.MetricFamily(serie.Name)
		if e.metricCadences[family] <= 0 {
			return false, errors.New("observed metric family lacks a scheduled cadence")
		}
		families[family] = true
		e.profile.MetricNames = appendUnique(e.profile.MetricNames, serie.Name)
	}
	for family := range families {
		e.metricOffsets[family] = append(e.metricOffsets[family], ref.Offset)
	}
	return true, nil
}

func metadataProjection(in *tc.HostMetadata) *capture.HostMetadata {
	result := &capture.HostMetadata{AgentVersion: in.AgentVersion, UUID: in.UUID, Hostname: in.Hostname, OS: in.OS, AgentFlavor: in.AgentFlavor,
		HostTags: in.HostTags, Network: map[string]string{"network-id": in.NetworkID}, SystemStats: map[string]json.RawMessage{}}
	for key, value := range map[string]any{"cpuCores": in.CPUCores, "machine": in.Machine, "platform": in.Platform,
		"macV": []any{in.MacVersion, [3]string{}, in.MacMachine}, "winV": in.Windows} {
		result.SystemStats[key], _ = json.Marshal(value)
	}
	if len(in.Gohai) != 0 {
		data, _ := json.Marshal(in.Gohai)
		result.Gohai = string(data)
	}
	return result
}

func (e *Evidence) metadata(ctx context.Context, record tc.Record, ref bundle.SampleRef) (bool, error) {
	if record.Payload.Metadata == nil || record.Payload.Metadata.Hostname == "" || len(record.Payload.Series) != 0 || len(record.Payload.Chunks) != 0 || record.Payload.Inventory != nil || record.Payload.Software != nil || record.Payload.Metadata.AgentVersion != record.Producer.Version {
		return false, errors.New("invalid host metadata projection")
	}
	clean, err := e.sanitizer.HostMetadata(metadataProjection(record.Payload.Metadata))
	if err != nil {
		return false, errors.New("cannot sanitize host metadata")
	}
	if err := e.validateSample(ref.Stream, clean); err != nil {
		return false, err
	}
	if !e.retainCycle(ref.Stream, ref.Offset) {
		return false, nil
	}
	protocols := map[string]bool{}
	var references []bundle.WireReference
	for _, route := range ref.Routes {
		if protocols[route.Protocol] {
			continue
		}
		protocols[route.Protocol] = true
		pipeline, err := e.pipeline(ctx, route.Protocol)
		if err != nil {
			return false, err
		}
		if err := pipeline.pipeline.Serializer.SendHostMetadata(clean); err != nil {
			return false, errors.New("cannot regenerate host metadata wire evidence")
		}
		if err := pipeline.pipeline.Wait(ctx); err != nil {
			return false, errors.New("host metadata wire evidence did not complete")
		}
		references = append(references, pipeline.recorder.Drain()...)
	}
	if err := e.writer.Append(ref, clean, references); err != nil {
		return false, err
	}
	return true, nil
}

func (e *Evidence) inventory(ctx context.Context, record tc.Record, ref bundle.SampleRef) (bool, error) {
	in := record.Payload.Inventory
	if in == nil || record.Payload.Metadata != nil || record.Payload.Software != nil || len(record.Payload.Series) != 0 || len(record.Payload.Chunks) != 0 ||
		in.Timestamp < record.CollectedAt.UnixNano() || in.Timestamp > record.ObservedAt.UnixNano() ||
		(record.Stream == tc.AgentInventory && (in.Agent == nil || in.Host != nil || in.SystemInfo != nil || in.Agent.AgentVersion != record.Producer.Version)) ||
		(record.Stream == tc.HostInventory && (in.Host == nil || in.Agent != nil || in.SystemInfo != nil || in.Host.AgentVersion != record.Producer.Version)) ||
		(record.Stream == tc.HostSystemInfo && (in.SystemInfo == nil || in.Agent != nil || in.Host != nil)) {
		return false, errors.New("invalid inventory projection or collection boundary")
	}
	clean, err := e.sanitizer.Inventory(in, e.session.Origin)
	if err != nil {
		return false, err
	}
	if err := e.validateSample(ref.Stream, clean); err != nil {
		return false, err
	}
	if !e.retainCycle(ref.Stream, ref.Offset) {
		return false, nil
	}
	pipeline, err := e.pipeline(ctx, "inventory-v1")
	if err != nil {
		return false, err
	}
	if err := pipeline.pipeline.Serializer.SendMetadata(clean); err != nil {
		return false, errors.New("cannot regenerate inventory wire evidence")
	}
	if err := pipeline.pipeline.Wait(ctx); err != nil {
		return false, errors.New("inventory wire evidence did not complete")
	}
	if err := e.writer.Append(ref, clean, pipeline.recorder.Drain()); err != nil {
		return false, err
	}
	return true, nil
}

func (e *Evidence) software(ctx context.Context, record tc.Record, ref bundle.SampleRef) (bool, error) {
	message := record.Payload.Software
	if message == nil || record.Payload.Metadata != nil || record.Payload.Inventory != nil || len(record.Payload.Series) != 0 || len(record.Payload.Chunks) != 0 ||
		message.Timestamp != record.CollectedAt.UnixNano() {
		return false, errors.New("invalid software snapshot evidence")
	}
	var native softwareimpl.Payload
	if err := bundle.DecodeJSON(message.Body, &native); err != nil || native.Hostname == "" {
		return false, errors.New("invalid software snapshot")
	}
	if len(native.Metadata.Software) == 0 {
		return false, nil
	}
	clean := &softwareimpl.Payload{Hostname: "capture-host", Metadata: softwareimpl.HostSoftware{Software: e.sanitizer.Software(native.Metadata.Software)}}
	if err := e.validateSample(ref.Stream, clean); err != nil {
		return false, err
	}
	if !e.retainCycle(ref.Stream, ref.Offset) {
		return false, nil
	}
	body, err := clean.MarshalJSON()
	if err != nil {
		return false, errors.New("cannot encode sanitized software snapshot")
	}
	pipeline, err := e.pipeline(ctx, "software")
	if err != nil {
		return false, err
	}
	if err := pipeline.pipeline.Event(ctx, eventplatform.EventTypeSoftwareInventory, body, time.Unix(0, ref.Offset.Nanoseconds())); err != nil {
		return false, errors.New("cannot regenerate software wire evidence")
	}
	if err := pipeline.pipeline.Wait(ctx); err != nil {
		return false, errors.New("software wire evidence did not complete")
	}
	if err := e.writer.Append(ref, clean, pipeline.recorder.Drain()); err != nil {
		return false, err
	}
	for _, entry := range clean.Metadata.Software {
		e.profile.SoftwareNames = appendUnique(e.profile.SoftwareNames, entry.DisplayName)
	}
	return true, nil
}

func (e *Evidence) group(ctx context.Context, record tc.Record, ref bundle.SampleRef) (bool, error) {
	if record.Payload.Metadata != nil || record.Payload.Inventory != nil || record.Payload.Software != nil || len(record.Payload.Series) != 0 {
		return false, errors.New("invalid group evidence shape")
	}
	messages, err := processapi.DecodeCaptureGroup(&record)
	if err != nil {
		return false, err
	}
	retain := e.retainCycle(ref.Stream, ref.Offset)
	sanitizer := e.sanitizer
	if !retain {
		// Omitted cycles still undergo complete semantic validation, but their
		// identities must not grow the session's cross-stream PID mapping.
		sanitizer, err = capture.NewSanitizer()
		if err != nil {
			return false, errors.New("cannot initialize discarded group validation")
		}
	}
	clean := make([]model.MessageBody, 0, len(messages))
	total := 0
	for _, message := range messages {
		switch value := message.(type) {
		case *model.CollectorProc:
			body := sanitizer.Process(value)
			if body.Info == nil || body.Info.TotalMemory <= 0 {
				return false, errors.New("process group lacks system information")
			}
			if e.profile.MemoryBytes != 0 && e.profile.MemoryBytes != uint64(body.Info.TotalMemory) {
				return false, errors.New("process memory capacity changed during capture")
			}
			e.profile.MemoryBytes = uint64(body.Info.TotalMemory)
			for _, process := range body.Processes {
				process.CreateTime -= e.session.Origin.UnixMilli()
			}
			total += len(body.Processes)
			clean = append(clean, body)
		case *model.CollectorConnections:
			body := sanitizer.Connections(value)
			if body == nil {
				return false, errors.New("cannot sanitize connection DNS evidence")
			}
			total += len(body.Connections)
			clean = append(clean, body)
		}
	}
	for _, body := range clean {
		if err := e.validateSample(ref.Stream, body); err != nil {
			return false, err
		}
	}
	if total == 0 {
		return false, nil
	}
	if !retain {
		return false, nil
	}
	check := checks.ProcessCheckName
	if record.Stream == tc.Connections {
		check = checks.ConnectionsCheckName
	}
	pipeline, err := e.pipeline(ctx, "group")
	if err != nil {
		return false, err
	}
	if err := pipeline.pipeline.Group(ctx, time.Unix(0, ref.Offset.Nanoseconds()), check, "capture-host", clean); err != nil {
		return false, errors.New("cannot regenerate complete group wire evidence")
	}
	if err := pipeline.pipeline.Wait(ctx); err != nil {
		return false, errors.New("group wire evidence did not complete")
	}
	references := pipeline.recorder.Drain()
	if len(references) != len(clean) {
		return false, errors.New("regenerated group has incomplete wire evidence")
	}
	ordered := make([]bundle.WireReference, len(clean))
	seen := make([]bool, len(clean))
	for _, reference := range references {
		id, err := strconv.ParseUint(reference.Headers.Get(headers.RequestIDHeader), 10, 64)
		index := int(id & ((1 << 14) - 1))
		if err != nil || index >= len(clean) || seen[index] {
			return false, errors.New("regenerated wire chunk order is invalid")
		}
		decoded, err := model.DecodeMessage(reference.Body)
		if err != nil {
			return false, errors.New("regenerated wire group cannot be decoded")
		}
		expected, _ := json.Marshal(clean[index])
		actual, _ := json.Marshal(decoded.Body)
		if string(expected) != string(actual) {
			return false, errors.New("regenerated group differs from sanitized sample")
		}
		ordered[index], seen[index] = reference, true
	}
	ref.ChunkCount = len(clean)
	for i, body := range clean {
		ref.ChunkIndex = i
		if err := e.writer.Append(ref, body, []bundle.WireReference{ordered[i]}); err != nil {
			return false, err
		}
		switch value := body.(type) {
		case *model.CollectorProc:
			for _, process := range value.Processes {
				if process.Command != nil {
					e.profile.ProcessNames = appendUnique(e.profile.ProcessNames, process.Command.Comm)
				}
			}
		case *model.CollectorConnections:
			for _, connection := range value.Connections {
				e.profile.ConnectionSelectors = appendUnique(e.profile.ConnectionSelectors, telemetry.ConnectionSelector(connection))
			}
		}
	}
	return true, nil
}

func appendUnique(values []string, value string) []string {
	if !slices.Contains(values, value) {
		return append(values, value)
	}
	return values
}

func (e *Evidence) validateSample(stream schema.Stream, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return errors.New("invalid sanitized evidence sample")
	}
	decode := telemetry.Decode
	if stream == schema.Processes || stream == schema.Connections {
		decode = telemetry.DecodeGroupChunk
	}
	sample, err := decode(stream, data)
	if err != nil {
		return errors.New("invalid sanitized evidence sample")
	}
	osname := ""
	if sample.HostMetadata != nil {
		osname = sample.HostMetadata.OS
	}
	if sample.Processes != nil {
		osname = sample.Processes.Info.Os.Name
	}
	if sample.Inventory != nil && sample.Inventory.Host != nil {
		osname = strings.ToLower(sample.Inventory.Host.KernelName)
	}
	if osname != "" && ((e.profile.OS == "macos" && osname != "darwin") || (e.profile.OS == "windows" && osname != "windows" && osname != "win32")) {
		return errors.New("observed platform differs from capture platform")
	}
	return nil
}

// Retain metric flushes throughout capture so slow check families are not lost.
// The session timeout and cycle limit bound retention; other streams retain
// their minimum usable coverage while still validating all accepted records.
func (e *Evidence) retainCycle(stream schema.Stream, offset time.Duration) bool {
	if stream == schema.HostMetadata || stream == schema.AgentInventory || stream == schema.HostInventory || stream == schema.HostSystemInfo || stream == schema.Software {
		return len(e.offsets[stream]) == 0
	}
	return (stream == schema.Metrics || len(e.offsets[stream]) < 2) && !slices.Contains(e.offsets[stream], offset)
}

func (e *Evidence) observedCadence(stream schema.Stream) time.Duration {
	offsets := slices.Clone(e.offsets[stream])
	slices.Sort(offsets)
	for i := 1; i < len(offsets); i++ {
		if offsets[i] > offsets[i-1] {
			return offsets[i] - offsets[i-1]
		}
	}
	return 0
}

func (e *Evidence) Coverage() (bool, string) {
	var missing []string
	for _, stream := range []schema.Stream{schema.Metrics, schema.Processes, schema.HostMetadata, schema.AgentInventory, schema.HostInventory, schema.HostSystemInfo, schema.Software, schema.Connections} {
		if stream == schema.Connections && !slices.Contains(e.profile.Streams, schema.Connections) && e.profile.OS != "windows" {
			continue
		}
		if stream == schema.HostSystemInfo && !slices.Contains(e.profile.Streams, stream) {
			continue
		}
		required := 1
		if stream == schema.Metrics || stream == schema.Processes || stream == schema.Connections {
			required = 2
		}
		count := len(e.offsets[stream])
		if count < required || (required == 2 && e.observedCadence(stream) <= 0) {
			missing = append(missing, fmt.Sprintf("%s: %d/%d cycles, effective cadence %s", stream, count, required, e.cadences[stream]))
		}
	}
	for _, family := range slices.Sorted(maps.Keys(e.metricCadences)) {
		if count := len(e.metricOffsets[family]); count < 2 {
			missing = append(missing, fmt.Sprintf("metrics/%s: %d/2 cycles, effective cadence %s", family, count, e.metricCadences[family]))
		}
	}
	return len(missing) == 0, strings.Join(missing, "; ")
}

func (e *Evidence) Finish(ctx context.Context, statuses []tc.Status, ended time.Time) (err error) {
	defer e.Close()
	if e.closed || e.writer == nil || ended.Before(e.session.Origin) {
		return errors.New("evidence session cannot finalize")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if complete, detail := e.Coverage(); !complete {
		return fmt.Errorf("incomplete live coverage: %s", detail)
	}
	if len(statuses) != len(e.participants) {
		return errors.New("missing evidence producer stop acknowledgement")
	}
	var producers []bundle.Producer
	seen := map[string]bool{}
	for _, status := range statuses {
		participant, ok := e.participants[status.Producer.InstanceID]
		if !ok || seen[status.Producer.InstanceID] || status.Producer != participant.Status.Producer || status.ProtocolVersion != tc.ProtocolVersion ||
			status.SessionID != e.session.ID || status.State != tc.Stopped || !status.ActivatedAt.Equal(participant.Status.ActivatedAt) ||
			status.StoppedAt.Before(status.ActivatedAt) || status.StoppedAt.After(ended) || status.FinalSequence != status.Acknowledged || status.FinalSequence < e.sequences[status.Producer.InstanceID] || status.Failures != 0 || status.Drops != 0 {
			return errors.New("invalid evidence producer stopped acknowledgement")
		}
		seen[status.Producer.InstanceID] = true
		producer := bundle.Producer{Role: status.Producer.Role, InstanceID: status.Producer.InstanceID, Version: status.Producer.Version, Commit: status.Producer.Commit,
			ProtocolVersion: status.ProtocolVersion, StartOffset: status.ActivatedAt.Sub(e.session.Origin), StopOffset: status.StoppedAt.Sub(e.session.Origin),
			FinalSequence: status.FinalSequence, AcknowledgedSequence: status.Acknowledged, Stopped: true}
		for _, stream := range participant.Streams {
			producer.Streams = append(producer.Streams, evidenceStream(stream))
		}
		producers = append(producers, producer)
	}
	for _, stream := range []schema.Stream{schema.Metrics, schema.Processes, schema.Connections} {
		if cadence := e.observedCadence(stream); cadence > 0 {
			e.cadences[stream] = cadence
		}
	}
	slices.Sort(e.profile.MetricNames)
	slices.Sort(e.profile.ProcessNames)
	slices.Sort(e.profile.SoftwareNames)
	slices.Sort(e.profile.ConnectionSelectors)
	if err := e.writer.SetProducers(producers); err != nil {
		return err
	}
	_, err = e.writer.CompleteContext(ctx, ended.Sub(e.session.Origin), e.profile, e.cadences)
	return err
}

// Close releases all ephemeral pipeline, recorder and sanitizer state. It does
// not write a completion marker and is safe after cancellation or failure.
func (e *Evidence) Close() {
	if e.closed {
		return
	}
	e.closed = true
	for _, pipeline := range e.pipelines {
		pipeline.pipeline.Close()
		pipeline.recorder.Drain()
	}
	e.pipelines = nil
	e.sanitizer = nil
	e.cycles = nil
	e.sequences = nil
	e.participants = nil
}
