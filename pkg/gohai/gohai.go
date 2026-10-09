// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package gohai encapsulate all the metadata collected by it's subpackage into a single payload ready to be ingested by the
// backend.
package gohai

import (
	"context"
	"encoding/json"
	"net"
	"sync"
	"time"

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

// fallbackHostLookupTimeout bounds the DNS resolution performed by
// resolveFallbackHostIPv4. A broken or unreachable resolver for the configured
// fallback host must only ever delay metadata collection briefly, never hang it
// indefinitely.
const fallbackHostLookupTimeout = 2 * time.Second

// resolveFallbackHostIPv4 validates or resolves a fallback host value (e.g. the
// kubernetes_kubelet_host config, which may be an IP literal or a hostname - see
// pkg/util/kubernetes/kubelet/kubelet_hosts.go) into a usable IPv4 address string.
// network.ipaddress is documented as an IPv4 address, so a raw, possibly-non-IP
// config value must never be written there directly. Returns "" if no usable IPv4
// address could be determined (including loopback, which is never a meaningful
// node address here), logging why at debug level.
func resolveFallbackHostIPv4(host string) string {
	if host == "" {
		return ""
	}

	// PreferGo forces Go's own resolver rather than the cgo resolver used by default
	// on most platforms, which does not honor context cancellation/timeouts at all.
	// If host is already an IP literal, LookupIPAddr returns it directly without
	// performing any actual DNS query.
	resolver := &net.Resolver{PreferGo: true}
	ctx, cancel := context.WithTimeout(context.Background(), fallbackHostLookupTimeout)
	defer cancel()

	addrs, err := resolver.LookupIPAddr(ctx, host)
	if err != nil {
		log.Debugf("gohai: failed to resolve fallback host %q for network metadata: %s", host, err)
		return ""
	}

	for _, addr := range addrs {
		if addr.IP.IsLoopback() {
			continue
		}
		if ip4 := addr.IP.To4(); ip4 != nil {
			return ip4.String()
		}
	}

	log.Debugf("gohai: fallback host %q did not resolve to a usable IPv4 address", host)
	return ""
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
// fallbackHost, when non-empty, is resolved and reported as the host's network IP if the
// agent is containerized and running without host networking (see getGohaiInfo). It may be
// an IP literal or a hostname (e.g. the kubernetes_kubelet_host config value).
func GetPayload(hostname string, useHostnameResolver, isContainerized bool, fallbackHost string) *Payload {
	return &Payload{
		Gohai: getGohaiInfo(hostname, useHostnameResolver, isContainerized, false, fallbackHost),
	}
}

// GetPayloadWithProcesses builds a pyaload of all metdata including processes. See GetPayload
// for the meaning of fallbackHost.
func GetPayloadWithProcesses(hostname string, useHostnameResolver, isContainerized bool, fallbackHost string) *Payload {
	return &Payload{
		Gohai: getGohaiInfo(hostname, useHostnameResolver, isContainerized, true, fallbackHost),
	}
}

// GetPayloadAsString marshals the gohai struct twice (to a string). This allows the gohai payload to be embedded as a
// string in a JSON. This is required to mimic the metadata format inherited from Agent v5. See GetPayload for the
// meaning of fallbackHost.
func GetPayloadAsString(hostname string, useHostnameResolver, isContainerized bool, fallbackHost string) (string, error) {
	marshalledPayload, err := json.Marshal(getGohaiInfo(hostname, useHostnameResolver, isContainerized, false, fallbackHost))
	if err != nil {
		return "", err
	}
	return string(marshalledPayload), nil
}

func getGohaiInfo(hostname string, useHostnameResolver, isContainerized, withProcesses bool, fallbackHost string) *gohai {
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
	} else if resolvedIP := resolveFallbackHostIPv4(fallbackHost); resolvedIP != "" {
		// Containerized without a detectable docker0 bridge means we can't assume host
		// networking, so net.Interfaces() would only see the container's own ephemeral
		// network namespace (e.g. a pod IP that changes on reschedule), not a usable
		// host-level target. Report the node's real IP instead, when the caller has one
		// available (e.g. from the Kubernetes Downward API via kubernetes_kubelet_host).
		fallbackInfo := &network.Info{
			IPAddress:   resolvedIP,
			IPAddressV6: utils.NewErrorValue[string](network.ErrAddressNotFound),
		}
		fallbackPayload, fallbackWarns, fallbackErr := fallbackInfo.AsJSON()
		if fallbackErr == nil {
			res.Network = fallbackPayload
		} else {
			for _, warn := range fallbackWarns {
				log.Debug(warn)
			}
			log.Warnf("Failed to build fallback network metadata: %s", fallbackErr)
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
