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
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

type coreAssetLoader struct {
	coreDir   string
	btfLoader *orderedBTFLoader
	telemetry struct {
		success telemetry.Counter
		error   telemetry.Counter
	}
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
	var result COREResult
	base := strings.TrimSuffix(filename, path.Ext(filename))
	defer func() {
		c.reportTelemetry(base, result)
	}()

	btfData, result, err := c.btfLoader.Get()
	if err != nil {
		return fmt.Errorf("BTF load: %w", err)
	}
	if btfData == nil {
		return fmt.Errorf("no BTF data")
	}

	buf, err := bytecode.GetReader(c.coreDir, filename)
	if err != nil {
		result = assetReadError
		return fmt.Errorf("error reading %s: %s", filename, err)
	}
	defer buf.Close()

	opts := manager.Options{
		VerifierOptions: bpflib.CollectionOptions{
			Programs: bpflib.ProgramOptions{
				KernelTypes: btfData,
				LogSize:     10 * 1024 * 1024,
			},
		},
	}

	err = startFn(buf, opts)
	if err != nil {
		var ve *bpflib.VerifierError
		if errors.As(err, &ve) {
			result = verifierError
		} else {
			result = loaderError
		}
	}
	return err
}

func (c *coreAssetLoader) reportTelemetry(assetName string, result COREResult) {
	storeCORETelemetryForAsset(assetName, result)

	var err error
	platform, err := getBTFPlatform()
	if err != nil {
		return
	}
	platformVersion, err := kernel.PlatformVersion()
	if err != nil {
		return
	}
	kernelVersion, err := kernel.Release()
	if err != nil {
		return
	}
	arch, err := kernel.Machine()
	if err != nil {
		return
	}

	// capacity should match number of tags
	tags := make([]string, 0, 6)
	tags = append(tags, platform, platformVersion, kernelVersion, arch, assetName)
	if BTFResult(result) < btfNotFound {
		switch BTFResult(result) {
		case successCustomBTF:
			tags = append(tags, "custom")
		case successEmbeddedBTF:
			tags = append(tags, "embedded")
		case successDefaultBTF:
			tags = append(tags, "default")
		default:
			return
		}
		c.telemetry.success.Inc(tags...)
		return
	}

	if BTFResult(result) == btfNotFound {
		tags = append(tags, "btf_not_found")
	} else {
		switch result {
		case assetReadError:
			tags = append(tags, "asset_read")
		case verifierError:
			tags = append(tags, "verifier")
		case loaderError:
			tags = append(tags, "loader")
		default:
			return
		}
	}
	c.telemetry.error.Inc(tags...)
}
