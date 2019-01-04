// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package gohai

import (
	"github.com/DataDog/gohai/cpu"
	"github.com/DataDog/gohai/filesystem"
	"github.com/DataDog/gohai/memory"
	"github.com/DataDog/gohai/network"
	"github.com/DataDog/gohai/platform"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// GetPayload builds a payload of every metadata collected with gohai except processes metadata.
func GetPayload() *Payload {
	return &Payload{
		Gohai: getGohaiInfo(),
	}
}

func getGohaiInfo() *gohai {
	res := new(gohai)

	cpuPayload, err := new(cpu.Cpu).Collect()
	if err == nil {
		res.CPU = cpuPayload
	} else {
		log.Errorf("Failed to retrieve cpu metadata: %s", err)
	}

	fileSystemPayload, err := new(filesystem.FileSystem).Collect()
	if err == nil {
		res.FileSystem = fileSystemPayload
	} else {
		log.Errorf("Failed to retrieve filesystem metadata: %s", err)
	}

	memoryPayload, err := new(memory.Memory).Collect()
	if err == nil {
		res.Memory = memoryPayload
	} else {
		log.Errorf("Failed to retrieve memory metadata: %s", err)
	}

	if !config.IsContainerized() {
		networkPayload, err := new(network.Network).Collect()
		if err == nil {
			res.Network = networkPayload
		} else {
			log.Errorf("Failed to retrieve network metadata: %s", err)
		}
	}

	platformPayload, err := new(platform.Platform).Collect()
	if err == nil {
		res.Platform = platformPayload
	} else {
		log.Errorf("Failed to retrieve platform metadata: %s", err)
	}

	return res
}
