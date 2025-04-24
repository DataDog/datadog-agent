// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package listeners

import (
	"math"
	"net"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/snmp"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const uptimeDiffTolerance = 50

type deviceInfo struct {
	Name        string
	Description string
	BootTimeMs  int64
	IP          string
	SysObjectID string
}

type pendingService struct {
	svc        *SNMPService
	device     deviceInfo
	authIndex  int
	writeCache bool
}

func (d deviceInfo) equal(other deviceInfo) bool {
	diff := math.Abs(float64(d.BootTimeMs - other.BootTimeMs))

	return d.Name == other.Name &&
		d.Description == other.Description &&
		d.SysObjectID == other.SysObjectID &&
		diff <= float64(uptimeDiffTolerance)
}

type deviceDeduper interface {
	removeIP(ip string) bool
	checkPreviousIPs(deviceIP string) bool
	addDevice(device deviceInfo)
	contains(device deviceInfo) bool
	addPendingService(pendingSvc pendingService)
	flushPendingServices() []pendingService
}

type deviceDeduperImpl struct {
	sync.RWMutex
	devices         []deviceInfo
	pendingServices []pendingService
	ipsCounter      sync.Map
}

func newDeviceDeduper(config snmp.ListenerConfig) deviceDeduper {
	deduper := &deviceDeduperImpl{
		devices:         make([]deviceInfo, 0),
		pendingServices: make([]pendingService, 0),
	}
	deduper.initIPCounter(config)
	return deduper
}

func (d *deviceDeduperImpl) initIPCounter(config snmp.ListenerConfig) {
	for _, config := range config.Configs {
		ipAddr, ipNet, err := net.ParseCIDR(config.Network)
		startingIP := ipAddr.Mask(ipNet.Mask)
		if err != nil {
			log.Error(err)
			continue
		}

		for currentIP := startingIP; ipNet.Contains(currentIP); incrementIP(currentIP) {
			if ignored := config.IsIPIgnored(currentIP); ignored {
				continue
			}
			count, _ := d.ipsCounter.LoadOrStore(currentIP.String(), 0)
			d.ipsCounter.Store(currentIP.String(), count.(int)+len(config.Authentications))
		}
	}
}

func (d *deviceDeduperImpl) checkPreviousIPs(deviceIP string) bool {
	previousIPsDiscovered := true
	d.ipsCounter.Range(func(key, value any) bool {
		ip := key.(string)
		count := value.(int)
		if count > 0 && minimumIP(ip, deviceIP) == ip {
			previousIPsDiscovered = false
			return false
		}
		return true
	})

	return previousIPsDiscovered
}

func (d *deviceDeduperImpl) addDevice(device deviceInfo) {
	d.Lock()
	defer d.Unlock()
	d.devices = append(d.devices, device)
}

func (d *deviceDeduperImpl) contains(device deviceInfo) bool {
	d.Lock()
	defer d.Unlock()
	for _, existingDevice := range d.devices {
		if existingDevice.equal(device) {
			return true
		}
	}
	return false
}

func (d *deviceDeduperImpl) addPendingService(svc pendingService) {
	d.Lock()
	defer d.Unlock()

	newPendingServices := make([]pendingService, 0)

	for _, pendingSvc := range d.pendingServices {
		if pendingSvc.device.equal(svc.device) {
			if minimumIP(pendingSvc.device.IP, svc.device.IP) != svc.device.IP {
				// current svc IP is greater than the pending one, so we don't add it
				return
			}
			// remove the pending service because it's IP is greater than the new one
		} else {
			newPendingServices = append(newPendingServices, pendingSvc)
		}
	}

	newPendingServices = append(newPendingServices, svc)

	d.pendingServices = newPendingServices
}

func (d *deviceDeduperImpl) flushPendingServices() []pendingService {
	d.Lock()
	defer d.Unlock()
	svcToActivate := make([]pendingService, 0)
	newPendingServices := make([]pendingService, 0)

	for _, pendingSvc := range d.pendingServices {
		log.Debugf("Checking pending service for device %s", pendingSvc.svc.deviceIP)
		previousIPsScanned := d.checkPreviousIPs(pendingSvc.svc.deviceIP)

		if previousIPsScanned {
			log.Debugf("All previous IPs scanned for device %s, activating service", pendingSvc.svc.deviceIP)
			svcToActivate = append(svcToActivate, pendingSvc)
		} else {
			newPendingServices = append(newPendingServices, pendingSvc)
		}
	}
	d.pendingServices = newPendingServices
	return svcToActivate
}

func (d *deviceDeduperImpl) removeIP(ip string) bool {
	d.Lock()
	defer d.Unlock()
	count, _ := d.ipsCounter.Load(ip)
	newCount := count.(int) - 1
	d.ipsCounter.Store(ip, newCount)
	return newCount == 0
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
