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
	"strings"

	manager "github.com/DataDog/ebpf-manager"
	bpflib "github.com/cilium/ebpf"

	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
)

type coreAssetLoader struct {
	coreDir   string
	btfLoader orderedBTFLoader
}

// LoadCOREAsset attempts to find kernel BTF, reads the CO-RE object file, and then calls the callback function with the
// asset and BTF options pre-filled. You should attempt to load the CO-RE program in the startFn func for telemetry to
// be correctly recorded.
func LoadCOREAsset(filename string, startFn func(bytecode.AssetReader, manager.Options) error) error {
	loader, err := coreLoader(NewConfig())
	if err != nil {
		return err
	}
	return loader.loadCOREAsset(filename, startFn)
}

func (c *coreAssetLoader) loadCOREAsset(filename string, startFn func(bytecode.AssetReader, manager.Options) error) error {
	var telemetry COREResult
	base := strings.TrimSuffix(filename, path.Ext(filename))
	defer func() {
		storeCORETelemetryForAsset(base, telemetry)
	}()

	btfData, telemetry, err := c.btfLoader.Get()
	if err != nil {
		return fmt.Errorf("BTF load: %w", err)
	}
	if btfData == nil {
		return fmt.Errorf("no BTF data")
	}

	buf, err := bytecode.GetReader(c.coreDir, filename)
	if err != nil {
		telemetry = assetReadError
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
			telemetry = verifierError
		} else {
			telemetry = loaderError
		}
	}
	return err
}
