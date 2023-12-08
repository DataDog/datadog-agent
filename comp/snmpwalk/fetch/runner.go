// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package fetch

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/DataDog/datadog-agent/comp/snmpwalk/fetch/valuestore"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/epforwarder"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/metadata"
	"github.com/DataDog/datadog-agent/pkg/snmp/snmpparse"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/gosnmp/gosnmp"
	"os"
	"strings"
	"sync"
	"time"
)

// SnmpwalkRunner receives configuration from remote-config
type SnmpwalkRunner struct {
	upToDate bool
	sender   sender.Sender
	jobs     chan SnmpwalkJob
	stop     chan bool

	prevSnmpwalkTime   map[string]time.Time
	prevSnmpwalkTimeMu sync.Mutex

	isRunning   map[string]bool
	isRunningMu sync.Mutex
}

// SnmpwalkJob ...
type SnmpwalkJob struct {
	namespace string
	tags      []string
	config    snmpparse.SNMPConfig
}

// NewSnmpwalkRunner creates a new SnmpwalkRunner.
func NewSnmpwalkRunner(sender sender.Sender) *SnmpwalkRunner {
	snmpwalk := &SnmpwalkRunner{
		upToDate:         false,
		sender:           sender,
		prevSnmpwalkTime: make(map[string]time.Time),
		isRunning:        make(map[string]bool),
	}
	workers := pkgconfig.Datadog.GetInt("network_devices.snmpwalk.workers")
	if workers == 0 {
		workers = 10
	}
	log.Infof("[NewSnmpwalkRunner] Workers: %d", workers)

	jobs := make(chan SnmpwalkJob)
	for w := 0; w < workers; w++ {
		go worker(snmpwalk, jobs, w)
	}

	snmpwalk.jobs = jobs
	return snmpwalk
}

// Don't make it a method, to be overridden in tests
var worker = func(l *SnmpwalkRunner, jobs <-chan SnmpwalkJob, workerId int) {
	for {
		select {
		// TODO: IMPL STOP
		case <-l.stop:
			log.Debug("Stopping SNMP worker")
			return
		case job := <-jobs:
			log.Debugf("[worker %d] Handling IP %s", workerId, job.config.IPAddress)
			l.snmpwalkOneDevice(job.config, job.namespace, job.tags)
		}
	}
}

// Callback is when profiles updates are available (rc product NDM_DEVICE_PROFILES_CUSTOM)
func (rc *SnmpwalkRunner) Callback() {
	//globalStart := time.Now()
	log.Debug("[SNMP RUNNER] SNMP RUNNER")

	// TODO: Do not collect snmp-listener configs
	snmpConfigList, err := snmpparse.GetConfigCheckSnmp()
	if err != nil {
		log.Debugf("[SNMP RUNNER] Couldn't parse the SNMP config: %v", err)
		return
	}
	log.Debugf("[SNMP RUNNER] snmpConfigList len=%d", len(snmpConfigList))

	for _, config := range snmpConfigList {
		log.Debugf("[SNMP RUNNER] SNMP config: %+v", config)
	}

	hname, err := hostname.Get(context.TODO())
	if err != nil {
		log.Warnf("Error getting the hostname: %v", err)
	}
	commonTags := []string{
		"agent_host:" + hname,
	}
	namespace := "default"

	var interval time.Duration = pkgconfig.Datadog.GetDuration("network_devices.snmpwalk.interval")
	if interval == 0 {
		interval = 60 * time.Second
	}
	var throttlerMinIntervalPerOid time.Duration = pkgconfig.Datadog.GetDuration("network_devices.snmpwalk.throttler_min_interval_per_oid")

	rc.sender.Gauge("datadog.snmpwalk.interval", float64(interval.Seconds()), "", commonTags)
	rc.sender.Gauge("datadog.snmpwalk.workers", float64(pkgconfig.Datadog.GetDuration("network_devices.snmpwalk.workers")), "", commonTags)
	rc.sender.Gauge("datadog.snmpwalk.throttler_min_interval_per_oid", float64(throttlerMinIntervalPerOid.Seconds()), "", commonTags)

	for _, config := range snmpConfigList {
		if config.IPAddress == "" {
			continue
		}
		deviceID := namespace + ":" + config.IPAddress

		rc.prevSnmpwalkTimeMu.Lock()
		if prevTime, ok := rc.prevSnmpwalkTime[deviceID]; ok { // TODO: check config instanceId instead?
			// TODO: Also skip if the device is currently being walked
			if time.Since(prevTime) < interval {
				log.Debugf("[SNMP RUNNER] Skip Device, interval not reached: %s", deviceID)
				continue
			}
		}
		rc.prevSnmpwalkTimeMu.Unlock()

		// TODO: Also skip if the device is currently being walked
		rc.isRunningMu.Lock()
		if rc.isRunning[deviceID] {
			log.Debugf("[SNMP RUNNER] Skip Device, already running: %s", deviceID)
			continue
		}
		rc.isRunning[deviceID] = true
		rc.isRunningMu.Unlock()

		//rc.snmpwalkOneDevice(config, namespace, commonTags)
		rc.jobs <- SnmpwalkJob{
			namespace: namespace,
			tags:      commonTags,
			config:    config,
		}
	}
	//rc.sender.Gauge("datadog.snmpwalk.total.duration", time.Since(globalStart).Seconds(), "", commonTags)
}

