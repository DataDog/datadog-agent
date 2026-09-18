// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package additionalfields

import (
	"bytes"
	"sync"

	"github.com/netsampler/goflow2/decoders/netflow"
	"github.com/netsampler/goflow2/producer"

	ddlog "github.com/DataDog/datadog-agent/pkg/util/log"
)

// IANA IPFIX Information Element numbers for in Options Data records (RFC 6759)
const (
	ipfixFieldApplicationID   uint16 = 95
	ipfixFieldApplicationName uint16 = 96
)

// ApplicationMapper collects and looks up applicationId/applicationName mappings
// reported by exporters via IPFIX Options Data records, scoped per exporter IP.
type ApplicationMapper struct {
	mu   sync.RWMutex
	apps map[string]map[uint32]string // exporterIP -> applicationId -> applicationName
}

// NewApplicationMapper returns an initialized ApplicationMapper
func NewApplicationMapper() *ApplicationMapper {
	return &ApplicationMapper{apps: make(map[string]map[uint32]string)}
}

// Lookup returns the application name for the given exporterIP/appID, if known
func (m *ApplicationMapper) Lookup(exporterIP string, appID uint32) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	apps, ok := m.apps[exporterIP]
	if !ok {
		return "", false
	}
	name, ok := apps[appID]
	return name, ok
}

// addToCache extracts applicationId/applicationName pairs from IPFIX Options Data records
// and stores them, scoped to exporterIP, for later lookup.
func (m *ApplicationMapper) addToCache(exporterIP string, optionsDataFlowSet []netflow.OptionsDataFlowSet) {
	ddlog.Debugf("DEBUGTMP addToCache exporterIP=%s optionsDataFlowSets=%d", exporterIP, len(optionsDataFlowSet))
	for _, dataFlowSet := range optionsDataFlowSet {
		for _, record := range dataFlowSet.Records {
			appID, haveID := extractApplicationID(record.ScopesValues)
			appName, haveName := extractApplicationName(record.OptionsValues)
			ddlog.Debugf("DEBUGTMP options record: appID=%d haveID=%v appName=%q haveName=%v scopes=%+v options=%+v", appID, haveID, appName, haveName, record.ScopesValues, record.OptionsValues)
			if haveID && haveName {
				m.set(exporterIP, appID, appName)
			}
		}
	}
}

// lookupApplicationName decodes a raw applicationId field value (IPFIX field 95) and resolves it
// against names previously cached from this exporter's Options Data records.
func (m *ApplicationMapper) lookupApplicationName(exporterIP string, rawAppID []byte) (string, bool) {
	var id uint64
	if err := producer.DecodeUNumber(rawAppID, &id); err != nil {
		ddlog.Debugf("DEBUGTMP lookupApplicationName decode error exporterIP=%s rawAppID=%x err=%v", exporterIP, rawAppID, err)
		return "", false
	}
	name, found := m.Lookup(exporterIP, uint32(id))
	ddlog.Debugf("DEBUGTMP lookupApplicationName exporterIP=%s appID=%d found=%v name=%q", exporterIP, uint32(id), found, name)
	return name, found
}

func extractApplicationID(fields []netflow.DataField) (appID uint32, haveID bool) {
	for _, f := range fields {
		if f.Type != ipfixFieldApplicationID {
			continue
		}
		v, ok := f.Value.([]byte)
		if !ok {
			continue
		}
		var id uint64
		if err := producer.DecodeUNumber(v, &id); err == nil {
			return uint32(id), true
		}
	}
	return 0, false
}

func extractApplicationName(fields []netflow.DataField) (appName string, haveName bool) {
	for _, f := range fields {
		if f.Type != ipfixFieldApplicationName {
			continue
		}
		v, ok := f.Value.([]byte)
		if !ok {
			continue
		}
		return string(bytes.Trim(v, "\x00")), true
	}
	return "", false
}

func (m *ApplicationMapper) set(exporterIP string, appID uint32, appName string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	apps, ok := m.apps[exporterIP]
	if !ok {
		apps = make(map[uint32]string)
		m.apps[exporterIP] = apps
	}
	apps[appID] = appName
}
