// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package overlay

import (
	"errors"
	"fmt"
	"math"
	"slices"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/schema"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/telemetry"
)

const megabyte = 1024 * 1024

// Apply mutates only the supplied owned sample, before identity rewriting. An
// omitted overlay preserves captured evidence, including during recovery.
func Apply(ctx Context, sample *telemetry.Sample) error {
	if sample == nil || ctx.Scenario == nil || ctx.PhaseIndex < 0 || ctx.PhaseIndex >= len(ctx.Scenario.Phases) {
		return errors.New("overlay requires a sample and valid phase")
	}
	phase := ctx.Scenario.Phases[ctx.PhaseIndex]
	switch ctx.Stream {
	case schema.Processes:
		if sample.Processes == nil {
			return errors.New("process overlay requires a process sample")
		}
		return applyProcesses(ctx, sample.Processes, phase.Processes[ctx.Group.Group])
	case schema.Metrics:
		return applyMetrics(ctx, sample, phase)
	case schema.Software:
		return applySoftware(ctx, sample)
	case schema.Connections:
		return applyConnections(ctx, sample, phase.Connections[ctx.Group.Group])
	case schema.HostMetadata:
		return nil
	default:
		return fmt.Errorf("unsupported overlay stream %q", ctx.Stream)
	}
}

func applyMetrics(ctx Context, sample *telemetry.Sample, phase schema.Phase) error {
	var userDelta, systemDelta, memoryDelta, totalMemory float64
	for _, def := range phase.Processes[ctx.Group.Group] {
		changes, err := processChanges(ctx, def)
		if err != nil {
			return err
		}
		for _, change := range changes {
			userDelta += (float64(change.cpu.UserPct) - float64(change.before.Cpu.UserPct)) / change.cpus
			systemDelta += (float64(change.cpu.SystemPct) - float64(change.before.Cpu.SystemPct)) / change.cpus
			memoryDelta += (float64(change.rss) - float64(change.before.Memory.Rss)) / megabyte
			totalMemory = change.totalMemory / megabyte
		}
	}
	for _, serie := range sample.Metrics {
		if serie == nil {
			return errors.New("nil captured metric")
		}
		pattern, explicit := phase.Metrics[ctx.Group.Group][serie.Name]
		if explicit && len(phase.Processes[ctx.Group.Group]) != 0 && reconciledMetric(serie.Name) {
			return fmt.Errorf("explicit %s conflicts with process resource reconciliation", serie.Name)
		}
		for i := range serie.Points {
			value := serie.Points[i].Value
			if explicit {
				var err error
				value, err = PatternValue(ctx, pattern, "metric/"+serie.Name)
				if err != nil {
					return err
				}
			} else {
				switch serie.Name {
				case "system.cpu.user":
					value += userDelta
				case "system.cpu.system":
					value += systemDelta
				case "system.cpu.idle":
					value -= userDelta + systemDelta
				case "system.mem.used":
					value += memoryDelta
				case "system.mem.free", "system.mem.usable":
					value -= memoryDelta
				case "system.mem.pct_usable":
					if totalMemory > 0 {
						value -= memoryDelta / totalMemory
					}
				}
			}
			if explicit || len(phase.Processes[ctx.Group.Group]) != 0 && reconciledMetric(serie.Name) {
				if !metricValueValid(serie.Name, value) {
					return fmt.Errorf("overlay exceeds captured resource capacity for %s", serie.Name)
				}
				serie.Points[i].Value = value
			}
		}
	}
	return nil
}

func reconciledMetric(name string) bool {
	return slices.Contains([]string{"system.cpu.user", "system.cpu.system", "system.cpu.idle", "system.mem.used", "system.mem.free", "system.mem.usable", "system.mem.pct_usable"}, name)
}

func metricValueValid(name string, value float64) bool {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return false
	}
	switch name {
	case "system.wlan.rssi", "system.wlan.noise":
		return value >= -120 && value <= 0
	case "system.battery.charge_rate":
		return true
	case "system.cpu.user", "system.cpu.system", "system.cpu.idle", "system.disk.utilized", "system.battery.maximum_capacity_pct", "system.battery.current_charge_pct":
		return value >= 0 && value <= 100
	case "system.mem.pct_usable":
		return value >= 0 && value <= 1
	default:
		return value >= 0
	}
}

func applyConnections(ctx Context, sample *telemetry.Sample, overlays []schema.ConnectionOverlay) error {
	if len(overlays) == 0 {
		return nil
	}
	if sample.Connections == nil {
		return errors.New("connection overlay requires captured connections")
	}
	for _, overlay := range overlays {
		for _, conn := range sample.Connections.Connections {
			if conn == nil || telemetry.ConnectionSelector(conn) != overlay.Selector {
				continue
			}
			if conn.Type != model.ConnectionType_tcp {
				return errors.New("connection overlay requires a captured TCP path")
			}
			fields := []struct {
				pattern *schema.Pattern
				name    string
				scale   float64
				target  *uint32
			}{
				{overlay.RTTMilliseconds, "rtt", 1000, &conn.Rtt},
				{overlay.RTTVarianceMilliseconds, "rtt_variance", 1000, &conn.RttVar},
				{overlay.Retransmits, "retransmits", 1, &conn.LastRetransmits},
			}
			for _, field := range fields {
				if field.pattern == nil {
					continue
				}
				value, err := connectionValue(ctx, *field.pattern, overlay.Selector+"/"+field.name, field.scale)
				if err != nil {
					return err
				}
				*field.target = value
			}
			for code, pattern := range overlay.TCPFailures {
				if !slices.Contains([]uint32{104, 110, 111, 125}, code) {
					return fmt.Errorf("unsupported standardized TCP error code %d", code)
				}
				value, err := connectionValue(ctx, pattern, fmt.Sprintf("%s/failure/%d", overlay.Selector, code), 1)
				if err != nil {
					return err
				}
				if conn.TcpFailuresByErrCode == nil {
					conn.TcpFailuresByErrCode = make(map[uint32]uint32)
				}
				conn.TcpFailuresByErrCode[code] = value
			}
		}
	}
	return nil
}

func connectionValue(ctx Context, pattern schema.Pattern, field string, scale float64) (uint32, error) {
	value, err := PatternValue(ctx, pattern, "connection/"+field)
	if err != nil {
		return 0, err
	}
	value = math.Round(value * scale)
	if value < 0 || value > math.MaxUint32 {
		return 0, errors.New("connection overlay exceeds Agent field range")
	}
	return uint32(value), nil
}
