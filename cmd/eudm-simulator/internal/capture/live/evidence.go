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
	"strings"
	"time"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/bundle"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/capture"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/schema"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/telemetry"
	softwareimpl "github.com/DataDog/datadog-agent/comp/softwareinventory/impl"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	processapi "github.com/DataDog/datadog-agent/pkg/process/util/api"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	tc "github.com/DataDog/datadog-agent/pkg/telemetrycapture"
)

// Evidence normalizes complete observed cycles and writes typed samples.
// The coordinator validates IPC provenance before handing records to the sink.
type Evidence struct {
	progress       *captureProgress
	directory      string
	tool           bundle.BuildIdentity
	profile        schema.Profile
	session        Session
	normalizer     *capture.Normalizer
	writer         *bundle.Writer
	participants   map[string]Participant
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
	if e.writer != nil || e.closed || session.Origin.IsZero() || session.Duration <= 0 || len(session.Participants) == 0 ||
		(e.profile.OS != "macos" && e.profile.OS != "windows") || (e.profile.Architecture != "amd64" && e.profile.Architecture != "arm64") {
		return errors.New("invalid evidence capture session")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	e.participants = map[string]Participant{}
	e.offsets = map[schema.Stream][]time.Duration{}
	e.cadences = map[schema.Stream]time.Duration{}
	e.metricCadences = map[string]time.Duration{}
	e.metricOffsets = map[string][]time.Duration{}
	owners := map[tc.Stream]bool{}
	latestActivation := false
	for _, participant := range session.Participants {
		status := participant.Status
		if status.ProtocolVersion != tc.ProtocolVersion || status.SessionID != session.ID || status.State != tc.Active ||
			status.ActivatedAt.After(session.Origin) || session.Origin.Sub(status.ActivatedAt) > 5*time.Second || len(participant.Streams) == 0 ||
			e.participants[status.Producer.InstanceID].Status.Producer.InstanceID != "" {
			return errors.New("invalid evidence producer activation")
		}
		latestActivation = latestActivation || status.ActivatedAt.Equal(session.Origin)
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
	}
	if !latestActivation {
		return errors.New("evidence origin must match the latest producer activation")
	}
	e.normalizer = capture.NewNormalizer()
	var err error
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
		!slices.Contains(participant.Streams, record.Stream) ||
		record.CollectedAt.IsZero() || record.ObservedAt.Before(record.CollectedAt) || record.Cadence <= 0 {
		return errors.New("invalid evidence record provenance")
	}
	if record.CollectedAt.Before(e.session.Origin) || !record.ObservedAt.Before(e.session.Origin.Add(e.session.Duration)) {
		return nil
	}
	stream := evidenceStream(record.Stream)
	ref := bundle.SampleRef{Stream: stream, ProducerID: record.Producer.InstanceID, CycleID: record.CycleID, Sequence: record.Sequence,
		Offset: record.CollectedAt.Sub(e.session.Origin), ChunkCount: 1}
	var nonempty bool
	switch record.Stream {
	case tc.Metrics:
		nonempty, err = e.metrics(record, ref)
	case tc.Metadata:
		nonempty, err = e.metadata(record, ref)
	case tc.AgentInventory, tc.HostInventory, tc.HostSystemInfo:
		nonempty, err = e.inventory(record, ref)
	case tc.Software:
		nonempty, err = e.software(record, ref)
	case tc.Processes, tc.Connections:
		nonempty, err = e.group(record, ref)
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

func (e *Evidence) metrics(record tc.Record, ref bundle.SampleRef) (bool, error) {
	if record.Payload.Metadata != nil || record.Payload.Inventory != nil || record.Payload.Software != nil || len(record.Payload.Chunks) != 0 {
		return false, errors.New("invalid metric evidence shape")
	}
	if len(record.Payload.Series) == 0 {
		return false, nil
	}
	var native []*metrics.Serie
	for _, serie := range record.Payload.Series {
		if !tc.MetricAllowed(serie.Name) || uint32(metrics.MetricSource(serie.Source)) != serie.Source {
			return false, errors.New("invalid metric semantic evidence")
		}
		value := &metrics.Serie{Name: serie.Name, Source: metrics.MetricSource(serie.Source), MType: metrics.APIMetricType(serie.Type), Interval: serie.Interval,
			Host: serie.Host, Device: serie.Device, Tags: tagset.CompositeTagsFromSlice(serie.Tags),
			Unit: serie.Unit, SourceTypeName: serie.SourceTypeName, NoIndex: serie.NoIndex}
		for _, resource := range serie.Resources {
			value.Resources = append(value.Resources, metrics.Resource{Type: resource.Type, Name: resource.Name})
		}
		for _, point := range serie.Points {
			value.Points = append(value.Points, metrics.Point{Ts: point.Timestamp - float64(e.session.Origin.UnixNano())/1e9, Value: point.Value})
		}
		native = append(native, value)
	}
	source, err := e.normalizer.Series(capture.NewSeriesSource(native))
	if err != nil {
		return false, errors.New("cannot normalize metric evidence")
	}
	clean := source.(*capture.SeriesSource).Series
	typed, err := telemetry.NewMetricSample(clean)
	if err != nil {
		return false, errors.New("invalid normalized metric evidence")
	}
	if err := e.writer.Append(ref, typed); err != nil {
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
	if e.progress != nil {
		e.progress.event("metrics", fmt.Sprintf("captured %s (%d series)", strings.Join(slices.Sorted(maps.Keys(families)), ", "), len(clean)))
	}
	return true, nil
}

func metadataProjection(in *tc.HostMetadata) *capture.HostMetadata {
	result := &capture.HostMetadata{AgentVersion: in.AgentVersion, UUID: in.UUID, Hostname: in.Hostname, OS: in.OS, AgentFlavor: in.AgentFlavor,
		PythonVersion: in.PythonVersion, InstallMethod: in.InstallMethod, Logs: in.Logs, OTLP: map[string]bool{"enabled": in.OTLPEnabled},
		FIPSMode: in.FIPSMode, FIPSProxyEnabled: in.FIPSProxyEnabled, ContainerMeta: in.ContainerMeta, Proxy: in.Proxy,
		HostTags: in.HostTags, Network: map[string]string{"network-id": in.NetworkID}, SystemStats: map[string]json.RawMessage{}}
	if in.PublicIPv4 != "" {
		result.Network["public-ipv4"] = in.PublicIPv4
	}
	for key, value := range map[string]any{"cpuCores": in.CPUCores, "machine": in.Machine, "platform": in.Platform,
		"pythonV": in.PythonRuntimeVersion, "processor": in.Processor,
		"macV": []any{in.MacVersion, in.MacReleaseInfo, in.MacMachine}, "winV": in.Windows, "nixV": in.UnixVersion, "fbsdV": in.FreeBSDVersion} {
		result.SystemStats[key], _ = json.Marshal(value)
	}
	if in.Meta != nil {
		data, _ := json.Marshal(in.Meta)
		_ = json.Unmarshal(data, &result.Meta)
	}
	result.Gohai = in.Gohai
	return result
}

func (e *Evidence) metadata(record tc.Record, ref bundle.SampleRef) (bool, error) {
	if record.Payload.Metadata == nil || record.Payload.Metadata.Hostname == "" || len(record.Payload.Series) != 0 || len(record.Payload.Chunks) != 0 || record.Payload.Inventory != nil || record.Payload.Software != nil || record.Payload.Metadata.AgentVersion != record.Producer.Version {
		return false, errors.New("invalid host metadata projection")
	}
	clean, err := e.normalizer.HostMetadata(metadataProjection(record.Payload.Metadata))
	if err != nil {
		return false, errors.New("cannot normalize host metadata")
	}
	if err := e.validateSample(ref.Stream, clean); err != nil {
		return false, err
	}
	if err := e.writer.Append(ref, clean); err != nil {
		return false, err
	}
	e.progress.event("host_metadata", "captured host metadata")
	return true, nil
}

func (e *Evidence) inventory(record tc.Record, ref bundle.SampleRef) (bool, error) {
	in := record.Payload.Inventory
	if in == nil || record.Payload.Metadata != nil || record.Payload.Software != nil || len(record.Payload.Series) != 0 || len(record.Payload.Chunks) != 0 ||
		in.Timestamp < record.CollectedAt.UnixNano() || in.Timestamp > record.ObservedAt.UnixNano() ||
		(record.Stream == tc.AgentInventory && (in.Agent == nil || in.Host != nil || in.SystemInfo != nil || in.Agent.AgentVersion != record.Producer.Version)) ||
		(record.Stream == tc.HostInventory && (in.Host == nil || in.Agent != nil || in.SystemInfo != nil || in.Host.AgentVersion != record.Producer.Version)) ||
		(record.Stream == tc.HostSystemInfo && (in.SystemInfo == nil || in.Agent != nil || in.Host != nil)) {
		return false, errors.New("invalid inventory projection or collection boundary")
	}
	clean, err := e.normalizer.Inventory(in, e.session.Origin)
	if err != nil {
		return false, err
	}
	if err := e.validateSample(ref.Stream, clean); err != nil {
		return false, err
	}
	if err := e.writer.Append(ref, clean); err != nil {
		return false, err
	}
	e.progress.event(string(ref.Stream), "captured snapshot")
	return true, nil
}

func (e *Evidence) software(record tc.Record, ref bundle.SampleRef) (bool, error) {
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
	clean := &softwareimpl.Payload{Hostname: native.Hostname, Metadata: softwareimpl.HostSoftware{Software: e.normalizer.Software(native.Metadata.Software)}}
	if err := e.validateSample(ref.Stream, clean); err != nil {
		return false, err
	}
	if err := e.writer.Append(ref, clean); err != nil {
		return false, err
	}
	for _, entry := range clean.Metadata.Software {
		e.profile.SoftwareNames = appendUnique(e.profile.SoftwareNames, entry.DisplayName)
	}
	e.progress.event("software", fmt.Sprintf("captured inventory (%d applications)", len(clean.Metadata.Software)))
	return true, nil
}

func (e *Evidence) group(record tc.Record, ref bundle.SampleRef) (bool, error) {
	if record.Payload.Metadata != nil || record.Payload.Inventory != nil || record.Payload.Software != nil || len(record.Payload.Series) != 0 {
		return false, errors.New("invalid group evidence shape")
	}
	messages, err := processapi.DecodeCaptureGroup(&record)
	if err != nil {
		return false, err
	}
	clean := make([]model.MessageBody, 0, len(messages))
	total := 0
	for _, message := range messages {
		switch value := message.(type) {
		case *model.CollectorProc:
			body := e.normalizer.Process(value)
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
			body := e.normalizer.Connections(value)
			if body == nil {
				return false, errors.New("invalid connection DNS framing")
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
	ref.ChunkCount = len(clean)
	for i, body := range clean {
		ref.ChunkIndex = i
		if err := e.writer.Append(ref, body); err != nil {
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
	e.progress.event(string(ref.Stream), fmt.Sprintf("captured %d %s (%d chunks)", total, ref.Stream, len(clean)))
	return true, nil
}

func appendUnique(values []string, value string) []string {
	if !slices.Contains(values, value) {
		return append(values, value)
	}
	return values
}

func (e *Evidence) validateSample(stream schema.Stream, value any) error {
	data, err := telemetry.Encode(value)
	if err != nil {
		return errors.New("invalid normalized evidence sample")
	}
	decode := telemetry.Decode
	if stream == schema.Processes || stream == schema.Connections {
		decode = telemetry.DecodeGroupChunk
	}
	sample, err := decode(stream, data)
	if err != nil {
		return errors.New("invalid normalized evidence sample")
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
		distinct := count >= 2 && slices.ContainsFunc(e.offsets[stream][1:], func(offset time.Duration) bool { return offset != e.offsets[stream][0] })
		if count < required || (required == 2 && !distinct) {
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
	if e.closed || e.writer == nil || ended.Before(e.session.Origin.Add(e.session.Duration)) {
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
			status.StoppedAt.Before(e.session.Origin.Add(e.session.Duration)) || status.StoppedAt.After(ended) || status.FinalSequence != status.Acknowledged || status.Failures != 0 || status.Drops != 0 {
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
	_, err = e.writer.CompleteContext(ctx, e.session.Duration, e.profile, e.cadences)
	return err
}

// Close releases the normalizer and session state. It does
// not write a completion marker and is safe after cancellation or failure.
func (e *Evidence) Close() {
	if e.closed {
		return
	}
	e.closed = true
	e.normalizer = nil
	e.participants = nil
}
