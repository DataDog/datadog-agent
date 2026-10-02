// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package utils

import "strings"

// GohaiFields borrows the original collection's semantic sections until the
// serializer reserves and copies its projection. Filesystems and processes are
// deliberately absent. Gohai's AsJSON methods expose these scalar values as
// strings inside map[string]any; nested values are never capture inputs.
type GohaiFields struct {
	CPU, Memory, Platform, Network any
}

var gohaiCaptureKeys = [...]struct {
	section string
	keys    []string
}{
	{"cpu", []string{"cpu_cores", "cpu_logical_processors", "cpu_pkgs", "cpu_numa_nodes", "cache_size", "cache_size_l1", "cache_size_l2", "cache_size_l3", "mhz", "model_name", "vendor_id", "model", "family", "stepping"}},
	{"memory", []string{"total", "swap_total"}},
	{"platform", []string{"hostname", "serial_number", "hardware_uuid", "machine", "hardware_platform", "GOOARCH", "processor", "kernel_name", "os", "GOOS", "kernel_release", "family"}},
	{"network", []string{"ipaddress", "ipaddressv6", "macaddress"}},
}

func (f GohaiFields) sections() [4]any { return [4]any{f.CPU, f.Memory, f.Platform, f.Network} }

// CaptureGohaiSize charges only selected strings and conservative map storage.
// It does not allocate, encode, or inspect unrelated fields.
func CaptureGohaiSize(source GohaiFields) int64 {
	n := int64(256)
	for i, input := range source.sections() {
		fields, ok := input.(map[string]any)
		if !ok {
			continue
		}
		n += 256 + int64(len(gohaiCaptureKeys[i].section))
		for _, key := range gohaiCaptureKeys[i].keys {
			if value, ok := fields[key].(string); ok {
				n += 256 + int64(len(key)+len(value))
			}
		}
	}
	return n
}

// CopyCaptureGohai owns only the scalar fields consumed by the sanitizer.
// Callers must reserve CaptureGohaiSize before invoking it. This performs no
// JSON processing and cannot retain opaque configuration or interface data.
func CopyCaptureGohai(source GohaiFields) map[string]map[string]string {
	var result map[string]map[string]string
	for i, input := range source.sections() {
		fields, ok := input.(map[string]any)
		if !ok {
			continue
		}
		var values map[string]string
		for _, key := range gohaiCaptureKeys[i].keys {
			if value, ok := fields[key].(string); ok {
				if values == nil {
					values = make(map[string]string)
				}
				values[strings.Clone(key)] = strings.Clone(value)
			}
		}
		if len(values) != 0 {
			if result == nil {
				result = make(map[string]map[string]string)
			}
			result[strings.Clone(gohaiCaptureKeys[i].section)] = values
		}
	}
	return result
}
