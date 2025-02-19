// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package snmpscanimpl

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/networkdevice/metadata"
	"github.com/DataDog/datadog-agent/pkg/snmp/gosnmplib"
	"github.com/gosnmp/gosnmp"
)

func (s snmpScannerImpl) RunDeviceScan(snmpConnection *gosnmp.GoSNMP, deviceNamespace string, deviceID string) error {
	// execute the scan
	pdus, err := gatherPDUs(snmpConnection)
	if err != nil {
		return err
	}

	var deviceOids []*metadata.DeviceOID
	for _, pdu := range pdus {
		record, err := metadata.DeviceOIDFromPDU(deviceID, pdu)
		if err != nil {
			s.log.Warnf("PDU parsing error: %v", err)
			continue
		}
		deviceOids = append(deviceOids, record)
	}

	metadataPayloads := metadata.BatchDeviceScan(deviceNamespace, time.Now(), metadata.PayloadMetadataBatchSize, deviceOids)
	for _, payload := range metadataPayloads {
		err := s.SendPayload(payload)
		if err != nil {
			return err
		}
	}

	return nil
}

// gatherPDUs returns PDUs from the given SNMP device that should cover ever
// scalar value and at least one row of every table.
func gatherPDUs(snmp *gosnmp.GoSNMP) ([]*gosnmp.SnmpPDU, error) {
	var pdus []*gosnmp.SnmpPDU
	err := gosnmplib.ConditionalWalk(snmp, "", false, func(dataUnit gosnmp.SnmpPDU) (string, error) {
		pdus = append(pdus, &dataUnit)
		return gosnmplib.SkipOIDRowsNaive(dataUnit.Name), nil
	})
	if err != nil {
		return nil, err
	}
	return pdus, nil
}
