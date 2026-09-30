// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package dpi

import (
	"bytes"
	"sync"

	"github.com/netsampler/goflow2/decoders/netflow"

	"github.com/DataDog/datadog-agent/comp/netflow/common"
)

type ApplicationMapper struct {
	mu   sync.RWMutex
	apps map[string]map[uint64]common.DPIApplication // exporterIP -> applicationId -> application
}

func NewApplicationMapper() *ApplicationMapper {
	return &ApplicationMapper{apps: make(map[string]map[uint64]common.DPIApplication)}
}

func (m *ApplicationMapper) lookupApplication(exporterIP string, rawAppID []byte) (common.DPIApplication, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	apps, ok := m.apps[exporterIP]
	if !ok {
		return common.DPIApplication{}, false
	}

	id, ok := applicationIDToUint64(rawAppID)
	if !ok {
		return common.DPIApplication{}, false
	}

	app, found := apps[id]
	return app, found
}

func (m *ApplicationMapper) addToCache(exporterIP string, optionsDataFlowSet []netflow.OptionsDataFlowSet) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, dataFlowSet := range optionsDataFlowSet {
		for _, record := range dataFlowSet.Records {
			m.addRecord(exporterIP, record)
		}
	}
}

func (m *ApplicationMapper) addRecord(exporterIP string, record netflow.OptionsDataRecord) {
	rawID, ok := findField(record.ScopesValues, ianaField(ipfixFieldApplicationID))
	if !ok {
		return
	}
	id, ok := applicationIDToUint64(rawID)
	if !ok {
		return
	}

	apps := m.apps[exporterIP]
	app, known := apps[id]
	if name := stringField(record.OptionsValues, ianaField(ipfixFieldApplicationName)); name != "" {
		if name != app.ApplicationName {
			// the id now maps to a different application, so drop the old name, description and metadata
			app = common.DPIApplication{ID: id, ApplicationName: name}
		}
		app.ApplicationDescription = stringField(record.OptionsValues, ianaField(ipfixFieldApplicationDescription))
		known = true
	}
	if !known {
		// never cache metadata for an id without a name
		return
	}
	// only replace metadata when the record has some, so name-only records keep it
	if metadata := extractMetadata(record.OptionsValues); metadata != (common.DPIApplicationMetadata{}) {
		app.DPIApplicationMetadata = metadata
	}

	if apps == nil {
		apps = make(map[uint64]common.DPIApplication)
		m.apps[exporterIP] = apps
	}
	apps[id] = app
}

// extractMetadata reads the attributes of Cisco NBAR's `option application-attributes` records
func extractMetadata(fields []netflow.DataField) common.DPIApplicationMetadata {
	return common.DPIApplicationMetadata{
		Category:            stringField(fields, ciscoField(ciscoFieldApplicationCategory)),
		SubCategory:         stringField(fields, ciscoField(ciscoFieldApplicationSubCategory)),
		ApplicationGroup:    stringField(fields, ciscoField(ciscoFieldApplicationGroup)),
		P2PTechnology:       stringField(fields, ianaField(ipfixFieldP2PTechnology)),
		TunnelTechnology:    stringField(fields, ianaField(ipfixFieldTunnelTechnology)),
		EncryptedTechnology: stringField(fields, ianaField(ipfixFieldEncryptedTechnology)),
		TrafficClass:        stringField(fields, ciscoField(ciscoFieldApplicationTrafficClass)),
		BusinessRelevance:   stringField(fields, ciscoField(ciscoFieldApplicationBusinessRelevance)),
		ApplicationSet:      stringField(fields, ciscoField(ciscoFieldApplicationSet)),
		ApplicationFamily:   stringField(fields, ciscoField(ciscoFieldApplicationFamily)),
	}
}

// fieldKey identifies an IPFIX field by its number and enterprise number (pen 0 for IANA fields)
type fieldKey struct {
	pen       uint32
	fieldType uint16
}

func ianaField(fieldType uint16) fieldKey {
	return fieldKey{fieldType: fieldType}
}

func ciscoField(fieldType uint16) fieldKey {
	return fieldKey{pen: ciscoPEN, fieldType: fieldType}
}

func stringField(fields []netflow.DataField, key fieldKey) string {
	v, _ := findField(fields, key)
	// remove null and space padding of fixed-width fields
	return string(bytes.Trim(v, "\x00 "))
}

func findField(fields []netflow.DataField, key fieldKey) ([]byte, bool) {
	for _, f := range fields {
		if v, ok := f.Value.([]byte); ok && matchesField(f, key) {
			return v, true
		}
	}
	return nil, false
}

// check if this is the field identified by key, comparing both field number and enterprise number
func matchesField(f netflow.DataField, key fieldKey) bool {
	// ignore the enterprise bit (0x8000), which goflow2 strips from data template fields but not options templates
	return f.PenProvided == (key.pen != 0) && f.Pen == key.pen && f.Type&^0x8000 == key.fieldType
}
