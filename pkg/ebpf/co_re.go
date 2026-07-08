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

	"github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	ddbtf "github.com/DataDog/datadog-agent/pkg/ebpf/btf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
)

type coreAssetLoader struct {
	coreDir   string
	btfLoader *orderedBTFLoader
	telemetry struct {
		success telemetry.Counter
		error   telemetry.Counter
	}
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
			Cache: ddbtf.Cache(),
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

	// capacity should match number of tags
	tags := make([]string, len(c.btfLoader.fixedtags), len(c.btfLoader.fixedtags)+2)
	copy(tags, c.btfLoader.fixedtags)
	tags = append(tags, assetName)
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
