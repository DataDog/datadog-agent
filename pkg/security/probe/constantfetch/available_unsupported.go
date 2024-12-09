// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//go:build linux && !linux_bpf

// Package constantfetch holds constantfetch related files
package constantfetch

import (
	"errors"

	"github.com/DataDog/datadog-go/v5/statsd"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	"github.com/DataDog/datadog-agent/pkg/security/probe/config"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
)

// GetAvailableConstantFetchers returns available constant fetchers
func GetAvailableConstantFetchers(_ *config.Config, kv *kernel.Version, _ statsd.ClientInterface) []ConstantFetcher {
	fetchers := make([]ConstantFetcher, 0)

	btfhubFetcher, err := NewBTFHubConstantFetcher(kv)
	if err != nil {
		seclog.Debugf("failed to create btfhub constant fetcher: %v", err)
	} else {
		fetchers = append(fetchers, btfhubFetcher)
	}

	fallbackConstantFetcher := NewFallbackConstantFetcher(kv)
	fetchers = append(fetchers, fallbackConstantFetcher)

	return fetchers
}

// GetHasUsernamespaceFirstArgWithBtf not available
func GetHasUsernamespaceFirstArgWithBtf() (bool, error) {
	return false, errors.New("unsupported BTF request")
}

// GetHasVFSRenameStructArgs not available
func GetHasVFSRenameStructArgs() (bool, error) {
	return false, errors.New("unsupported BTF request")
}
