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

	"github.com/DataDog/datadog-agent/comp/netflow/common"
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

func ciscoOptionField(fieldType uint16, value string) netflow.DataField {
	// goflow2 leaves the enterprise bit (0x8000) set on options template fields
	return netflow.DataField{PenProvided: true, Pen: ciscoPEN, Type: 0x8000 | fieldType, Value: []byte(value + "\x00\x00")}
}

// mirrors the `option application-attributes`
func attributesOptionsRecord(id uint32) netflow.OptionsDataRecord {
	return netflow.OptionsDataRecord{
		ScopesValues: []netflow.DataField{{Type: ipfixFieldApplicationID, Value: appIDtoBytes(id)}},
		OptionsValues: []netflow.DataField{
			{PenProvided: true, Pen: 12345, Type: 0x8000 | ciscoFieldApplicationCategory, Value: []byte("other-vendor")}, // must be ignored
			ciscoOptionField(ciscoFieldApplicationCategory, "browsing"),
			ciscoOptionField(ciscoFieldApplicationSubCategory, "other"),
			ciscoOptionField(ciscoFieldApplicationGroup, "other"),
			ciscoOptionField(ciscoFieldApplicationTrafficClass, "transactional-data"),
			ciscoOptionField(ciscoFieldApplicationBusinessRelevance, "default"),
			{Type: ipfixFieldP2PTechnology, Value: []byte("no\x00")},
			{Type: ipfixFieldTunnelTechnology, Value: []byte("no\x00")},
			{Type: ipfixFieldEncryptedTechnology, Value: []byte("yes\x00")},
			ciscoOptionField(ciscoFieldApplicationSet, "general-browsing"),
			ciscoOptionField(ciscoFieldApplicationFamily, "encrypted"),
		},
	}
}

var browsingMetadata = common.DPIApplicationMetadata{
	Category:            "browsing",
	SubCategory:         "other",
	ApplicationGroup:    "other",
	P2PTechnology:       "no",
	TunnelTechnology:    "no",
	EncryptedTechnology: "yes",
	TrafficClass:        "transactional-data",
	BusinessRelevance:   "default",
	ApplicationSet:      "general-browsing",
	ApplicationFamily:   "encrypted",
}

