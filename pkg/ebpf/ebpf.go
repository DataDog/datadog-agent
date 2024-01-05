// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package ebpf

import (
	"fmt"
	"path/filepath"
	"sync"

	"github.com/cilium/ebpf/rlimit"

	"github.com/DataDog/datadog-agent/pkg/telemetry"
)

var core struct {
	sync.RWMutex
	loader *coreAssetLoader
}

// Setup initializes CO-RE and BTF loaders with the provided config
// [Reset] must be called first if you want a different config to take effect
func Setup(cfg *Config) error {
	_, err := coreLoader(cfg)
	return err
}

func coreLoader(cfg *Config) (*coreAssetLoader, error) {
	core.RLock()
	loader := core.loader
	core.RUnlock()
	if loader != nil {
		return loader, nil
	}

	core.Lock()
	defer core.Unlock()
	if err := rlimit.RemoveMemlock(); err != nil {
		return nil, fmt.Errorf("rlimit memlock: %w", err)
	}
	core.loader = &coreAssetLoader{
		coreDir:   filepath.Join(cfg.BPFDir, "co-re"),
		btfLoader: initBTFLoader(cfg),
		telemetry: struct {
			success telemetry.Counter
			error   telemetry.Counter
		}{
			success: telemetry.NewCounter("ebpf__core__load", "success", []string{"platform", "platform_version", "kernel", "arch", "asset", "btf_type"}, "gauge of CO-RE load successes"),
			error:   telemetry.NewCounter("ebpf__core__load", "error", []string{"platform", "platform_version", "kernel", "arch", "asset", "error_type"}, "gauge of CO-RE load errors"),
		},
	}
	return core.loader, nil
}

// Reset resets CO-RE and BTF loaders back to uninitialized state
func Reset() {
	core.Lock()
	defer core.Unlock()

	core.loader.btfLoader.Flush()
	core.loader = nil
}
