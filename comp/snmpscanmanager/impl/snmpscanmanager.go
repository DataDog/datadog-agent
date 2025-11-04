// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package snmpscanmanagerimpl implements the snmpscanmanager component interface
package snmpscanmanagerimpl

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	snmpscan "github.com/DataDog/datadog-agent/comp/snmpscan/def"
	snmpscanmanager "github.com/DataDog/datadog-agent/comp/snmpscanmanager/def"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/metadata"
	"github.com/DataDog/datadog-agent/pkg/persistentcache"
	"github.com/DataDog/datadog-agent/pkg/snmp/snmpparse"
)

const (
	scanWorkers   = 2 // Max concurrent scans allowed
	scanQueueSize = 10000

	snmpCallsPerSecond = 8
	snmpCallInterval   = time.Second / snmpCallsPerSecond

	cacheKey = "snmp:scanned_devices"
)

// Requires defines the dependencies for the snmpscanmanager component
type Requires struct {
	compdef.In
	Lifecycle  compdef.Lifecycle
	Logger     log.Component
	Config     config.Component
	HttpClient ipc.HTTPClient
	Scanner    snmpscan.Component
}

// Provides defines the output of the snmpscanmanager component
type Provides struct {
	Comp snmpscanmanager.Component
}

// NewComponent creates a new snmpscanmanager component
func NewComponent(reqs Requires) (Provides, error) {
	scanManager := &snmpScanManagerImpl{
		log:         reqs.Logger,
		scanner:     reqs.Scanner,
		agentConfig: reqs.Config,
		httpClient:  reqs.HttpClient,
	}
	scanManager.loadCache()

	reqs.Lifecycle.Append(compdef.Hook{
		OnStart: func(_ context.Context) error {
			scanManager.start()
			return nil
		},
		OnStop: func(_ context.Context) error {
			scanManager.stop()
			return nil
		},
	})

	return Provides{
		Comp: scanManager,
	}, nil
}

type snmpScanManagerImpl struct {
	log         log.Component
	scanner     snmpscan.Component
	agentConfig config.Component
	httpClient  ipc.HTTPClient

	scanQueue   chan snmpscanmanager.ScanRequest
	deviceScans deviceScansByIP

	ctx        context.Context
	cancelFunc context.CancelFunc
	wg         sync.WaitGroup
	mtx        sync.Mutex
}

func (m *snmpScanManagerImpl) start() {
	m.scanQueue = make(chan snmpscanmanager.ScanRequest, scanQueueSize)
	m.deviceScans = make(deviceScansByIP)

	m.ctx, m.cancelFunc = context.WithCancel(context.Background())

	for i := 0; i < scanWorkers; i++ {
		m.wg.Add(1)
		go m.scanWorker()
	}
}

func (m *snmpScanManagerImpl) stop() {
	close(m.scanQueue)

	m.cancelFunc()
	m.wg.Wait()
}

// RequestScan queues a new scan request when the device has not been already scanned
func (m *snmpScanManagerImpl) RequestScan(req snmpscanmanager.ScanRequest) {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	if m.agentConfig.GetBool("network_devices.default_scan.disabled") {
		return
	}

	_, exists := m.deviceScans[req.DeviceIP]
	if exists {
		return
	}

	m.deviceScans[req.DeviceIP] = deviceScan{
		DeviceIP:   req.DeviceIP,
		ScanStatus: pendingStatus,
	}

	select {
	case m.scanQueue <- req:
		m.log.Infof("Queued scan request for device %s", req.DeviceIP)
	default:
		m.log.Warnf("Dropping scan request for device %s, scan queue is full", req.DeviceIP)
	}
}

func (m *snmpScanManagerImpl) scanWorker() {
	defer m.wg.Done()

	for req := range m.scanQueue {
		err := m.processScanRequest(req)
		if err != nil {
			m.log.Errorf("Error processing scan request: %v", err)
		}
	}
}

func (m *snmpScanManagerImpl) processScanRequest(req snmpscanmanager.ScanRequest) error {
	select {
	case <-m.ctx.Done():
		return nil
	default:
	}

	instanceConfig, err := snmpparse.GetParamsFromAgent(req.DeviceIP, m.agentConfig, m.httpClient)
	if err != nil {
		m.log.Errorf("Error getting instance config for device %s: %v", req.DeviceIP, err)
		m.setDeviceScan(deviceScan{
			DeviceIP:   req.DeviceIP,
			ScanStatus: failedStatus,
		})
		m.writeCache()
		return err
	}

	err = m.scanner.ScanDeviceAndSendData(m.ctx, instanceConfig, req.Namespace,
		snmpscan.ScanParams{
			ScanType:     metadata.DefaultScan,
			CallInterval: snmpCallInterval,
		})
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return nil
		}

		m.log.Errorf("Error processing scan request for device %s: %v", req.DeviceIP, err)
		m.setDeviceScan(deviceScan{
			DeviceIP:   req.DeviceIP,
			ScanStatus: failedStatus,
		})
		m.writeCache()
		return err
	}

	m.setDeviceScan(deviceScan{
		DeviceIP:   req.DeviceIP,
		ScanEndTs:  time.Now(),
		ScanStatus: successStatus,
	})
	m.writeCache()

	return nil
}

func (m *snmpScanManagerImpl) loadCache() {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	cacheValue, err := persistentcache.Read(cacheKey)
	if err != nil {
		m.log.Errorf("Error loading scanned devices cache: %v", err)
		return
	}
	if cacheValue == "" {
		return
	}

	var deviceScans []deviceScan
	err = json.Unmarshal([]byte(cacheValue), &deviceScans)
	if err != nil {
		m.log.Errorf("Error unmarshaling scanned devices cache to JSON: %v", err)
		return
	}

	for _, scan := range deviceScans {
		m.deviceScans[scan.DeviceIP] = scan
	}
}

func (m *snmpScanManagerImpl) writeCache() {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	var deviceScans []deviceScan
	for _, scan := range m.deviceScans {
		if scan.isCacheable() {
			deviceScans = append(deviceScans, scan)
		}
	}

	cacheValue, err := json.Marshal(deviceScans)
	if err != nil {
		m.log.Errorf("Error marshaling scanned devices cache to JSON: %v", err)
		return
	}

	err = persistentcache.Write(cacheKey, string(cacheValue))
	if err != nil {
		m.log.Errorf("Error writing scanned devices cache: %v", err)
	}
}

func (m *snmpScanManagerImpl) setDeviceScan(deviceScan deviceScan) {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	m.deviceScans[deviceScan.DeviceIP] = deviceScan
}
