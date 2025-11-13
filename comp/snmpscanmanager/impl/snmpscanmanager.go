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
	"math/rand"
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
)

const (
	scanWorkers   = 2 // Max concurrent scans allowed
	scanQueueSize = 10000

	snmpCallsPerSecond = 8
	snmpCallInterval   = time.Second / snmpCallsPerSecond

	scanSchedulerCheckInterval = 10 * time.Minute
	scanRefreshInterval        = 182 * 24 * time.Hour   // 6 months
	scanRefreshJitter          = 2 * 7 * 24 * time.Hour // 2 weeks

	cacheKey = "snmp_scanned_devices"
)

// Requires defines the dependencies for the snmpscanmanager component
type Requires struct {
	compdef.In
	Lifecycle  compdef.Lifecycle
	Logger     log.Component
	Config     config.Component
	HTTPClient ipc.HTTPClient
	Scanner    snmpscan.Component
}

// Provides defines the output of the snmpscanmanager component
type Provides struct {
	Comp snmpscanmanager.Component
}

// NewComponent creates a new snmpscanmanager component
func NewComponent(reqs Requires) (Provides, error) {
	ctx, cancelFunc := context.WithCancel(context.Background())
	scanManager := &snmpScanManagerImpl{
		log:         reqs.Logger,
		scanner:     reqs.Scanner,
		agentConfig: reqs.Config,
		httpClient:  reqs.HTTPClient,

		snmpConfigProvider: newSnmpConfigProvider(),
		scanScheduler:      newScanScheduler(),

		scanQueue:       make(chan snmpscanmanager.ScanRequest, scanQueueSize),
		allRequestedIPs: make(ipSet),
		deviceScans:     make(deviceScansByIP),

		ctx:        ctx,
		cancelFunc: cancelFunc,
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

	snmpConfigProvider snmpConfigProvider
	scanScheduler      scanScheduler

	scanQueue       chan snmpscanmanager.ScanRequest
	allRequestedIPs ipSet
	deviceScans     deviceScansByIP

	ctx        context.Context
	cancelFunc context.CancelFunc
	wg         sync.WaitGroup
	mtx        sync.Mutex
}

func (m *snmpScanManagerImpl) start() {
	m.wg.Add(1)
	go m.scanSchedulerWorker()

	for i := 0; i < scanWorkers; i++ {
		m.wg.Add(1)
		go m.scanWorker()
	}
}

func (m *snmpScanManagerImpl) stop() {
	m.cancelFunc()
	close(m.scanQueue)

	m.wg.Wait()
}

// RequestScan queues a new scan request when the device has not already been scanned.
// When forceQueue is true, the request is always added to the queue.
func (m *snmpScanManagerImpl) RequestScan(req snmpscanmanager.ScanRequest, forceQueue bool) {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	select {
	case <-m.ctx.Done():
		return
	default:
	}

	if m.agentConfig.GetBool("network_devices.default_scan.disabled") {
		return
	}

	if !forceQueue && m.allRequestedIPs.contains(req.DeviceIP) {
		return
	}

	select {
	case m.scanQueue <- req:
		m.allRequestedIPs.add(req.DeviceIP)
		m.log.Infof("Queued default scan request for device %s", req.DeviceIP)
	default:
		m.log.Warnf("Dropping default scan request for device %s, scan queue is full", req.DeviceIP)
	}
}

func (m *snmpScanManagerImpl) scanWorker() {
	defer m.wg.Done()

	for {
		select {
		case <-m.ctx.Done():
			return
		case req, ok := <-m.scanQueue:
			if !ok {
				return
			}

			err := m.processScanRequest(req)
			if err != nil {
				m.log.Errorf("Error processing default scan request for device '%s': %v", req.DeviceIP, err)
			}
		}
	}
}

func (m *snmpScanManagerImpl) processScanRequest(req snmpscanmanager.ScanRequest) error {
	snmpConfig, namespace, err := m.snmpConfigProvider.GetDeviceConfig(req.DeviceIP, m.agentConfig, m.httpClient)
	if err != nil {
		now := time.Now()
		m.setDeviceScan(deviceScan{
			DeviceIP:   req.DeviceIP,
			ScanStatus: failedStatus,
			ScanEndTs:  now,
		})
		m.writeCache()
		return err
	}

	err = m.scanner.ScanDeviceAndSendData(m.ctx, snmpConfig, namespace,
		snmpscan.ScanParams{
			ScanType:     metadata.DefaultScan,
			CallInterval: snmpCallInterval,
		})
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return nil
		}

		now := time.Now()
		m.setDeviceScan(deviceScan{
			DeviceIP:   req.DeviceIP,
			ScanStatus: failedStatus,
			ScanEndTs:  now,
		})
		m.writeCache()
		return err
	}

	now := time.Now()
	m.setDeviceScan(deviceScan{
		DeviceIP:   req.DeviceIP,
		ScanStatus: successStatus,
		ScanEndTs:  now,
	})
	m.writeCache()

	m.scheduleScanRefresh(req, now)

	m.log.Infof("Successfully processed default scan request for device '%s'", req.DeviceIP)

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
		m.allRequestedIPs.add(scan.DeviceIP)
		m.deviceScans[scan.DeviceIP] = scan

		if scan.isSuccess() {
			m.scheduleScanRefresh(snmpscanmanager.ScanRequest{
				DeviceIP: scan.DeviceIP,
			}, scan.ScanEndTs)
		}
	}
}

func (m *snmpScanManagerImpl) writeCache() {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	deviceScans := make([]deviceScan, 0)
	for _, scan := range m.deviceScans {
		deviceScans = append(deviceScans, scan)
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

func (m *snmpScanManagerImpl) scanSchedulerWorker() {
	defer m.wg.Done()

	timeTicker := time.NewTicker(scanSchedulerCheckInterval)
	defer timeTicker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-timeTicker.C:
			m.queueDueScans()
		}
	}
}

func (m *snmpScanManagerImpl) queueDueScans() {
	now := time.Now()
	scanReqs := m.scanScheduler.PopDueScans(now)
	for _, scanReq := range scanReqs {
		m.RequestScan(scanReq, true)
	}
}

func (m *snmpScanManagerImpl) scheduleScanRefresh(req snmpscanmanager.ScanRequest, lastScanTs time.Time) {
	refreshJitter := time.Duration(rand.Int63n(int64(2*scanRefreshJitter))) - scanRefreshJitter
	m.scanScheduler.QueueScanTask(scanTask{
		req:        req,
		nextScanTs: lastScanTs.Add(scanRefreshInterval).Add(refreshJitter),
	})
}

func (m *snmpScanManagerImpl) setDeviceScan(deviceScan deviceScan) {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	m.deviceScans[deviceScan.DeviceIP] = deviceScan
}
