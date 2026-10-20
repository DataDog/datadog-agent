// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !linux

package privilegedhelper

import (
	"fmt"
	"os"
)

func readCredentialFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read verification credential: %w", err)
	}
	if len(data) > MaxMessageBytes {
		return nil, fmt.Errorf("verification credential exceeds %d bytes", MaxMessageBytes)
	}
	return data, nil
}
