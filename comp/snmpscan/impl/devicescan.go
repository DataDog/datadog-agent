package snmpscanimpl

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/metadata"
	"github.com/DataDog/datadog-agent/pkg/snmp/gosnmplib"
	"github.com/gosnmp/gosnmp"
)

func (s snmpScannerImpl) RunDeviceScan(snmpConnection *gosnmp.GoSNMP, deviceNamespace string) error {
	forwarder, err := s.demux.GetEventPlatformForwarder()
	if err != nil {
		return fmt.Errorf("unable to get sender: %w", err)
	}

	if err := snmpConnection.Connect(); err != nil {
		return fmt.Errorf("unable to connect to SNMP agent on %s:%d: %w", snmpConnection.LocalAddr, snmpConnection.Port, err)
	}
	defer snmpConnection.Conn.Close()
	pdus, err := gatherPDUs(snmpConnection)
	if err != nil {
		return err
	}

	deviceID := deviceNamespace + ":" + snmpConnection.LocalAddr
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
		payloadBytes, err := json.Marshal(payload)
		if err != nil {
			s.log.Errorf("Error marshalling device metadata: %v", err)
			continue
		}
		m := message.NewMessage(payloadBytes, nil, "", 0)
		s.log.Debugf("Device OID metadata payload is %d bytes", len(payloadBytes))
		s.log.Tracef("Device OID metadata payload: %s", string(payloadBytes))
		if err := forwarder.SendEventPlatformEventBlocking(m, eventplatform.EventTypeNetworkDevicesMetadata); err != nil {
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
