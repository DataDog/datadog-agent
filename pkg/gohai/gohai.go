// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package gohai encapsulate all the metadata collected by it's subpackage into a single payload ready to be ingested by the
// backend.
package gohai

import (
	"encoding/json"
	"net"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/gohai/cpu"
	"github.com/DataDog/datadog-agent/pkg/gohai/filesystem"
	"github.com/DataDog/datadog-agent/pkg/gohai/memory"
	"github.com/DataDog/datadog-agent/pkg/gohai/network"
	"github.com/DataDog/datadog-agent/pkg/gohai/platform"
	"github.com/DataDog/datadog-agent/pkg/gohai/processes"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	// we can use this a hint that docker is running in host mode and it's safe to use detect
	docker0Detected = false
	docker0Detector sync.Once
)

func detectDocker0() bool {
	docker0Detector.Do(func() {
		iface, _ := net.InterfaceByName("docker0")
		docker0Detected = iface != nil
	})

	return docker0Detected
}

type gohai struct {
	CPU        interface{} `json:"cpu"`
	FileSystem interface{} `json:"filesystem"`
	Memory     interface{} `json:"memory"`
	Network    interface{} `json:"network"`
	Platform   interface{} `json:"platform"`
	Processes  interface{} `json:"processes,omitempty"`
}

// Payload handles the JSON unmarshalling of the metadata payload
type Payload struct {
	Gohai *gohai `json:"gohai"`
}

// GetPayload builds a payload of every metadata collected with gohai except processes metadata.
func GetPayload(isContainerized bool) *Payload {
	return &Payload{
		Gohai: getGohaiInfo(isContainerized, false),
	}
}

// GetPayloadWithProcesses builds a pyaload of all metdata including processes
func GetPayloadWithProcesses(isContainerized bool) *Payload {
	return &Payload{
		Gohai: getGohaiInfo(isContainerized, true),
	}
}

// GetPayloadAsString marshals the gohai struct twice (to a string). This allows the gohai payload to be embedded as a
// string in a JSON. This is required to mimic the metadata format inherited from Agent v5.
func GetPayloadAsString(IsContainerized bool) (string, error) {
	marshalledPayload, err := json.Marshal(getGohaiInfo(IsContainerized, false))
	if err != nil {
		return "", err
	}
	return string(marshalledPayload), nil
}

func getGohaiInfo(isContainerized, withProcesses bool) *gohai {
	res := new(gohai)

	cpuPayload, _, err := cpu.CollectInfo().AsJSON()
	if err == nil {
		res.CPU = cpuPayload
	} else {
		log.Errorf("Failed to retrieve cpu metadata: %s", err)
	}

	var fileSystemPayload interface{}
	fileSystemInfo, err := filesystem.CollectInfo()
	if err == nil {
		fileSystemPayload, _, err = fileSystemInfo.AsJSON()
	}
	if err == nil {
		res.FileSystem = fileSystemPayload
	} else {
		log.Errorf("Failed to retrieve filesystem metadata: %s", err)
	}

	memoryPayload, _, err := memory.CollectInfo().AsJSON()
	if err == nil {
		res.Memory = memoryPayload
	} else {
		log.Errorf("Failed to retrieve memory metadata: %s", err)
	}

	if !isContainerized || detectDocker0() {
		var networkPayload interface{}
		networkInfo, err := network.CollectInfo()
		if err == nil {
			networkPayload, _, err = networkInfo.AsJSON()
		}
		if err == nil {
			res.Network = networkPayload
		} else {
			log.Errorf("Failed to retrieve network metadata: %s", err)
		}
	}

	platformPayload, _, err := platform.CollectInfo().AsJSON()
	if err == nil {
		res.Platform = platformPayload
	} else {
		log.Errorf("Failed to retrieve platform metadata: %s", err)
	}

	if withProcesses {
		var processesPayload interface{}
		processesInfo, err := processes.CollectInfo()
		if err == nil {
			processesPayload, _, err = processesInfo.AsJSON()
		}
		if err == nil {
			res.Processes = processesPayload
		} else {
			log.Errorf("Failed to retrieve processes metadata: %s", err)
		}
	}

	return res
}