func TestApplicationMapper_addToCache(t *testing.T) {
	validRecord := httpOptionsRecord(100)
	validRecord.OptionsValues = append(validRecord.OptionsValues,
		netflow.DataField{Type: 30, Value: []byte{0, 0, 0, 100}},                                                 // unrelated field, should be ignored
		netflow.DataField{PenProvided: true, Pen: ciscoPEN, Type: ipfixFieldApplicationName, Value: []byte("x")}, // enterprise field sharing the IANA number, should be ignored
	)

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
				assert.Equal(t, "HTTP", app.ApplicationName)
				assert.Equal(t, "Hypertext Transfer Protocol", app.ApplicationDescription)

				_, ok = mapper.lookupApplication("10.0.0.1", appIDtoBytes(101))
				assert.False(t, ok, "unrelated application ids must not resolve")

				dnsRecord := httpOptionsRecord(100)
				dnsRecord.OptionsValues = []netflow.DataField{{Type: ipfixFieldApplicationName, Value: []byte("DNS\x00")}}
				mapper.addToCache("10.0.0.2", []netflow.OptionsDataFlowSet{{Records: []netflow.OptionsDataRecord{dnsRecord}}})

				app, ok = mapper.lookupApplication("10.0.0.2", appIDtoBytes(100))
				assert.True(t, ok, "same application id from a different exporter must resolve independently")
				assert.Equal(t, "DNS", app.ApplicationName)
				assert.Empty(t, app.ApplicationDescription, "an options record with no description field must leave it empty")

				app, ok = mapper.lookupApplication("10.0.0.1", appIDtoBytes(100))
				assert.True(t, ok, "caching a second exporter must not disturb the first")
				assert.Equal(t, "HTTP", app.ApplicationName)
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
			name: "missing or empty application name is not cached, even with attributes",
			records: []netflow.OptionsDataRecord{
				{ScopesValues: []netflow.DataField{{Type: ipfixFieldApplicationID, Value: appIDtoBytes(100)}}},
				{
					ScopesValues:  []netflow.DataField{{Type: ipfixFieldApplicationID, Value: appIDtoBytes(100)}},
					OptionsValues: []netflow.DataField{{Type: ipfixFieldApplicationName, Value: []byte("\x00\x00")}},
				},
				attributesOptionsRecord(100),
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
		{
			name:    "attributes received before the name are dropped",
			records: []netflow.OptionsDataRecord{attributesOptionsRecord(100), httpOptionsRecord(100)},
			check: func(t *testing.T, mapper *ApplicationMapper) {
				app, _ := mapper.lookupApplication("10.0.0.1", appIDtoBytes(100))
				assert.Equal(t, "HTTP", app.ApplicationName)
				assert.Empty(t, app.DPIApplicationMetadata)
			},
		},
		{
			name:    "attributes are kept when the same name is refreshed",
			records: []netflow.OptionsDataRecord{httpOptionsRecord(100), attributesOptionsRecord(100), httpOptionsRecord(100)},
			check: func(t *testing.T, mapper *ApplicationMapper) {
				app, _ := mapper.lookupApplication("10.0.0.1", appIDtoBytes(100))
				assert.Equal(t, browsingMetadata, app.DPIApplicationMetadata)
			},
		},
		{
			name: "new name for a cached id evicts the previous application",
			records: []netflow.OptionsDataRecord{
				httpOptionsRecord(100),
				attributesOptionsRecord(100),
				{
					ScopesValues: []netflow.DataField{{Type: ipfixFieldApplicationID, Value: appIDtoBytes(100)}},
					OptionsValues: []netflow.DataField{
						{Type: ipfixFieldApplicationName, Value: []byte("DNS\x00")},
						ciscoOptionField(ciscoFieldApplicationCategory, "net-admin"),
					},
				},
			},
			check: func(t *testing.T, mapper *ApplicationMapper) {
				app, _ := mapper.lookupApplication("10.0.0.1", appIDtoBytes(100))
				assert.Equal(t, common.DPIApplication{
					ID:                     100,
					ApplicationName:        "DNS",
					DPIApplicationMetadata: common.DPIApplicationMetadata{Category: "net-admin"},
				}, app, "nothing from the previous application must carry over")
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

func TestApplicationMapper_enterpriseSpecificIDs(t *testing.T) {
	ciscoID := make([]byte, 8)
	binary.BigEndian.PutUint32(ciscoID[0:4], 9) // Cisco's PEN
	binary.BigEndian.PutUint32(ciscoID[4:8], 100)

	otherID := make([]byte, 8)
	binary.BigEndian.PutUint32(otherID[0:4], 12345) // a different enterprise's PEN
	binary.BigEndian.PutUint32(otherID[4:8], 100)   // same low 32 bits as ciscoID

	ciscoRecord := netflow.OptionsDataRecord{
		ScopesValues:  []netflow.DataField{{Type: ipfixFieldApplicationID, Value: ciscoID}},
		OptionsValues: []netflow.DataField{{Type: ipfixFieldApplicationName, Value: []byte("nbar:webex")}},
	}
	otherRecord := netflow.OptionsDataRecord{
		ScopesValues:  []netflow.DataField{{Type: ipfixFieldApplicationID, Value: otherID}},
		OptionsValues: []netflow.DataField{{Type: ipfixFieldApplicationName, Value: []byte("other:app")}},
	}

	mapper := NewApplicationMapper()
	mapper.addToCache("10.0.0.1", []netflow.OptionsDataFlowSet{{Records: []netflow.OptionsDataRecord{ciscoRecord, otherRecord}}})

	app, ok := mapper.lookupApplication("10.0.0.1", ciscoID)
	assert.True(t, ok)
	assert.Equal(t, "nbar:webex", app.ApplicationName)

	app, ok = mapper.lookupApplication("10.0.0.1", otherID)
	assert.True(t, ok)
	assert.Equal(t, "other:app", app.ApplicationName, "ids sharing their low 32 bits but differing in their enterprise number must not overwrite each other")
}
