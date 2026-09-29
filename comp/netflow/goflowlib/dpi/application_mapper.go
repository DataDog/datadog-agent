// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package dpi

import (
	"bytes"
	"sync"

	"github.com/netsampler/goflow2/decoders/netflow"
)

const (
	ipfixFieldApplicationDescription uint16 = 94
	ipfixFieldApplicationID          uint16 = 95
	ipfixFieldApplicationName        uint16 = 96
)

type Application struct {
	name        string
	description string
}

type ApplicationMapper struct {
	mu   sync.RWMutex
	apps map[string]map[uint64]Application // exporterIP -> applicationId -> Application
}

func NewApplicationMapper() *ApplicationMapper {
	return &ApplicationMapper{apps: make(map[string]map[uint64]Application)}
}

func (m *ApplicationMapper) lookupApplication(exporterIP string, rawAppID []byte) (Application, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	apps, ok := m.apps[exporterIP]
	if !ok {
		return Application{}, false
	}

	id, ok := applicationIDToUint64(rawAppID)
	if !ok {
		return Application{}, false
	}

	app, found := apps[id]
	return app, found
}

func (m *ApplicationMapper) addToCache(exporterIP string, optionsDataFlowSet []netflow.OptionsDataFlowSet) {
	for _, dataFlowSet := range optionsDataFlowSet {
		for _, record := range dataFlowSet.Records {
			if id, app, ok := extractApplication(record); ok {
				m.set(exporterIP, id, app)
			}
		}
	}
}

// gets an Application from IPFIX Options Data Record applicationId (95) from the scope fields,
// applicationName (96) & applicationDescription (94) from the option fields
func extractApplication(record netflow.OptionsDataRecord) (uint64, Application, bool) {
	app := Application{}

	id, haveID := findField(record.ScopesValues, ipfixFieldApplicationID)
	if !haveID {
		return 0, app, false
	}

	name, haveName := findField(record.OptionsValues, ipfixFieldApplicationName)
	if haveName {
		// strip the trailing null padding exporters use for fixed-width string fields
		app = Application{name: string(bytes.Trim(name, "\x00"))}
	} else {
		return 0, app, false
	}

	description, haveDescription := findField(record.OptionsValues, ipfixFieldApplicationDescription)
	if haveDescription {
		// trim null padding
		app.description = string(bytes.Trim(description, "\x00"))
	}

	key, ok := applicationIDToUint64(id)
	if !ok {
		return 0, app, false
	}
	return key, app, true
}

// returns the raw bytes of the given field type
func findField(fields []netflow.DataField, fieldType uint16) ([]byte, bool) {
	for _, f := range fields {
		if v, ok := f.Value.([]byte); f.Type == fieldType && ok {
			// found the requested type with a well-formed value
			return v, true
		}
	}
	return nil, false
}

func (m *ApplicationMapper) set(exporterIP string, id uint64, app Application) {
	m.mu.Lock()
	defer m.mu.Unlock()
	apps, ok := m.apps[exporterIP]
	if !ok {
		apps = make(map[uint64]Application)
		m.apps[exporterIP] = apps
	}
	apps[id] = app
}
