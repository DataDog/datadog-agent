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
	ebpftelemetry "github.com/DataDog/datadog-agent/pkg/ebpf/telemetry"
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

// GetBTFLoaderInfo Returns where the ebpf BTF files were sourced from
func GetBTFLoaderInfo() (string, error) {
	loader, err := coreLoader(NewConfig())
	if err != nil {
		return "", err
	}

	metadataStr := loader.btfLoader.resultMetadata.String()
	infoStr := fmt.Sprintf("btfLoader result: %d\n%s", loader.btfLoader.result, metadataStr)
	return infoStr, nil
}

func (c *coreAssetLoader) loadCOREAsset(filename string, startFn func(bytecode.AssetReader, manager.Options) error) error {
	var result ebpftelemetry.COREResult
	base := strings.TrimSuffix(filename, path.Ext(filename))
	defer func() {
		c.reportTelemetry(base, result)
	}()

	ret, result, err := c.btfLoader.Get()
	if err != nil {
		return fmt.Errorf("BTF load: %w", err)
	}
	if ret == nil {
		return fmt.Errorf("no BTF data")
	}

	buf, err := bytecode.GetReader(c.coreDir, filename)
	if err != nil {
		result = ebpftelemetry.AssetReadError
		return fmt.Errorf("error reading %s: %s", filename, err)
	}
	defer buf.Close()

	opts := manager.Options{
		KernelModuleBTFLoadFunc: ret.moduleLoadFunc,
		VerifierOptions: bpflib.CollectionOptions{
			Programs: bpflib.ProgramOptions{
				KernelTypes: ret.vmlinux,
			},
		},
	}

	err = startFn(buf, opts)
	if err != nil {
		var ve *bpflib.VerifierError
		if errors.As(err, &ve) {
			result = ebpftelemetry.VerifierError
		} else {
			result = ebpftelemetry.LoaderError
		}
	}
	return err
}

func (c *coreAssetLoader) reportTelemetry(assetName string, result ebpftelemetry.COREResult) {
	ebpftelemetry.StoreCORETelemetryForAsset(assetName, result)

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
	tags = append(tags, platform.String(), platformVersion, kernelVersion, arch, assetName)
	if ebpftelemetry.BTFResult(result) < ebpftelemetry.BtfNotFound {
		switch ebpftelemetry.BTFResult(result) {
		case ebpftelemetry.SuccessCustomBTF:
			tags = append(tags, "custom")
		case ebpftelemetry.SuccessEmbeddedBTF:
			tags = append(tags, "embedded")
		case ebpftelemetry.SuccessDefaultBTF:
			tags = append(tags, "default")
		default:
			return
		}
		c.telemetry.success.Inc(tags...)
		return
	}

	if ebpftelemetry.BTFResult(result) == ebpftelemetry.BtfNotFound {
		tags = append(tags, "btf_not_found")
	} else {
		switch result {
		case ebpftelemetry.AssetReadError:
			tags = append(tags, "asset_read")
		case ebpftelemetry.VerifierError:
			tags = append(tags, "verifier")
		case ebpftelemetry.LoaderError:
			tags = append(tags, "loader")
		default:
			return
		}
	}
	c.telemetry.error.Inc(tags...)
}
