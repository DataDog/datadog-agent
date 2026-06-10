// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux_bpf && !test

package ebpf

import (
	"errors"
	"fmt"

	manager "github.com/DataDog/ebpf-manager"

	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
)

// LoadCOREAsset attempts to find kernel BTF, reads the CO-RE object file, and then calls the callback function with the
// asset and BTF options pre-filled. You should attempt to load the CO-RE program in the startFn func for telemetry to
// be correctly recorded.
func LoadCOREAsset(filename string, startFn func(bytecode.AssetReader, manager.Options) error) error {
	loader := getCORELoader()
	if loader == nil {
		return errors.New("CO-RE loader not setup")
	}
	return loader.loadCOREAsset(filename, startFn)
}

// GetBTFLoaderInfo Returns where the ebpf BTF files were sourced from
func GetBTFLoaderInfo() (string, error) {
	loader := getCORELoader()
	if loader == nil {
		return "", errors.New("CO-RE loader not setup")
	}

	metadataStr := loader.btfLoader.resultMetadata.String()
	infoStr := fmt.Sprintf("btfLoader result: %d\n%s", loader.btfLoader.result, metadataStr)
	return infoStr, nil
}
