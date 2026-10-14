// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package dpioptions decodes the application tables exporters send in NetFlow v9/IPFIX
// options records, so that the dpi package only deals with typed values.
package dpioptions

import (
	"bytes"

	"github.com/netsampler/goflow2/decoders/netflow"
	"github.com/netsampler/goflow2/producer"

	"github.com/DataDog/datadog-agent/comp/netflow/dpi"
)

// fieldKey identifies an options record field by its enterprise number and field number
type fieldKey struct {
	pen   uint32 // 0 for IANA fields
	field uint16
}

var applicationIDKey = fieldKey{field: ipfixFieldApplicationID}

// stringFields sets the application attribute carried by each string field of the application table records
var stringFields = map[fieldKey]func(app *dpi.Application, value string){
	{field: ipfixFieldApplicationName}:        func(app *dpi.Application, v string) { app.Name = v },
	{field: ipfixFieldApplicationDescription}: func(app *dpi.Application, v string) { app.Description = v },

	// Cisco NBAR's IPFIX `option application-attributes`
	{pen: ciscoPEN, field: ciscoFieldApplicationCategory}:          func(app *dpi.Application, v string) { app.Category = v },
	{pen: ciscoPEN, field: ciscoFieldApplicationSubCategory}:       func(app *dpi.Application, v string) { app.SubCategory = v },
	{pen: ciscoPEN, field: ciscoFieldApplicationGroup}:             func(app *dpi.Application, v string) { app.ApplicationGroup = v },
	{field: ipfixFieldP2PTechnology}:                               func(app *dpi.Application, v string) { app.P2PTechnology = v },
	{field: ipfixFieldTunnelTechnology}:                            func(app *dpi.Application, v string) { app.TunnelTechnology = v },
	{field: ipfixFieldEncryptedTechnology}:                         func(app *dpi.Application, v string) { app.EncryptedTechnology = v },
	{pen: ciscoPEN, field: ciscoFieldApplicationTrafficClass}:      func(app *dpi.Application, v string) { app.TrafficClass = v },
	{pen: ciscoPEN, field: ciscoFieldApplicationBusinessRelevance}: func(app *dpi.Application, v string) { app.BusinessRelevance = v },
	{pen: ciscoPEN, field: ciscoFieldApplicationSet}:               func(app *dpi.Application, v string) { app.ApplicationSet = v },
	{pen: ciscoPEN, field: ciscoFieldApplicationFamily}:            func(app *dpi.Application, v string) { app.ApplicationFamily = v },

	// Cisco NBAR's NetFlow v9 export of the same fields, with enterpriseBit set instead of a PEN
	{field: ciscoFieldApplicationCategory | enterpriseBit}:          func(app *dpi.Application, v string) { app.Category = v },
	{field: ciscoFieldApplicationSubCategory | enterpriseBit}:       func(app *dpi.Application, v string) { app.SubCategory = v },
	{field: ciscoFieldApplicationGroup | enterpriseBit}:             func(app *dpi.Application, v string) { app.ApplicationGroup = v },
	{field: ciscoFieldApplicationTrafficClass | enterpriseBit}:      func(app *dpi.Application, v string) { app.TrafficClass = v },
	{field: ciscoFieldApplicationBusinessRelevance | enterpriseBit}: func(app *dpi.Application, v string) { app.BusinessRelevance = v },
	{field: ciscoFieldApplicationSet | enterpriseBit}:               func(app *dpi.Application, v string) { app.ApplicationSet = v },
	{field: ciscoFieldApplicationFamily | enterpriseBit}:            func(app *dpi.Application, v string) { app.ApplicationFamily = v },
}

// DecodeApplications decodes the applications announced in the options records of a NetFlow v9 or IPFIX packet
func DecodeApplications(msgDec interface{}) []dpi.Application {
	var optionsDataFlowSet []netflow.OptionsDataFlowSet
	var isIPFIX bool
	switch packet := msgDec.(type) {
	case netflow.NFv9Packet:
		_, _, _, optionsDataFlowSet = producer.SplitNetFlowSets(packet)
	case netflow.IPFIXPacket:
		_, _, _, optionsDataFlowSet = producer.SplitIPFIXSets(packet)
		isIPFIX = true
	default:
		return nil
	}

	var apps []dpi.Application
	for _, dataFlowSet := range optionsDataFlowSet {
		for _, record := range dataFlowSet.Records {
			if app, ok := decodeApplication(record, isIPFIX); ok {
				apps = append(apps, app)
			}
		}
	}
	return apps
}

func decodeApplication(record netflow.OptionsDataRecord, isIPFIX bool) (dpi.Application, bool) {
	id, ok := decodeApplicationID(record, isIPFIX)
	if !ok {
		return dpi.Application{}, false
	}

	app := dpi.Application{ID: id}
	for _, df := range record.OptionsValues {
		v, ok := df.Value.([]byte)
		if !ok {
			continue
		}
		if set, ok := stringFields[optionsFieldKey(df)]; ok {
			set(&app, string(bytes.Trim(v, "\x00"))) // removing null padding
		}
	}
	return app, true
}

func decodeApplicationID(record netflow.OptionsDataRecord, isIPFIX bool) (uint64, bool) {
	// the application id is a scope value in IPFIX, but an option field in NetFlow v9
	idFields := record.OptionsValues
	if isIPFIX {
		idFields = record.ScopesValues
	}
	for _, df := range idFields {
		v, ok := df.Value.([]byte)
		if !ok || optionsFieldKey(df) != applicationIDKey {
			continue
		}
		var id uint64
		if producer.DecodeUNumber(v, &id) == nil && id != 0 {
			return id, true
		}
	}
	return 0, false
}

func optionsFieldKey(df netflow.DataField) fieldKey {
	if !df.PenProvided {
		// NetFlow v9 fields have no PEN, so the enterprise bit (if any) is kept as part of the field type
		return fieldKey{field: df.Type}
	}
	// IPFIX enterprise fields: the PEN identifies the vendor,
	// so strip the enterprise bit (left set by goflow2) to get the field number
	return fieldKey{pen: df.Pen, field: df.Type &^ enterpriseBit}
}
