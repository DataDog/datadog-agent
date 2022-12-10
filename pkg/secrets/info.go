// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package secrets

import (
	"fmt"
	"io"
	"runtime"
	"strings"
)

// SecretInfo export troubleshooting information about the decrypted secrets
type SecretInfo struct {
	ExecutablePath       string
	ExecutablePathSHA256 string
	Rights               string
	RightDetails         string
	UnixOwner            string
	UnixGroup            string
	SecretsHandles       map[string][]string
}

// Print output a SecretInfo to a io.Writer
func (si *SecretInfo) Print(w io.Writer) {
	fmt.Fprintf(w, "=== Checking executable rights ===\n")
	fmt.Fprintf(w, "Executable path: %s\n", si.ExecutablePath)

	sha256 := si.ExecutablePathSHA256
	if si.ExecutablePathSHA256 == "" {
		sha256 = "Not configured"
	}
	fmt.Fprintf(w, "Executable path SHA256: %s\n", sha256)

	fmt.Fprintf(w, "Check Rights: %s\n", si.Rights)

	fmt.Fprintf(w, "\nRights Detail:\n")
	fmt.Fprintf(w, "%s\n", si.RightDetails)

	if runtime.GOOS != "windows" {
		fmt.Fprintf(w, "Owner username: %s\n", si.UnixOwner)
		fmt.Fprintf(w, "Group name: %s\n", si.UnixGroup)
	}

	fmt.Fprintf(w, "\n=== Secrets stats ===\n")
	fmt.Fprintf(w, "Number of secrets decrypted: %d\n", len(si.SecretsHandles))
	fmt.Fprintf(w, "Secrets handle decrypted:\n")
	for handle, origins := range si.SecretsHandles {
		fmt.Fprintf(w, "- %s: from %s\n", handle, strings.Join(origins, ", "))
	}
}