func (rc *SnmpwalkRunner) snmpwalkOneDevice(config snmpparse.SNMPConfig, namespace string, commonTags []string) {
	ipaddr := config.IPAddress
	if ipaddr == "" {
		log.Debugf("[SNMP RUNNER] Missing IP Addr: %v", config)
		return
	}

	log.Debugf("[SNMP RUNNER] Run Device OID Scan for: %s", ipaddr)

	localStart := time.Now()
	deviceID := namespace + ":" + ipaddr
	fetchStrategy := useGetNext
	devTags := []string{
		"namespace:" + namespace, // TODO: FIX ME
		"device_ip:" + ipaddr,
		"device_id:" + deviceID,
		"snmp_command:" + string(fetchStrategy),
	}
	devTags = append(devTags, commonTags...)

	// TODO: Need mutex lock?
	rc.prevSnmpwalkTimeMu.Lock()
	if prevTime, ok := rc.prevSnmpwalkTime[deviceID]; ok { // TODO: check config instanceId instead?
		rc.sender.Gauge("datadog.snmpwalk.device.interval", localStart.Sub(prevTime).Seconds(), "", devTags)
	}
	rc.prevSnmpwalkTime[deviceID] = localStart
	rc.prevSnmpwalkTimeMu.Unlock()

	oidsCollectedCount := rc.collectDeviceOIDs(config, fetchStrategy)
	duration := time.Since(localStart)
	rc.sender.Gauge("datadog.snmpwalk.device.duration", duration.Seconds(), "", devTags)
	rc.sender.Gauge("datadog.snmpwalk.device.oids", float64(oidsCollectedCount), "", devTags)
	rc.sender.Counter("datadog.snmpwalk.device.oids_count", float64(oidsCollectedCount), "", devTags)

	rc.sender.Commit()

	rc.isRunningMu.Lock()
	delete(rc.isRunning, deviceID)
	rc.isRunningMu.Unlock()
}

func (rc *SnmpwalkRunner) collectDeviceOIDs(config snmpparse.SNMPConfig, fetchStrategy fetchStrategyType) int {
	prefix := fmt.Sprintf("(%s)", config.CommunityString)
	namespace := "default" // TODO: CHANGE PLACEHOLDER
	deviceID := namespace + ":" + config.IPAddress

	session := createSession(config)
	log.Debugf("[SNMP RUNNER]%s session: %+v", prefix, session)

	// Establish connection
	err := session.Connect()
	if err != nil {
		log.Errorf("[SNMP RUNNER]%s Connect err: %v\n", prefix, err)
		os.Exit(1)
		return 0
	}
	defer session.Conn.Close()

	variables := AllFirstRowOIDsVariables(session, fetchStrategy)
	log.Debugf("[SNMP RUNNER]%s Variables: %d", prefix, len(variables))

	for idx, variable := range variables {
		log.Debugf("[SNMP RUNNER]%s Variable Name (%d): %s", prefix, idx+1, variable.Name)
	}

	deviceOIDs := buildDeviceScanMetadata(deviceID, variables)

	metadataPayloads := metadata.BatchPayloads(namespace,
		"",
		time.Now(),
		metadata.PayloadMetadataBatchSize,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		deviceOIDs,
	)
	for _, payload := range metadataPayloads {
		payloadBytes, err := json.Marshal(payload)
		if err != nil {
			log.Errorf("[SNMP RUNNER]%s Error marshalling device metadata: %s", prefix, err)
			continue
		}
		log.Debugf("[SNMP RUNNER]%s Device OID metadata payload: %s", prefix, string(payloadBytes))
		rc.sender.EventPlatformEvent(payloadBytes, epforwarder.EventTypeNetworkDevicesMetadata)
		if err != nil {
			log.Errorf("[SNMP RUNNER]%s Error sending event platform event for Device OID metadata: %s", prefix, err)
		}
	}
	return len(deviceOIDs)
}

// Stop queues a shutdown of SNMPListener
func (rc *SnmpwalkRunner) Stop() {
	rc.stop <- true
}

func buildDeviceScanMetadata(deviceID string, oidsValues []gosnmp.SnmpPDU) []metadata.DeviceOid {
	var deviceOids []metadata.DeviceOid
	if oidsValues == nil {
		return deviceOids
	}
	for _, variablePdu := range oidsValues {
		_, value, err := valuestore.GetResultValueFromPDU(variablePdu)
		if err != nil {
			log.Debugf("GetValueFromPDU error: %s", err)
			continue
		}

		// TODO: How to store different types? Use Base64?
		strValue, err := value.ToString()
		if err != nil {
			log.Debugf("ToString error: %s", err)
			continue
		}

		deviceOids = append(deviceOids, metadata.DeviceOid{
			DeviceID:    deviceID,
			Oid:         strings.TrimLeft(variablePdu.Name, "."),
			Type:        variablePdu.Type.String(), // TODO: Map to internal types?
			ValueString: strValue,
		})
	}
	return deviceOids
}
