// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package dpioptions decodes the application tables exporters send in IPFIX options records,
// so that the dpi package only deals with typed values.
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

	// Cisco NBAR's `option application-attributes`
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
}

// DecodeApplications decodes the applications announced in the options records of an IPFIX packet
func DecodeApplications(msgDec interface{}) []dpi.Application {
	ipfixPacket, ok := msgDec.(netflow.IPFIXPacket)
	if !ok {
		return nil
	}

	_, _, _, optionsDataFlowSet := producer.SplitIPFIXSets(ipfixPacket)

	var apps []dpi.Application
	for _, dataFlowSet := range optionsDataFlowSet {
		for _, record := range dataFlowSet.Records {
			if app, ok := decodeApplication(record); ok {
				apps = append(apps, app)
			}
		}
	}
	return apps
}

func decodeApplication(record netflow.OptionsDataRecord) (dpi.Application, bool) {
	var app dpi.Application
	for _, values := range [][]netflow.DataField{record.ScopesValues, record.OptionsValues} {
		for _, df := range values {
			v, ok := df.Value.([]byte)
			if !ok {
				continue
			}
			key := optionsFieldKey(df)
			if key == applicationIDKey {
				var id uint64
				if producer.DecodeUNumber(v, &id) == nil {
					app.ID = id
				}
			} else if set, ok := stringFields[key]; ok {
				set(&app, string(bytes.Trim(v, "\x00"))) // removing null padding
			}
		}
	}
	return app, app.ID != 0
}

func optionsFieldKey(df netflow.DataField) fieldKey {
	if !df.PenProvided {
		return fieldKey{field: df.Type}
	}
	// goflow2 leaves the enterprise bit (0x8000) set on options template fields
	return fieldKey{pen: df.Pen, field: df.Type &^ 0x8000}
}
