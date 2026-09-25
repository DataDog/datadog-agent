// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package dpi

import (
	"encoding/binary"
	"testing"

	"github.com/netsampler/goflow2/decoders/netflow"
	"github.com/stretchr/testify/assert"
)

func appIDtoBytes(id uint32) []byte {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, uint64(id))
	return buf
}

func httpOptionsRecord(id uint32) netflow.OptionsDataRecord {
	return netflow.OptionsDataRecord{
		ScopesValues: []netflow.DataField{{Type: ipfixFieldApplicationID, Value: appIDtoBytes(id)}},
		OptionsValues: []netflow.DataField{
			{Type: ipfixFieldApplicationName, Value: []byte("HTTP\x00\x00")},
			{Type: ipfixFieldApplicationDescription, Value: []byte("Hypertext Transfer Protocol\x00")},
		},
	}
}

func TestApplicationMapper_addToCache(t *testing.T) {
	validRecord := httpOptionsRecord(100)
	validRecord.OptionsValues = append(validRecord.OptionsValues, netflow.DataField{Type: 30, Value: []byte{0, 0, 0, 100}}) // unrelated field, must be ignored

	tests := []struct {
		name    string
		records []netflow.OptionsDataRecord
		check   func(t *testing.T, mapper *ApplicationMapper)
	}{
		{
			name:    "id and name present, with unrelated field ignored",
			records: []netflow.OptionsDataRecord{validRecord},
			check: func(t *testing.T, mapper *ApplicationMapper) {
				app, ok := mapper.lookupApplication("10.0.0.1", appIDtoBytes(100))
				assert.True(t, ok, "unrelated option fields must not prevent caching")
				assert.Equal(t, "HTTP", app.applicationName)
				assert.Equal(t, "Hypertext Transfer Protocol", app.applicationDescription)

				_, ok = mapper.lookupApplication("10.0.0.1", appIDtoBytes(101))
				assert.False(t, ok, "unrelated application ids must not resolve")

				dnsRecord := httpOptionsRecord(100)
				dnsRecord.OptionsValues = []netflow.DataField{{Type: ipfixFieldApplicationName, Value: []byte("DNS\x00")}}
				mapper.addToCache("10.0.0.2", []netflow.OptionsDataFlowSet{{Records: []netflow.OptionsDataRecord{dnsRecord}}})

				app, ok = mapper.lookupApplication("10.0.0.2", appIDtoBytes(100))
				assert.True(t, ok, "same application id from a different exporter must resolve independently")
				assert.Equal(t, "DNS", app.applicationName)
				assert.Empty(t, app.applicationDescription, "an options record with no description field must leave it empty")

				app, ok = mapper.lookupApplication("10.0.0.1", appIDtoBytes(100))
				assert.True(t, ok, "caching a second exporter must not disturb the first")
				assert.Equal(t, "HTTP", app.applicationName)
			},
		},
		{
			name: "missing application id is not cached",
			records: []netflow.OptionsDataRecord{
				{OptionsValues: []netflow.DataField{{Type: ipfixFieldApplicationName, Value: []byte("HTTP")}}},
			},
			check: func(t *testing.T, mapper *ApplicationMapper) {
				_, ok := mapper.lookupApplication("10.0.0.1", appIDtoBytes(100))
				assert.False(t, ok)
			},
		},
		{
			name: "missing application name is not cached",
			records: []netflow.OptionsDataRecord{
				{ScopesValues: []netflow.DataField{{Type: ipfixFieldApplicationID, Value: appIDtoBytes(100)}}},
			},
			check: func(t *testing.T, mapper *ApplicationMapper) {
				_, ok := mapper.lookupApplication("10.0.0.1", appIDtoBytes(100))
				assert.False(t, ok)
			},
		},
		{
			name:    "record with no fields at all",
			records: []netflow.OptionsDataRecord{{}},
			check: func(t *testing.T, mapper *ApplicationMapper) {
				_, ok := mapper.lookupApplication("10.0.0.1", appIDtoBytes(100))
				assert.False(t, ok)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mapper := NewApplicationMapper()
			mapper.addToCache("10.0.0.1", []netflow.OptionsDataFlowSet{{Records: tt.records}})
			tt.check(t, mapper)
		})
	}
}
