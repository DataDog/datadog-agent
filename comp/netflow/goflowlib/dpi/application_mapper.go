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
	applicationName        string
	applicationDescription string
}

type ApplicationMapper struct {
	mu   sync.RWMutex
	apps map[string]map[string]Application // exporterIP -> applicationId -> {applicationName, applicationDescription}
}

func NewApplicationMapper() *ApplicationMapper {
	return &ApplicationMapper{apps: make(map[string]map[string]Application)}
}

func (m *ApplicationMapper) lookupApplication(exporterIP string, rawAppID []byte) (Application, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	apps, ok := m.apps[exporterIP]
	if !ok {
		return Application{}, false
	}

	app, found := apps[string(rawAppID)]
	return app, found
}

func (m *ApplicationMapper) addToCache(exporterIP string, optionsDataFlowSet []netflow.OptionsDataFlowSet) {
	for _, dataFlowSet := range optionsDataFlowSet {
		for _, record := range dataFlowSet.Records {
			appID, haveID := extractApplicationID(record.ScopesValues)
			appName, appDescription, haveName := extractApplicationName(record.OptionsValues)
			if haveID && haveName {
				m.set(exporterIP, appID, appName, appDescription)
			}
		}
	}
}

func extractApplicationID(fields []netflow.DataField) (appID string, haveID bool) {
	for _, f := range fields {
		if f.Type != ipfixFieldApplicationID {
			continue
		}
		v, ok := f.Value.([]byte)
		if !ok {
			continue
		}
		return string(v), true
	}
	return "", false
}

func extractApplicationName(fields []netflow.DataField) (appName string, appDescription string, haveName bool) {
	for _, f := range fields {
		v, ok := f.Value.([]byte)
		if !ok {
			continue
		}
		switch f.Type {
		case ipfixFieldApplicationName:
			appName = string(bytes.Trim(v, "\x00"))
			haveName = true
		case ipfixFieldApplicationDescription:
			appDescription = string(bytes.Trim(v, "\x00"))
		}
	}
	return appName, appDescription, haveName
}

func (m *ApplicationMapper) set(exporterIP string, appID string, appName string, appDescription string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	apps, ok := m.apps[exporterIP]
	if !ok {
		apps = make(map[string]Application)
		m.apps[exporterIP] = apps
	}
	apps[appID] = Application{applicationName: appName, applicationDescription: appDescription}
}

func CreateApplicationMapper(enableDPI bool) *ApplicationMapper {
	if !enableDPI {
		return nil
	}
	return NewApplicationMapper()
}
