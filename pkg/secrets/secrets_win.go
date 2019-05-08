// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build windows

package secrets

import (
	"fmt"
)

// Init encrypted secrets are not available on windows
func Init(command string, arguments []string, timeout int, maxSize int) {
}

// Decrypt encrypted secrets are not available on windows
func Decrypt(data []byte, origin string) ([]byte, error) {
	return data, nil
}

// GetDebugInfo exposes debug informations about secrets to be included in a flare
func GetDebugInfo() (*SecretInfo, error) {
	return nil, fmt.Errorf("Secret feature is not yet available on windows")
}
