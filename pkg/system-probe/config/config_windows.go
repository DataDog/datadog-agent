// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package config

import (
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

const (
	// ServiceName is the service name used for the system-probe
	ServiceName = "datadog-system-probe"
)

var (
	defaultConfigDir = "c:\\programdata\\datadog\\"
)

func init() {
	pd, err := winutil.GetProgramDataDir()
	if err == nil {
		defaultConfigDir = pd
	}
}

// ValidateSocketAddress validates that the sysprobe socket config option is of the correct format.
func ValidateSocketAddress(sockAddress string) error {
	if !strings.HasPrefix(sockAddress, `\\.\pipe\`) {
		return fmt.Errorf(`named pipe must be of the form '\\.\pipe\<pipename>'`)
	}
	return nil
}

// ProcessEventDataStreamSupported returns true if process event data stream is supported
func ProcessEventDataStreamSupported() bool {
	return true
}

func allowPrebuiltEbpfFallback(_ model.Config) {
}
