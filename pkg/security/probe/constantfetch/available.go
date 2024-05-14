// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//go:build linux && linux_bpf

// Package constantfetch holds constantfetch related files
package constantfetch

import (
	"errors"

	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/cilium/ebpf/btf"

	pkgebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	"github.com/DataDog/datadog-agent/pkg/security/probe/config"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
)

// GetAvailableConstantFetchers returns available constant fetchers
func GetAvailableConstantFetchers(config *config.Config, kv *kernel.Version, statsdClient statsd.ClientInterface) []ConstantFetcher {
	fetchers := make([]ConstantFetcher, 0)

	if coreFetcher, err := NewBTFConstantFetcherFromCurrentKernel(); err == nil {
		fetchers = append(fetchers, coreFetcher)
	}

	if config.RuntimeCompiledConstantsEnabled {
		rcConstantFetcher := NewRuntimeCompilationConstantFetcher(&config.Config, statsdClient)
		fetchers = append(fetchers, rcConstantFetcher)
	}

	btfhubFetcher, err := NewBTFHubConstantFetcher(kv)
	if err != nil {
		seclog.Debugf("failed to create btfhub constant fetcher: %v", err)
	} else {
		fetchers = append(fetchers, btfhubFetcher)
	}

	OffsetGuesserFetcher := NewOffsetGuesserFetcher(config, kv)
	fetchers = append(fetchers, OffsetGuesserFetcher)

	fallbackConstantFetcher := NewFallbackConstantFetcher(kv)
	fetchers = append(fetchers, fallbackConstantFetcher)

	return fetchers
}

// GetHasUsernamespaceFirstArgWithBtf uses BTF to check if the security_inode_setattr function has a user namespace as its first argument
func GetHasUsernamespaceFirstArgWithBtf() (bool, error) {
	spec, err := pkgebpf.GetKernelSpec()
	if err != nil {
		return false, err
	}

	var function *btf.Func
	if err := spec.TypeByName("security_inode_setattr", &function); err != nil {
		return false, err
	}

	proto, ok := function.Type.(*btf.FuncProto)
	if !ok {
		return false, errors.New("security_inode_setattr has no prototype")
	}

	if len(proto.Params) == 0 {
		return false, errors.New("security_inode_setattr has no parameters")
	}

	return proto.Params[0].Name != "dentry", nil
}
