// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package dpioptions

import (
	"encoding/binary"
	"testing"

	"github.com/netsampler/goflow2/decoders/netflow"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/netflow/dpi"
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
			{Type: 30, Value: []byte{0, 0, 0, 100}},                                                 // unrelated field, should be ignored
			{PenProvided: true, Pen: ciscoPEN, Type: ipfixFieldApplicationName, Value: []byte("x")}, // enterprise field sharing the IANA number, should be ignored
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

func optionsPacket(records ...netflow.OptionsDataRecord) netflow.IPFIXPacket {
	return netflow.IPFIXPacket{
		Version: 10,
		FlowSets: []interface{}{
			netflow.OptionsDataFlowSet{
				FlowSetHeader: netflow.FlowSetHeader{Id: 257, Length: 20},
				Records:       records,
			},
		},
	}
}

func TestDecodeApplications(t *testing.T) {
	apps := DecodeApplications(optionsPacket(
		httpOptionsRecord(100),
		attributesOptionsRecord(100),
		netflow.OptionsDataRecord{OptionsValues: []netflow.DataField{{Type: ipfixFieldApplicationName, Value: []byte("no id")}}},
		netflow.OptionsDataRecord{},
	))
	assert.Equal(t, []dpi.Application{
		{ID: 100, Name: "HTTP", Description: "Hypertext Transfer Protocol"},
		{ID: 100, Metadata: dpi.Metadata{
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
		}},
	}, apps, "null padding is trimmed, fields of other enterprises are ignored and records without an id are skipped")
}

func TestDecodeApplications_enterpriseSpecificIDs(t *testing.T) {
	ciscoID := make([]byte, 8)
	binary.BigEndian.PutUint32(ciscoID[0:4], 9) // Cisco's PEN
	binary.BigEndian.PutUint32(ciscoID[4:8], 100)

	otherID := make([]byte, 8)
	binary.BigEndian.PutUint32(otherID[0:4], 12345) // a different enterprise's PEN
	binary.BigEndian.PutUint32(otherID[4:8], 100)   // same low 32 bits as ciscoID

	apps := DecodeApplications(optionsPacket(
		netflow.OptionsDataRecord{
			ScopesValues:  []netflow.DataField{{Type: ipfixFieldApplicationID, Value: ciscoID}},
			OptionsValues: []netflow.DataField{{Type: ipfixFieldApplicationName, Value: []byte("nbar:webex")}},
		},
		netflow.OptionsDataRecord{
			ScopesValues:  []netflow.DataField{{Type: ipfixFieldApplicationID, Value: otherID}},
			OptionsValues: []netflow.DataField{{Type: ipfixFieldApplicationName, Value: []byte("other:app")}},
		},
	))
	assert.Equal(t, []dpi.Application{
		{ID: binary.BigEndian.Uint64(ciscoID), Name: "nbar:webex"},
		{ID: binary.BigEndian.Uint64(otherID), Name: "other:app"},
	}, apps, "ids sharing their low 32 bits but differing in their enterprise number must decode to distinct ids")
}

func TestDecodeApplications_noOptions(t *testing.T) {
	dataPacket := netflow.IPFIXPacket{
		Version: 10,
		FlowSets: []interface{}{
			netflow.DataFlowSet{
				Records: []netflow.DataRecord{{Values: []netflow.DataField{{Type: ipfixFieldApplicationID, Value: appIDtoBytes(100)}}}},
			},
		},
	}
	assert.Nil(t, DecodeApplications(dataPacket), "data records are left to the additional fields mapping")
	assert.Nil(t, DecodeApplications(netflow.NFv9Packet{}), "NFv9 is not supported")
}
