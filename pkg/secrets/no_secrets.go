// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !secrets

// Placeholder for the secrets package when compiled without it

package secrets

import (
	"fmt"
	"io"
)

// SecretBackendOutputMaxSize defines max size of the JSON output from a secrets reader backend
var SecretBackendOutputMaxSize = 1024 * 1024

// Init placeholder when compiled without the 'secrets' build tag
func Init(command string, arguments []string, timeout int, maxSize int, groupExecPerm bool, removeTrailingLineBreak bool) {
}

// Decrypt encrypted secrets are not available on windows
func Decrypt(data []byte, origin string) ([]byte, error) {
	return data, nil
}

// GetDebugInfo exposes debug informations about secrets to be included in a flare
func GetDebugInfo(w io.Writer) {
	fmt.Fprintf(w, "Secret feature is not available in this version of the agent")
}
