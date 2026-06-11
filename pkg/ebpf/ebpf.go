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

	"github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	rcclient "github.com/DataDog/datadog-agent/comp/remote-config/rcclient/def"
)

var core struct {
	sync.RWMutex
	loader *coreAssetLoader
}

// Setup initializes CO-RE and BTF loaders with the provided config.
// [Reset] must be called first if you want a different config to take effect
func Setup(cfg *Config, rcclient rcclient.Component, telemetry telemetry.Component) error {
	_, err := coreLoader(cfg, rcclient, telemetry)
	return err
}

func getCORELoader() *coreAssetLoader {
	core.RLock()
	defer core.RUnlock()
	return core.loader
}

func coreLoader(cfg *Config, rcclient rcclient.Component, telemetrycomp telemetry.Component) (*coreAssetLoader, error) {
	loader := getCORELoader()
	if loader != nil {
		return loader, nil
	}

	core.Lock()
	defer core.Unlock()
	if err := rlimit.RemoveMemlock(); err != nil {
		return nil, fmt.Errorf("rlimit memlock: %w", err)
	}

	btfLoader, err := initBTFLoader(cfg, rcclient, telemetrycomp)
	if err != nil {
		return nil, err
	}

	core.loader = &coreAssetLoader{
		coreDir:   filepath.Join(cfg.BPFDir, "co-re"),
		btfLoader: btfLoader,
		telemetry: struct {
			success telemetry.Counter
			error   telemetry.Counter
		}{
			success: telemetrycomp.NewCounter("ebpf", "core_load_success", []string{"platform", "platform_version", "kernel", "arch", "asset", "btf_type"}, "count of CO-RE load successes"),
			error:   telemetrycomp.NewCounter("ebpf", "core_load_error", []string{"platform", "platform_version", "kernel", "arch", "asset", "error_type"}, "count of CO-RE load errors"),
		},
	}
	return core.loader, nil
}

// Reset resets CO-RE and BTF loaders and manager modifiers back to uninitialized state
func Reset() {
	core.Lock()
	defer core.Unlock()

	core.loader.btfLoader.Flush()
	core.loader = nil
}
