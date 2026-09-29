// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package dpi provides deep packet inspection enrichment for IPFIX flows,
// resolved from applicationId/applicationName reported by exporters.
package dpi

import (
	"github.com/netsampler/goflow2/decoders/netflow"
	"github.com/netsampler/goflow2/producer"

	"github.com/DataDog/datadog-agent/comp/netflow/common"
)

func ProcessMessageApplicationNames(msgDec interface{}, exporterIP string, mapper *ApplicationMapper) []common.DPIFields {
	if mapper == nil {
		return nil
	}

	ipfixPacket, ok := msgDec.(netflow.IPFIXPacket)
	if !ok {
		return nil
	}

	dataFlowSet, _, _, optionsDataFlowSet := producer.SplitIPFIXSets(ipfixPacket)
	mapper.addToCache(exporterIP, optionsDataFlowSet)

	var flowsFields []common.DPIFields
	for _, fs := range dataFlowSet {
		for _, record := range fs.Records {
			var fields common.DPIFields
			for _, df := range record.Values {
				if df.Type != ipfixFieldApplicationID {
					continue
				}
				v, ok := df.Value.([]byte)
				if !ok {
					continue
				}
				if app, found := mapper.lookupApplication(exporterIP, v); found {
					id, _ := applicationIDToUint64(v)
					fields = common.DPIFields{
						ID:                     id,
						ApplicationName:        app.name,
						ApplicationDescription: app.description,
					}
				}
			}
			flowsFields = append(flowsFields, fields)
		}
	}
	return flowsFields
}

func applicationIDToUint64(raw []byte) (uint64, bool) {
	var id uint64
	if err := producer.DecodeUNumber(raw, &id); err != nil {
		return 0, false
	}
	return id, true
}
