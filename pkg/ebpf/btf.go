// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package ebpf

import (
	"os"
	"path/filepath"

	"github.com/cilium/ebpf/btf"
	"github.com/mholt/archiver/v3"

	"github.com/DataDog/datadog-agent/pkg/metadata/host"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func GetBTF(userProvidedBtfPath, collectionPath string) (*btf.Spec, CoReResult) {
	var btfSpec *btf.Spec
	var err error

	if userProvidedBtfPath != "" {
		btfSpec, err = loadBTFFrom(userProvidedBtfPath)
		if err == nil {
			log.Debugf("loaded BTF from %s", userProvidedBtfPath)
			return btfSpec, successCustomBTF
		}
	}

	btfSpec, err = checkEmbeddedCollection(collectionPath)
	if err == nil {
		log.Debugf("loaded BTF from embedded collection")
		return btfSpec, successEmbeddedBTF
	}
	log.Debugf("couldn't find BTF in embedded collection: %s", err)

	btfSpec, err = btf.LoadKernelSpec()
	if err == nil {
		log.Debugf("loaded BTF from default kernel location")
		return btfSpec, successDefaultBTF
	}
	log.Debugf("couldn't find BTF in default kernel locations: %s", err)

	return nil, btfNotFound
}

func checkEmbeddedCollection(collectionPath string) (*btf.Spec, error) {
	si := host.GetStatusInformation()
	platform := si.Platform
	platformVersion := si.PlatformVersion
	kernelVersion := si.KernelVersion

	btfFolder := filepath.Join(collectionPath, platform)
	if platform == "ubuntu" {
		btfFolder = filepath.Join(btfFolder, platformVersion)
	}
	btfPath := filepath.Join(btfFolder, kernelVersion+".btf")
	compressedBtfPath := btfPath + ".tar.xz"

	log.Debugf("checking embedded collection for btf at %s", compressedBtfPath)

	// All embedded BTFs must first be decompressed
	if err := archiver.NewTarXz().Unarchive(compressedBtfPath, btfFolder); err != nil {
		return nil, err
	}

	return loadBTFFrom(btfPath)
}

func loadBTFFrom(path string) (*btf.Spec, error) {
	data, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	return btf.LoadSpecFromReader(data)
}
