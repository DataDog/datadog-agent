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
	"maps"
	"math/rand"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	flare "github.com/DataDog/datadog-agent/comp/core/flare/types"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/status"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	snmpscan "github.com/DataDog/datadog-agent/comp/snmpscan/def"
	snmpscanmanager "github.com/DataDog/datadog-agent/comp/snmpscanmanager/def"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/metadata"
	"github.com/DataDog/datadog-agent/pkg/persistentcache"
	"github.com/DataDog/datadog-agent/pkg/snmp/gosnmplib"
)

const (
	scanWorkers   = 2 // Max concurrent scans allowed
	scanQueueSize = 10_000

	snmpCallsPerSecond = 8
	snmpCallInterval   = time.Second / snmpCallsPerSecond
	maxSnmpCallCount   = 100_000

	scanSchedulerCheckInterval = 10 * time.Minute
	scanRefreshInterval        = 182 * 24 * time.Hour   // 6 months
	scanRefreshJitter          = 2 * 7 * 24 * time.Hour // 2 weeks

	cacheKey = "snmp_scanned_devices"
)

// scanRetryDelays defines how long to wait before retrying a failed scan.
// Each duration represents the delay before the next retry attempt.
var scanRetryDelays = []time.Duration{
	1 * time.Hour,      // 1 hour
	12 * time.Hour,     // 12 hours
	24 * time.Hour,     // 1 day
	3 * 24 * time.Hour, // 3 days
	7 * 24 * time.Hour, // 1 week
}

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
	Comp          snmpscanmanager.Component
	Status        status.InformationProvider
	FlareProvider flare.Provider
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
		Comp:          scanManager,
		Status:        status.NewInformationProvider(scanManager),
		FlareProvider: flare.NewProvider(scanManager.fillFlare),
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

	if !m.agentConfig.GetBool("network_devices.default_scan.enabled") {
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
		m.onDeviceScanFailure(req, false)
		return err
	}

	err = m.scanner.ScanDeviceAndSendData(m.ctx, snmpConfig, namespace,
		snmpscan.ScanParams{
			ScanType:     metadata.DefaultScan,
			CallInterval: snmpCallInterval,
			MaxCallCount: maxSnmpCallCount,
		})
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return nil
		}

		m.onDeviceScanFailure(req, isRetryableError(err))
		return err
	}

	m.onDeviceScanSuccess(req)

	m.log.Infof("Successfully processed default scan request for device '%s'", req.DeviceIP)

	return nil
}

func (m *snmpScanManagerImpl) onDeviceScanSuccess(req snmpscanmanager.ScanRequest) {
	now := time.Now()

	m.mtx.Lock()
	m.deviceScans[req.DeviceIP] = deviceScan{
		DeviceIP:   req.DeviceIP,
		ScanStatus: successScan,
		ScanEndTs:  now,
		Failures:   0,
	}
	m.mtx.Unlock()

	m.writeCache()

	m.scheduleScanRefresh(req, now)
}

func (m *snmpScanManagerImpl) onDeviceScanFailure(req snmpscanmanager.ScanRequest, canRetry bool) {
	now := time.Now()

	m.mtx.Lock()
	var failuresCount int
	if !canRetry {
		failuresCount = -1 // -1 means that it will not be retried
	} else {
		oldScan, exists := m.deviceScans[req.DeviceIP]
		if !exists {
			failuresCount = 1
		} else {
			failuresCount = max(oldScan.Failures+1, 1)
		}
	}
	m.deviceScans[req.DeviceIP] = deviceScan{
		DeviceIP:   req.DeviceIP,
		ScanStatus: failedScan,
		ScanEndTs:  now,
		Failures:   failuresCount,
	}
	m.mtx.Unlock()

	m.writeCache()

	if canRetry {
		m.scheduleScanRetry(req, now, failuresCount)
	}
}

func isRetryableError(err error) bool {
	var connErr *gosnmplib.ConnectionError
	return errors.As(err, &connErr)
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

		if scan.isFailed() {
			m.scheduleScanRetry(snmpscanmanager.ScanRequest{
				DeviceIP: scan.DeviceIP,
			}, scan.ScanEndTs, scan.Failures)
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

func (m *snmpScanManagerImpl) scheduleScanRetry(req snmpscanmanager.ScanRequest, lastScanTs time.Time, failuresCount int) {
	idx := failuresCount - 1
	if idx < 0 || idx >= len(scanRetryDelays) {
		return
	}

	m.scanScheduler.QueueScanTask(scanTask{
		req:        req,
		nextScanTs: lastScanTs.Add(scanRetryDelays[idx]),
	})
}

func (m *snmpScanManagerImpl) cloneDeviceScans() deviceScansByIP {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	return maps.Clone(m.deviceScans)
}
