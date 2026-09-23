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

func ProcessMessageApplicationNames(msgDec interface{}, exporterIP string, mapper *ApplicationMapper) []common.AdditionalFields {
	if mapper == nil {
		return nil
	}

	ipfixPacket, ok := msgDec.(netflow.IPFIXPacket)
	if !ok {
		return nil
	}

	dataFlowSet, _, _, optionsDataFlowSet := producer.SplitIPFIXSets(ipfixPacket)
	mapper.addToCache(exporterIP, optionsDataFlowSet)

	var flowsFields []common.AdditionalFields
	for _, fs := range dataFlowSet {
		for _, record := range fs.Records {
			fields := make(common.AdditionalFields)
			for _, df := range record.Values {
				if df.Type != ipfixFieldApplicationID {
					continue
				}
				v, ok := df.Value.([]byte)
				if !ok {
					continue
				}
				if app, found := mapper.lookupApplication(exporterIP, v); found {
					fields["dpi.application_name"] = app.applicationName
					fields["dpi.application_description"] = app.applicationDescription
				}
			}
			flowsFields = append(flowsFields, fields)
		}
	}
	return flowsFields
}
