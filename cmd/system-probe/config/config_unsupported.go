// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !unix && !windows

package config

import (
	"errors"

	"github.com/DataDog/datadog-agent/pkg/config/model"
)

const (
	defaultConfigDir = ""
)

// ValidateSocketAddress is not supported on this platform
func ValidateSocketAddress(sockPath string) error {
	return errors.New("system-probe unsupported")
}

// ProcessEventDataStreamSupported returns true if process event data stream is supported
func ProcessEventDataStreamSupported() bool {
	return false
}

func allowPrebuiltEbpfFallback(_ model.Config) {
}
