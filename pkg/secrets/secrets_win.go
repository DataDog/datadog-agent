// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build windows

package secrets

import (
	"fmt"
	"io"
)

// Init encrypted secrets are not available on windows
func Init(command string, arguments []string, timeout int, maxSize int) {
}

// Decrypt encrypted secrets are not available on windows
func Decrypt(data []byte) ([]byte, error) {
	return data, nil
}

// GetDebugInfo exposes debug informations about secrets to be included in a flare
func GetDebugInfo(w io.Writer) {
	fmt.Fprintln(w, "Secret feature is not yet available on windows")
}
