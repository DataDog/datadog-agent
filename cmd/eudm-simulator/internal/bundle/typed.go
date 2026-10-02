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
)

// validateTyped reconciles the manifest's evidence claims with actual decoded
// samples. Digests alone prove integrity, not the presence of usable telemetry.
func (b *Loaded) validateTyped() error {
	b.Samples = make(map[string]*telemetry.Sample, len(b.Manifest.Samples))
	names := map[string]map[string]bool{
		"metric_names": {}, "process_names": {}, "software_names": {}, "connection_selectors": {},
	}
	profile := b.Manifest.Profile
	var hostname string
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
		sample, err := telemetry.Decode(ref.Stream, b.Files[ref.File])
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
			if err := checkHost(value.Hostname); err != nil {
				return err
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
		case schema.Processes:
			value := sample.Processes
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
			if profile.OS != "windows" {
				return errors.New("connections require a Windows capture")
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
				allowed = append(allowed, "app", "homebrew", "pkg", "macports", "mas", "kext", "sysext")
			}
			for _, software := range sample.Software.Metadata.Software {
				if !slices.Contains(allowed, software.Source) {
					return errors.New("software entry type is unsupported on captured platform")
				}
				names["software_names"][software.DisplayName] = true
			}
		}
		for _, name := range ref.WireFiles {
			var wire WireReference
			if err := DecodeJSON(b.Files[name], &wire); err != nil || len(wire.Body) == 0 || !strings.HasPrefix(wire.Path, "/") || strings.HasPrefix(wire.Path, "//") {
				return fmt.Errorf("sample %q contains an invalid Agent wire reference", ref.File)
			}
		}
		b.Samples[ref.File] = sample
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
