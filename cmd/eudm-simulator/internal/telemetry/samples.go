// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package telemetry

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"slices"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/schema"
	softwareimpl "github.com/DataDog/datadog-agent/comp/softwareinventory/impl"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/telemetrycapture"
)

// HostMetadata retains native host telemetry without the legacy API key or
// configuration/management credentials. Device identities are rewritten at replay.
type HostMetadata struct {
	AgentVersion     string                              `json:"agentVersion"`
	UUID             string                              `json:"uuid"`
	Hostname         string                              `json:"internalHostname"`
	OS               string                              `json:"os"`
	AgentFlavor      string                              `json:"agent-flavor"`
	PythonVersion    string                              `json:"python"`
	InstallMethod    *telemetrycapture.HostInstallMethod `json:"install-method,omitempty"`
	Logs             *telemetrycapture.HostLogsMetadata  `json:"logs,omitempty"`
	OTLP             map[string]bool                     `json:"otlp,omitempty"`
	FIPSMode         bool                                `json:"fips_mode"`
	FIPSProxyEnabled bool                                `json:"fips_proxy_enabled"`
	ContainerMeta    map[string]string                   `json:"container-meta,omitempty"`
	Proxy            *telemetrycapture.HostProxyMetadata `json:"proxy-info,omitempty"`
	SystemStats      map[string]json.RawMessage          `json:"systemStats,omitempty"`
	Meta             map[string]json.RawMessage          `json:"meta"`
	Network          map[string]string                   `json:"network,omitempty"`
	HostTags         map[string][]string                 `json:"host-tags"`
	Gohai            string                              `json:"gohai,omitempty"`
}

// MarshalJSON satisfies the Agent host metadata serialization boundary.
func (h *HostMetadata) MarshalJSON() ([]byte, error) {
	type plain HostMetadata
	return json.Marshal((*plain)(h))
}

// Sample holds exactly one decoded Agent output, selected by the declared stream.
type Sample struct {
	Metrics      []*metrics.Serie
	HostMetadata *HostMetadata
	Inventory    *telemetrycapture.Inventory
	Processes    *model.CollectorProc
	Connections  *model.CollectorConnections
	Software     *softwareimpl.Payload
}

// Decode rejects malformed, empty, unknown-field, and wrong-stream samples.
// Platform and profile consistency are checked separately against the manifest.
func Decode(stream schema.Stream, data []byte) (*Sample, error) {
	return decodeSample(stream, data, false)
}

// DecodeGroupChunk permits an empty process or connection chunk inside a
// validated multi-chunk group. The caller must verify that the complete group
// contains records, and must use Decode for singleton samples.
func DecodeGroupChunk(stream schema.Stream, data []byte) (*Sample, error) {
	if stream != schema.Processes && stream != schema.Connections {
		return nil, errors.New("group chunks require a process or connection stream")
	}
	return decodeSample(stream, data, true)
}

func decodeSample(stream schema.Stream, data []byte, allowEmptyChunk bool) (*Sample, error) {
	sample := &Sample{}
	switch stream {
	case schema.Metrics:
		var value MetricSample
		if err := decode(data, &value); err != nil {
			return nil, err
		}
		series, err := value.AgentSeries()
		if err != nil {
			return nil, err
		}
		sample.Metrics = series
	case schema.HostMetadata:
		var value HostMetadata
		if err := decode(data, &value); err != nil {
			return nil, err
		}
		if value.Hostname == "" || value.AgentVersion == "" || !slices.Contains([]string{"darwin", "windows", "win32"}, value.OS) {
			return nil, errors.New("host metadata lacks host, version, or supported operating system")
		}
		if value.Gohai != "" {
			var nested map[string]json.RawMessage
			if err := decode([]byte(value.Gohai), &nested); err != nil || nested == nil {
				return nil, errors.New("host metadata contains invalid gohai JSON")
			}
		}
		sample.HostMetadata = &value
	case schema.AgentInventory, schema.HostInventory, schema.HostSystemInfo:
		var value telemetrycapture.Inventory
		if err := decode(data, &value); err != nil {
			return nil, err
		}
		if err := validateInventory(stream, &value); err != nil {
			return nil, err
		}
		sample.Inventory = &value
	case schema.Processes:
		var value model.CollectorProc
		if err := decodeProto(data, &value); err != nil {
			return nil, err
		}
		if value.HostName == "" || value.Info == nil || value.Info.TotalMemory <= 0 || value.Info.Os == nil || !slices.Contains([]string{"darwin", "windows"}, value.Info.Os.Name) || (!allowEmptyChunk && len(value.Processes) == 0) {
			return nil, errors.New("process sample lacks host, system information, or processes")
		}
		for _, process := range value.Processes {
			if process == nil || process.Command == nil || process.Command.Comm == "" {
				return nil, errors.New("process sample contains an unnamed or nil process")
			}
		}
		sample.Processes = &value
	case schema.Connections:
		var value model.CollectorConnections
		if err := decodeProto(data, &value); err != nil {
			return nil, err
		}
		if value.HostName == "" || (!allowEmptyChunk && len(value.Connections) == 0) {
			return nil, errors.New("connection sample lacks host or connections")
		}
		for _, connection := range value.Connections {
			if connection == nil || connection.Laddr == nil || connection.Raddr == nil {
				return nil, errors.New("connection sample contains a nil connection or address")
			}
			for _, addr := range []*model.Addr{connection.Laddr, connection.Raddr} {
				if _, err := netip.ParseAddr(addr.Ip); err != nil || addr.Port < 0 || addr.Port > 65535 {
					return nil, errors.New("connection sample contains an invalid address")
				}
			}
		}
		sample.Connections = &value
	case schema.Software:
		var value softwareimpl.Payload
		if err := decode(data, &value); err != nil {
			return nil, err
		}
		if value.Hostname == "" || len(value.Metadata.Software) == 0 {
			return nil, errors.New("software snapshot lacks host or software entries")
		}
		for _, software := range value.Metadata.Software {
			if software.DisplayName == "" {
				return nil, errors.New("software snapshot contains an unnamed entry")
			}
		}
		sample.Software = &value
	default:
		return nil, errors.New("unsupported capture stream")
	}
	return sample, nil
}

// ConnectionSelector identifies the captured record that an overlay can address.
func ConnectionSelector(conn *model.Connection) string {
	return fmt.Sprintf("%d:%s:%d>%s:%d", conn.Pid, conn.GetLaddr().GetIp(), conn.GetLaddr().GetPort(), conn.GetRaddr().GetIp(), conn.GetRaddr().GetPort())
}

func decode(data []byte, value any) error {
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return errors.New("typed sample cannot be null")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return fmt.Errorf("invalid typed sample: %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return errors.New("expected exactly one sample JSON document")
	}
	return nil
}
