// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019 Datadog, Inc.

// +build docker

package metadata

import (
	"github.com/segmentio/encoding/json"
	"net"
	"net/http"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	timeout = 500 * time.Millisecond
)

// GetTaskMetadataWithURL extracts the metadata payload for a task given a metadata URL.
func GetTaskMetadataWithURL(url string) (TaskMetadata, error) {
	var meta TaskMetadata
	client := http.Client{
		Timeout: timeout,
	}
	resp, err := client.Get(url)
	if err != nil {
		return meta, err
	}
	defer resp.Body.Close()

	decoder := json.NewDecoder(resp.Body)
	err = decoder.Decode(&meta)
	if err != nil {
		log.Errorf("Decoding task metadata failed - %s", err)
	}
	return meta, err
}

// GetTaskMetadataWithURL extracts the metadata payload for a container given a metadata URL.
func GetContainerMetadataWithURL(url string) (ContainerMetadata, error) {
	var meta ContainerMetadata
	client := http.Client{
		Timeout: timeout,
	}
	resp, err := client.Get(url)
	if err != nil {
		return meta, err
	}
	defer resp.Body.Close()

	decoder := json.NewDecoder(resp.Body)
	err = decoder.Decode(&meta)
	if err != nil {
		log.Errorf("Decoding container metadata failed - %s", err)
	}
	return meta, err
}

// GetContainerNetworkAddresses returns the network addresses for a given container ID and container metadata URL.
func GetContainerNetworkAddresses(metadataURL string) ([]containers.NetworkAddress, error) {
	meta, err := GetContainerMetadataWithURL(metadataURL)
	if err != nil {
		return nil, err
	}
	return ParseECSContainerNetworkAddresses(meta.Ports, meta.Networks, meta.DockerName), nil
}

// ParseECSContainerNetworkAddresses converts ECS container ports and networks into a list of NetworkAddress
func ParseECSContainerNetworkAddresses(ports []Port, networks []Network, container string) []containers.NetworkAddress {
	addrList := []containers.NetworkAddress{}
	if networks == nil {
		log.Debugf("No network settings available in ECS metadata")
		return addrList
	}
	for _, network := range networks {
		for _, addr := range network.IPv4Addresses { // one-element list
			IP := net.ParseIP(addr)
			if IP == nil {
				log.Warnf("Unable to parse IP: %v for container: %s", addr, container)
				continue
			}
			if len(ports) > 0 {
				// Ports is not nil, get ports and protocols
				for _, port := range ports {
					addrList = append(addrList, containers.NetworkAddress{
						IP:       IP,
						Port:     int(port.ContainerPort),
						Protocol: port.Protocol,
					})
				}
			} else {
				// Ports is nil (omitted by the ecs api if there are no ports exposed).
				// Keep the container IP anyway.
				addrList = append(addrList, containers.NetworkAddress{
					IP: IP,
				})
			}
		}
	}
	return addrList
}
