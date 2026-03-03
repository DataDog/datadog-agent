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
	loader, err := coreLoader(NewConfig(), nil)
	if err != nil {
		return err
	}
	return loader.loadCOREAsset(filename, startFn)
}

// GetBTFLoaderInfo Returns where the ebpf BTF files were sourced from
func GetBTFLoaderInfo() (string, error) {
	loader, err := coreLoader(NewConfig(), nil)
	if err != nil {
		return "", err
	}

	metadataStr := loader.btfLoader.resultMetadata.String()
	infoStr := fmt.Sprintf("btfLoader result: %d\n%s", loader.btfLoader.result, metadataStr)
	return infoStr, nil
}

func (c *coreAssetLoader) loadCOREAsset(filename string, startFn func(bytecode.AssetReader, manager.Options) error) error {
	var result COREResult
	base := strings.TrimSuffix(filename, path.Ext(filename))
	defer func() {
		c.reportTelemetry(base, result)
	}()

	ret, result, err := c.btfLoader.Get()
	if err != nil {
		return fmt.Errorf("BTF load: %w", err)
	}
	if ret == nil {
		return errors.New("no BTF data")
	}

	buf, err := bytecode.GetReader(c.coreDir, filename)
	if err != nil {
		result = AssetReadError
		return fmt.Errorf("error reading %s: %s", filename, err)
	}
	defer buf.Close()

	opts := manager.Options{
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
			result = VerifierError
		} else {
			result = LoaderError
		}
	}
	return err
}

func (c *coreAssetLoader) reportTelemetry(assetName string, result COREResult) {
	StoreCORETelemetryForAsset(assetName, result)

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
	switch BTFResult(result) {
	case SuccessCustomBTF:
		tags = append(tags, "custom")
		c.telemetry.success.Inc(tags...)
	case SuccessEmbeddedBTF:
		tags = append(tags, "embedded")
		c.telemetry.success.Inc(tags...)
	case SuccessDefaultBTF:
		tags = append(tags, "default")
		c.telemetry.success.Inc(tags...)
	case SuccessRemoteConfigBTF:
		tags = append(tags, "remoteconfig")
		c.telemetry.success.Inc(tags...)
	case BtfNotFound:
		tags = append(tags, "btf_not_found")
		c.telemetry.error.Inc(tags...)
	default:
		switch result {
		case AssetReadError:
			tags = append(tags, "asset_read")
			c.telemetry.error.Inc(tags...)
		case VerifierError:
			tags = append(tags, "verifier")
			c.telemetry.error.Inc(tags...)
		case LoaderError:
			tags = append(tags, "loader")
			c.telemetry.error.Inc(tags...)
		default:
			return
		}
	}
}
