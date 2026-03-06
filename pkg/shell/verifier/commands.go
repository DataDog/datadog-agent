// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package verifier

// blockedBuiltins are shell builtins that are explicitly forbidden even though
// they might technically be "commands". These can be used to escape the sandbox.
var blockedBuiltins = map[string]bool{
	"eval":   true,
	"exec":   true,
	"source": true,
	".":      true,
	"trap":   true,
}

// IsBlockedBuiltin returns true if the command name is an explicitly blocked builtin.
func IsBlockedBuiltin(name string) bool {
	return blockedBuiltins[name]
}
