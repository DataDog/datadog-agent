// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package capture

import (
	"slices"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/inventory/software"
)

// Software copies the complete snapshot, including applications not named by
// a scenario. Fields not explicitly included here are deliberately discarded.
func (s *Sanitizer) Software(entries []software.Entry) []software.Entry {
	result := make([]software.Entry, 0, len(entries))
	for _, entry := range entries {
		name := entry.DisplayName
		if !slices.Contains([]string{"Google Chrome", "SentinelOne", "Sentinel Agent", "OS"}, name) {
			name = s.token("software", name)
		}
		publisher := entry.Publisher
		if !slices.Contains([]string{"Google LLC", "Google, Inc.", "SentinelOne", "Apple Inc.", "Microsoft Corporation"}, publisher) {
			publisher = s.token("publisher", publisher)
		}
		source := entry.Source
		if !slices.Contains([]string{"desktop", "msstore", "msi", "app", "homebrew", "pkg", "macports", "mas", "os", "driver", "kext", "sysext"}, source) {
			source = ""
		}
		status := entry.Status
		if !slices.Contains([]string{"installed", "absent", "pending_install", "pending_removal", "uninstalling", "failed", "broken"}, status) {
			status = ""
		}
		version := safeVersion(entry.Version)
		if source == "os" {
			version = safeOSVersion(entry.Version)
		}
		clean := software.Entry{DisplayName: name, Version: version, Source: source, Publisher: publisher, Status: status, Is64Bit: entry.Is64Bit, ProductCode: s.token("product", entry.ProductCode), UserSID: s.token("user", entry.UserSID)}
		for _, path := range entry.InstallPaths {
			prefix := "/capture/software/"
			windowsPath := strings.TrimPrefix(strings.ReplaceAll(path, "/", `\`), `\\?\`)
			if len(windowsPath) >= 3 && ((windowsPath[0] >= 'A' && windowsPath[0] <= 'Z') || (windowsPath[0] >= 'a' && windowsPath[0] <= 'z')) && windowsPath[1:3] == `:\` {
				prefix = `C:\capture\software\`
			} else if strings.HasPrefix(windowsPath, `\\`) || strings.HasPrefix(windowsPath, `UNC\`) {
				prefix = `\\capture-server\software\`
			}
			clean.InstallPaths = append(clean.InstallPaths, prefix+s.token("path", path))
		}
		result = append(result, clean)
	}
	return result
}
