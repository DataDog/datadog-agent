// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package ebpf

import (
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"strings"

	manager "github.com/DataDog/ebpf-manager"
	bpflib "github.com/cilium/ebpf"
	"github.com/cilium/ebpf/btf"

	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
)

// LoadCOREAsset attempts to find kernel BTF, reads the CO-RE object file, and then calls the callback function with the
// asset and BTF options pre-filled. You should attempt to load the CO-RE program in the startFn func for telemetry to
// be correctly recorded.
func LoadCOREAsset(cfg *Config, filename string, startFn func(bytecode.AssetReader, manager.Options) error) error {
	var telemetry COREResult
	base := strings.TrimSuffix(filename, path.Ext(filename))
	defer func() {
		StoreCORETelemetryForAsset(base, telemetry)
	}()

	var btfData *btf.Spec
	btfData, telemetry = GetBTF(cfg.BTFPath, cfg.BPFDir)
	if btfData == nil {
		return fmt.Errorf("could not find BTF data on host")
	}

	buf, err := bytecode.GetReader(filepath.Join(cfg.BPFDir, "co-re"), filename)
	if err != nil {
		telemetry = AssetReadError
		return fmt.Errorf("error reading %s: %s", filename, err)
	}
	defer buf.Close()

	opts := manager.Options{
		VerifierOptions: bpflib.CollectionOptions{
			Programs: bpflib.ProgramOptions{
				KernelTypes: btfData,
			},
		},
	}

	err = startFn(buf, opts)
	if err != nil {
		var ve *bpflib.VerifierError
		if errors.As(err, &ve) {
			telemetry = VerifierError
		} else {
			telemetry = LoaderError
		}
	}
	return err
}
