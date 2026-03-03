// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package devicededuper provides a deduplication mechanism for SNMP devices based on the device info
// It is used to deduplicate devices which have multiple IPs
package devicededuper

import (
	"math"
	"net"
	"sync"
	"sync/atomic"

	"github.com/DataDog/datadog-agent/pkg/snmp"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const uptimeDiffToleranceMs = 5000

// DeviceInfo contains information about a SNMP device that is used to deduplicate devices
type DeviceInfo struct {
	Name        string
	Description string
	BootTimeMs  int64
	SysObjectID string
}

// PendingDevice represents a device pending deduplication
type PendingDevice struct {
	Config     snmp.Config
	Info       DeviceInfo
	AuthIndex  int
	WriteCache bool
	IP         string
	Failures   int
}

func (d DeviceInfo) equal(other DeviceInfo) bool {
	diff := math.Abs(float64(d.BootTimeMs - other.BootTimeMs))

	return d.Name == other.Name &&
		d.Description == other.Description &&
		d.SysObjectID == other.SysObjectID &&
		diff <= float64(uptimeDiffToleranceMs)
}

// DeviceDeduper is an interface for deduplicating SNMP devices
type DeviceDeduper interface {
	MarkIPAsProcessed(ip string)
	AddPendingDevice(device PendingDevice)
	GetDedupedDevices() []PendingDevice
	ResetCounters()
}

type deviceDeduperImpl struct {
	sync.RWMutex
	deviceInfos    []DeviceInfo
	pendingDevices []PendingDevice
	ipsCounter     map[string]*atomic.Uint32
	config         snmp.ListenerConfig
}

// NewDeviceDeduper creates a new DeviceDeduper instance
func NewDeviceDeduper(config snmp.ListenerConfig) DeviceDeduper {
	deduper := &deviceDeduperImpl{
		deviceInfos:    make([]DeviceInfo, 0),
		pendingDevices: make([]PendingDevice, 0),
		ipsCounter:     make(map[string]*atomic.Uint32),
		config:         config,
	}

	deduper.initializeCounters()

	return deduper
}

func (d *deviceDeduperImpl) initializeCounters() {
	for _, config := range d.config.Configs {
		ipAddr, ipNet, err := net.ParseCIDR(config.Network)
		if err != nil {
			log.Errorf("Couldn't parse SNMP network: %s", err)
			continue
		}

		startingIP := ipAddr.Mask(ipNet.Mask)

		for currentIP := startingIP; ipNet.Contains(currentIP); IncrementIP(currentIP) {
			if ignored := config.IsIPIgnored(currentIP); ignored {
				continue
			}

			ipStr := currentIP.String()
			counter, exists := d.ipsCounter[ipStr]
			if !exists {
				counter = &atomic.Uint32{}
				d.ipsCounter[ipStr] = counter
			}
			counter.Add(uint32(len(config.Authentications)))
		}
	}
}

func (d *deviceDeduperImpl) checkPreviousIPs(deviceIP string) bool {
	previousIPsDiscovered := true

	for ip, counter := range d.ipsCounter {
		count := counter.Load()
		if count > 0 && minimumIP(ip, deviceIP) == ip {
			previousIPsDiscovered = false
			break
		}
	}

	return previousIPsDiscovered
}

func (d *deviceDeduperImpl) contains(device DeviceInfo) bool {
	for _, existingDevice := range d.deviceInfos {
		if existingDevice.equal(device) {
			return true
		}
	}
	return false
}

// AddPendingDevice adds a device to the pending queue while ensuring that the IP is the minimum IP for a device
func (d *deviceDeduperImpl) AddPendingDevice(device PendingDevice) {
	d.Lock()
	defer d.Unlock()

	if d.contains(device.Info) {
		log.Debugf("Device %s already discovered", device.IP)
		return
	}

	newPendingDevices := make([]PendingDevice, 0)

	for _, pendingDevice := range d.pendingDevices {
		if pendingDevice.Info.equal(device.Info) {
			if minimumIP(pendingDevice.IP, device.IP) != device.IP {
				// current device IP is greater than the pending one, so we don't add it
				return
			}
			// remove the pending device because it's IP is greater than the new one
		} else {
			newPendingDevices = append(newPendingDevices, pendingDevice)
		}
	}

	newPendingDevices = append(newPendingDevices, device)

	d.pendingDevices = newPendingDevices
}

// GetDedupedDevices returns the list of devices to activate
func (d *deviceDeduperImpl) GetDedupedDevices() []PendingDevice {
	d.Lock()
	defer d.Unlock()
	dedupedDevices := make([]PendingDevice, 0)
	newPendingDevices := make([]PendingDevice, 0)

	for _, pendingDevice := range d.pendingDevices {
		previousIPsScanned := d.checkPreviousIPs(pendingDevice.IP)

		if previousIPsScanned {
			dedupedDevices = append(dedupedDevices, pendingDevice)
			d.deviceInfos = append(d.deviceInfos, pendingDevice.Info)
		} else {
			newPendingDevices = append(newPendingDevices, pendingDevice)
		}
	}
	d.pendingDevices = newPendingDevices
	return dedupedDevices
}

// MarkIPAsProcessed removes an IP from the counter, it is used to make sure we get the minimum IP for a device
func (d *deviceDeduperImpl) MarkIPAsProcessed(ip string) {
	d.RLock()
	defer d.RUnlock()
	counter, exists := d.ipsCounter[ip]

	if exists {
		counter.Add(^uint32(0)) // Subtract 1 using bitwise NOT of 0
	}
}

// ResetCounters resets the IP counters and device infos for a new discovery interval
func (d *deviceDeduperImpl) ResetCounters() {
	d.Lock()
	defer d.Unlock()

	for _, counter := range d.ipsCounter {
		counter.Store(0)
	}

	d.initializeCounters()
	d.deviceInfos = make([]DeviceInfo, 0)
}

func minimumIP(ipStr1, ipStr2 string) string {
	ip1 := net.ParseIP(ipStr1)
	ip2 := net.ParseIP(ipStr2)

	if ip1 == nil || ip2 == nil {
		return ""
	}

	for i := range ip1 {
		if ip1[i] < ip2[i] {
			return ip1.String()
		} else if ip1[i] > ip2[i] {
			return ip2.String()
		}
	}
	return ip1.String()
}

// IncrementIP increments an IP address by 1
func IncrementIP(ip net.IP) {
	for i := len(ip) - 1; i >= 0; i-- {
		ip[i]++
		if ip[i] > 0 {
			break
		}
	}
}
