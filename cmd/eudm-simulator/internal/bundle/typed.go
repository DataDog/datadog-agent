// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package bundle

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/schema"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/telemetry"
	tc "github.com/DataDog/datadog-agent/pkg/telemetrycapture"
)

type typedGroupEvidence struct {
	id      int32
	records int
}

func checkGroup(groups map[cycleKey]*typedGroupEvidence, ref SampleRef, id, size int32, records int) error {
	if int(size) != ref.ChunkCount {
		return errors.New("typed group size differs from capture cycle evidence")
	}
	key := cycleKey{producer: ref.ProducerID, cycle: ref.CycleID}
	group := groups[key]
	if group == nil {
		group = &typedGroupEvidence{id: id}
		groups[key] = group
	}
	if group.id != id {
		return errors.New("typed group identifiers differ within a capture cycle")
	}
	group.records += records
	return nil
}

// validateTyped reconciles the manifest's evidence claims with actual decoded
// samples. Digests alone prove integrity, not the presence of usable telemetry.
func (b *Loaded) validateTyped() error {
	b.Samples = make(map[string]*telemetry.Sample, len(b.Manifest.Samples))
	names := map[string]map[string]bool{
		"metric_names": {}, "process_names": {}, "software_names": {}, "connection_selectors": {},
	}
	profile := b.Manifest.Profile
	producerVersions := map[string]string{}
	for _, producer := range b.Manifest.Producers {
		producerVersions[producer.InstanceID] = producer.Version
	}
	groups := map[cycleKey]*typedGroupEvidence{}
	var hostname, uuid string
	var logicalCPUs uint64
	checkUUID := func(value string) error {
		if value == "" {
			return nil // Legacy host metadata may omit this optional field.
		}
		if uuid != "" && value != uuid {
			return errors.New("capture inventories contain inconsistent UUID identities")
		}
		uuid = value
		return nil
	}
	checkLogicalCPUs := func(value uint64) error {
		if value == 0 || (logicalCPUs != 0 && logicalCPUs != value) {
			return errors.New("host metadata and inventory disagree about logical CPU resources")
		}
		logicalCPUs = value
		return nil
	}
	checkHost := func(value string) error {
		if hostname != "" && value != hostname {
			return errors.New("capture samples contain inconsistent host identities")
		}
		hostname = value
		return nil
	}
	platform := func(value string) string {
		switch value {
		case "darwin":
			return "macos"
		case "windows", "win32":
			return "windows"
		default:
			return ""
		}
	}
	for _, ref := range b.Manifest.Samples {
		decode := telemetry.Decode
		if ref.ChunkCount > 1 {
			decode = telemetry.DecodeGroupChunk
		}
		sample, err := decode(ref.Stream, b.Files[ref.File])
		if err != nil {
			return fmt.Errorf("sample %q (%s): %w", ref.File, ref.Stream, err)
		}
		switch ref.Stream {
		case schema.Metrics:
			for _, serie := range sample.Metrics {
				if err := checkHost(serie.Host); err != nil {
					return err
				}
				names["metric_names"][serie.Name] = true
			}
		case schema.HostMetadata:
			value := sample.HostMetadata
			if value.AgentVersion != producerVersions[ref.ProducerID] {
				return errors.New("host metadata version differs from its producing Agent build")
			}
			if err := checkHost(value.Hostname); err != nil {
				return err
			}
			if err := checkUUID(value.UUID); err != nil {
				return err
			}
			if raw, present := value.SystemStats["cpuCores"]; present {
				var cores uint64
				if json.Unmarshal(raw, &cores) != nil || checkLogicalCPUs(cores) != nil {
					return errors.New("invalid or inconsistent host metadata logical CPU count")
				}
			}
			if platform(value.OS) != profile.OS {
				return errors.New("host metadata operating system differs from capture profile")
			}
			if raw, present := value.SystemStats["machine"]; present {
				var machine string
				if err := json.Unmarshal(raw, &machine); err != nil {
					return errors.New("invalid host metadata architecture")
				}
				switch machine {
				case "x86_64":
					machine = "amd64"
				case "aarch64":
					machine = "arm64"
				}
				if machine != profile.Architecture {
					return errors.New("host metadata architecture differs from capture profile")
				}
			}
		case schema.AgentInventory, schema.HostInventory, schema.HostSystemInfo:
			value := sample.Inventory
			if err := checkHost(value.Hostname); err != nil {
				return err
			}
			if err := checkUUID(value.UUID); err != nil {
				return err
			}
			if value.Timestamp > int64(b.Manifest.Duration) {
				return errors.New("inventory timestamp lies outside the capture duration")
			}
			if value.Agent != nil {
				if value.Agent.AgentVersion != producerVersions[ref.ProducerID] {
					return errors.New("Agent inventory version differs from its producing Agent build")
				}
			} else if value.Host != nil {
				host := value.Host
				if host.AgentVersion != producerVersions[ref.ProducerID] || telemetry.InventoryPlatform(host.OS) != profile.OS || host.MemoryTotalKb != profile.MemoryBytes/1024 {
					return errors.New("host inventory build, platform, or memory differs from captured evidence")
				}
				if err := checkLogicalCPUs(host.CPULogicalProcessors); err != nil {
					return err
				}
				if host.CPUArchitecture != "" {
					architecture := strings.ToLower(host.CPUArchitecture)
					switch architecture {
					case "x86_64", "x64":
						architecture = "amd64"
					case "aarch64":
						architecture = "arm64"
					}
					if architecture != profile.Architecture {
						return errors.New("host inventory architecture differs from capture profile")
					}
				}
			}
		case schema.Processes:
			value := sample.Processes
			if err := checkGroup(groups, ref, value.GroupId, value.GroupSize, len(value.Processes)); err != nil {
				return err
			}
			if err := checkHost(value.HostName); err != nil {
				return err
			}
			if platform(value.Info.Os.Name) != profile.OS || uint64(value.Info.TotalMemory) != profile.MemoryBytes {
				return errors.New("process system information differs from capture profile")
			}
			for _, process := range value.Processes {
				names["process_names"][process.Command.Comm] = true
			}
		case schema.Connections:
			value := sample.Connections
			if err := checkGroup(groups, ref, value.GroupId, value.GroupSize, len(value.Connections)); err != nil {
				return err
			}
			if err := checkHost(sample.Connections.HostName); err != nil {
				return err
			}
			for _, connection := range sample.Connections.Connections {
				names["connection_selectors"][telemetry.ConnectionSelector(connection)] = true
			}
		case schema.Software:
			if err := checkHost(sample.Software.Hostname); err != nil {
				return err
			}
			allowed := []string{"", "os"}
			if profile.OS == "windows" {
				allowed = append(allowed, "desktop", "msstore", "msi", "driver")
			} else {
				allowed = append(allowed, "app", "system_app", "homebrew", "pkg", "macports", "mas", "kext", "sysext")
			}
			for _, software := range sample.Software.Metadata.Software {
				if !slices.Contains(allowed, software.Source) {
					return errors.New("software entry type is unsupported on captured platform")
				}
				names["software_names"][software.DisplayName] = true
			}
		}
		b.Samples[ref.File] = sample
	}
	for _, group := range groups {
		if group.records == 0 {
			return errors.New("complete captured group contains no records")
		}
	}
	for field, declared := range map[string][]string{
		"metric_names": profile.MetricNames, "process_names": profile.ProcessNames,
		"software_names": profile.SoftwareNames, "connection_selectors": profile.ConnectionSelectors,
	} {
		seen := map[string]bool{}
		for _, name := range declared {
			if name == "" || seen[name] || !names[field][name] {
				return fmt.Errorf("capture profile %s differs from observed typed samples", field)
			}
			seen[name] = true
		}
		if len(seen) != len(names[field]) {
			return fmt.Errorf("capture profile %s omits observed typed samples", field)
		}
	}
	return nil
}

// validateMetricCoverage rejects complete-looking bundles that omit slow checks.
func (b *Loaded) validateMetricCoverage() error {
	cadences := b.Manifest.MetricCadences
	if len(cadences) == 0 || len(cadences) > 7 {
		return errors.New("missing or invalid metric family schedule; recapture")
	}
	cycles := map[string]map[cycleKey]bool{}
	for family, cadence := range cadences {
		if tc.MetricCheckFamily(family) == "" || cadence <= 0 {
			return errors.New("invalid metric family cadence")
		}
		cycles[family] = map[cycleKey]bool{}
	}
	for _, ref := range b.Manifest.Samples {
		if ref.Stream != schema.Metrics {
			continue
		}
		for _, serie := range b.Samples[ref.File].Metrics {
			family := tc.MetricFamily(serie.Name)
			if cycles[family] == nil {
				return errors.New("metric evidence lacks a scheduled family cadence")
			}
			cycles[family][cycleKey{producer: ref.ProducerID, cycle: ref.CycleID}] = true
		}
	}
	for family, observed := range cycles {
		if len(observed) < 2 {
			return fmt.Errorf("metric family %s lacks two observed cycles; recapture", family)
		}
	}
	return nil
}
