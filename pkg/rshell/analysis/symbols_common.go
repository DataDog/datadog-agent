// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package analysis

// permanentlyBanned lists packages that may never be imported,
// regardless of what symbols they export.
// Keys ending with "/" are treated as prefix bans (any import path starting
// with that prefix is banned); all other keys are exact-match bans.
var permanentlyBanned = map[string]string{
	"reflect": "reflection defeats static safety analysis",
	"os/exec": "spawns arbitrary host processes, bypassing all shell restrictions",
	// NOTE: "net" (the base package) is intentionally NOT banned here so that the
	// ip builtin can use read-only interface enumeration (net.Interfaces, net.Interface,
	// net.IPNet, etc.) via the symbol-level allowlist. Only the safe, non-networking
	// symbols are permitted; connection-oriented symbols (net.Dial, net.Listen, etc.)
	// are not in builtinAllowedSymbols and therefore cannot be used. All net/ sub-packages
	// (net/http, net/smtp, etc.) remain permanently banned below.
	"net/":   "network sub-packages enable data exfiltration and C2 communication",
	"plugin": "dynamically loads Go shared libraries, enabling arbitrary code execution",
}
