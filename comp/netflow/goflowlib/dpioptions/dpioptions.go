// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package dpioptions decodes the application tables exporters send in IPFIX options records,
// so that the dpi package only deals with typed values.
package dpioptions

import (
	"slices"

	"github.com/netsampler/goflow2/decoders/netflow"
	"github.com/netsampler/goflow2/producer"

	"github.com/DataDog/datadog-agent/comp/netflow/common"
	config "github.com/DataDog/datadog-agent/comp/netflow/config/def"
	"github.com/DataDog/datadog-agent/comp/netflow/dpi"
	"github.com/DataDog/datadog-agent/comp/netflow/goflowlib/additionalfields"
)

// destinations of the application fields collected from options records
const (
	applicationID          = "application_id"
	applicationName        = "application_name"
	applicationDescription = "application_description"
	category               = "category"
	subCategory            = "sub_category"
	applicationGroup       = "application_group"
	p2pTechnology          = "p2p_technology"
	tunnelTechnology       = "tunnel_technology"
	encryptedTechnology    = "encrypted_technology"
	trafficClass           = "traffic_class"
	businessRelevance      = "business_relevance"
	applicationSet         = "application_set"
	applicationFamily      = "application_family"
)

var applicationMappings = map[uint16]config.Mapping{
	ipfixFieldApplicationID:          {Field: ipfixFieldApplicationID, Type: common.Integer, Destination: applicationID, Endian: common.BigEndian, MatchPen: true},
	ipfixFieldApplicationName:        stringMapping(ipfixFieldApplicationName, applicationName, 0),
	ipfixFieldApplicationDescription: stringMapping(ipfixFieldApplicationDescription, applicationDescription, 0),

	// Cisco NBAR's `option application-attributes`
	ciscoFieldApplicationCategory:          stringMapping(ciscoFieldApplicationCategory, category, ciscoPEN),
	ciscoFieldApplicationSubCategory:       stringMapping(ciscoFieldApplicationSubCategory, subCategory, ciscoPEN),
	ciscoFieldApplicationGroup:             stringMapping(ciscoFieldApplicationGroup, applicationGroup, ciscoPEN),
	ipfixFieldP2PTechnology:                stringMapping(ipfixFieldP2PTechnology, p2pTechnology, 0),
	ipfixFieldTunnelTechnology:             stringMapping(ipfixFieldTunnelTechnology, tunnelTechnology, 0),
	ipfixFieldEncryptedTechnology:          stringMapping(ipfixFieldEncryptedTechnology, encryptedTechnology, 0),
	ciscoFieldApplicationTrafficClass:      stringMapping(ciscoFieldApplicationTrafficClass, trafficClass, ciscoPEN),
	ciscoFieldApplicationBusinessRelevance: stringMapping(ciscoFieldApplicationBusinessRelevance, businessRelevance, ciscoPEN),
	ciscoFieldApplicationSet:               stringMapping(ciscoFieldApplicationSet, applicationSet, ciscoPEN),
	ciscoFieldApplicationFamily:            stringMapping(ciscoFieldApplicationFamily, applicationFamily, ciscoPEN),
}

// stringMapping maps a string field of the given enterprise number (0 for IANA fields)
func stringMapping(field uint16, destination string, pen uint32) config.Mapping {
	return config.Mapping{Field: field, Type: common.String, Destination: destination, MatchPen: true, Pen: pen}
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
	fields := additionalfields.ConvertNetFlowDataSet(slices.Concat(record.ScopesValues, record.OptionsValues), applicationMappings)
	id, _ := fields[applicationID].(uint64)
	if id == 0 {
		return dpi.Application{}, false
	}
	return dpi.Application{
		ID:          id,
		Name:        stringField(fields, applicationName),
		Description: stringField(fields, applicationDescription),
		Metadata: dpi.Metadata{
			Category:            stringField(fields, category),
			SubCategory:         stringField(fields, subCategory),
			ApplicationGroup:    stringField(fields, applicationGroup),
			P2PTechnology:       stringField(fields, p2pTechnology),
			TunnelTechnology:    stringField(fields, tunnelTechnology),
			EncryptedTechnology: stringField(fields, encryptedTechnology),
			TrafficClass:        stringField(fields, trafficClass),
			BusinessRelevance:   stringField(fields, businessRelevance),
			ApplicationSet:      stringField(fields, applicationSet),
			ApplicationFamily:   stringField(fields, applicationFamily),
		},
	}, true
}

func stringField(fields common.AdditionalFields, destination string) string {
	s, _ := fields[destination].(string)
	return s
}
