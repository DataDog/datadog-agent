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
	"github.com/DataDog/datadog-agent/pkg/gohai/utils"

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
func GetPayload(hostname string, useHostnameResolver, isContainerized bool) *Payload {
	return &Payload{
		Gohai: getGohaiInfo(hostname, useHostnameResolver, isContainerized, false),
	}
}

// GetPayloadWithProcesses builds a pyaload of all metdata including processes
func GetPayloadWithProcesses(hostname string, useHostnameResolver, isContainerized bool) *Payload {
	return &Payload{
		Gohai: getGohaiInfo(hostname, useHostnameResolver, isContainerized, true),
	}
}

// GetPayloadAsString marshals the gohai struct twice (to a string). This allows the gohai payload to be embedded as a
// string in a JSON. This is required to mimic the metadata format inherited from Agent v5.
func GetPayloadAsString(hostname string, useHostnameResolver, IsContainerized bool) (string, error) {
	marshalledPayload, err := json.Marshal(getGohaiInfo(hostname, useHostnameResolver, IsContainerized, false))
	if err != nil {
		return "", err
	}
	return string(marshalledPayload), nil
}

func getGohaiInfo(hostname string, useHostnameResolver, isContainerized, withProcesses bool) *gohai {
	res := new(gohai)

	cpuPayload, warns, err := cpu.CollectInfo().AsJSON()
	if err == nil {
		res.CPU = cpuPayload
	} else {
		for _, warn := range warns {
			log.Debug(warn)
		}
		log.Warnf("Failed to retrieve cpu metadata: %s", err)
	}

	var fileSystemPayload interface{}
	fileSystemInfo, err := filesystem.CollectInfo()
	warns = nil
	if err == nil {
		fileSystemPayload, warns, err = fileSystemInfo.AsJSON()
	}
	if err == nil {
		res.FileSystem = fileSystemPayload
	} else {
		for _, warn := range warns {
			log.Debug(warn)
		}
		log.Warnf("Failed to retrieve filesystem metadata: %s", err)
	}

	memoryPayload, warns, err := memory.CollectInfo().AsJSON()
	if err == nil {
		res.Memory = memoryPayload
	} else {
		for _, warn := range warns {
			log.Debug(warn)
		}
		log.Warnf("Failed to retrieve memory metadata: %s", err)
	}

	if !isContainerized || detectDocker0() {

		var networkPayload interface{}
		networkInfo, err := network.CollectInfo()
		warns = nil
		if err == nil {
			if useHostnameResolver {
				ipv4s, ipv6s, err := network.ResolveFromHostname(hostname)
				if err != nil {
					log.Errorf("failed to resolve hostname to IP addresses: %s", err) //nolint:errcheck
				} else {
					if len(ipv4s) > 0 {
						networkInfo.IPAddress = ipv4s[0]
					}
					if len(ipv6s) > 0 {
						networkInfo.IPAddressV6 = utils.NewValue(ipv6s[0])
					}
				}
			}
			networkPayload, warns, err = networkInfo.AsJSON()
		}
		if err == nil {
			res.Network = networkPayload
		} else {
			for _, warn := range warns {
				log.Debug(warn)
			}
			log.Warnf("Failed to retrieve network metadata: %s", err)
		}
	}

	platformPayload, warns, err := platform.CollectInfo().AsJSON()
	if err == nil {
		res.Platform = platformPayload
	} else {
		for _, warn := range warns {
			log.Debug(warn)
		}
		log.Warnf("Failed to retrieve platform metadata: %s", err)
	}

	if withProcesses {
		var processesPayload interface{}
		processesInfo, err := processes.CollectInfo()
		warns = nil
		if err == nil {
			processesPayload, warns, err = processesInfo.AsJSON()
		}
		if err == nil {
			res.Processes = processesPayload
		} else {
			for _, warn := range warns {
				log.Debug(warn)
			}
			log.Warnf("Failed to retrieve processes metadata: %s", err)
		}
	}

	return res
}
