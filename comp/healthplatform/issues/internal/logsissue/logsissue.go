// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package logsissue holds what the logs pipeline health issues share: identity,
// gating and rendering rules. The rules follow missedbytes, which predates it.
package logsissue

import (
	"fmt"
	"hash/fnv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/dustin/go-humanize"

	"github.com/DataDog/datadog-agent/comp/core/config"
)

const (
	// Source is the reporting component, not the scheduler's check source label.
	Source = "logs"
	// Category is the failing subsystem. Not "configuration", which would route the
	// issue to Fleet's Configuration tab.
	Category = "logs_pipeline"
	// Location is where the issue is detected.
	Location = "logs-agent"

	// MaxNameLen bounds one free-form name in Title, Description and Extra.
	MaxNameLen = 64

	// Unknown stands in for a missing or empty value.
	Unknown = "unknown"

	// ASCII on purpose: `agent diagnose` prints Description on non-UTF-8 consoles.
	ellipsis = "..."
)

// LogsEnabled mirrors the logs agent's own gate, which still honours the deprecated
// log_enabled (comp/logs/agent/impl/agent.go).
func LogsEnabled(cfg config.Component) bool {
	return cfg.GetBool("logs_enabled") || cfg.GetBool("log_enabled")
}

// HostIssueID scopes an issue id to this host. The backend dedups on id alone, so
// without the digest one host going clean resolves the issue for every host.
func HostIssueID(base, hostname string) string {
	h := fnv.New64a()
	h.Write([]byte(hostname)) // never returns an error for hash.Hash
	return fmt.Sprintf("%s:%016x", base, h.Sum64())
}

// SanitizeName bounds a name to MaxNameLen runes and drops control characters:
// names come from user YAML and reach Title and Description unescaped.
func SanitizeName(name string) string {
	return SanitizeTo(name, MaxNameLen)
}

// SanitizeTo is SanitizeName with a caller-chosen bound, for values such as hosts
// and paths that legitimately run longer than a name.
func SanitizeTo(name string, maxLen int) string {
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, name)

	switch {
	case name == "":
		return Unknown
	case utf8.RuneCountInString(name) <= maxLen:
		return name
	}
	return string([]rune(name)[:maxLen-utf8.RuneCountInString(ellipsis)]) + ellipsis
}

// HumanizeBytes renders a byte count, treating negatives as zero.
func HumanizeBytes(n int64) string {
	if n < 0 {
		n = 0
	}
	return humanize.Bytes(uint64(n))
}

// HumanizeCount renders a count with thousands separators.
func HumanizeCount(n int64) string {
	return humanize.Comma(n)
}

// Pluralize appends an s unless n is exactly one.
func Pluralize(n int64, word string) string {
	if n == 1 {
		return word
	}
	return word + "s"
}
