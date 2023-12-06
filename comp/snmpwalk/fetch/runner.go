// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package fetch

import (
	"encoding/json"
	"github.com/DataDog/datadog-agent/comp/snmpwalk/fetch/valuestore"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/epforwarder"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/metadata"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/profile/profiledefinition"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	parse "github.com/DataDog/datadog-agent/pkg/snmp/snmpparse"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/gosnmp/gosnmp"
	"os"
	"strings"
	"time"
)

// SnmpwalkRunner receives configuration from remote-config
type SnmpwalkRunner struct {
	upToDate bool
	sender   sender.Sender
}

// NewSnmpwalkRunner creates a new SnmpwalkRunner.
func NewSnmpwalkRunner(sender sender.Sender) *SnmpwalkRunner {
	return &SnmpwalkRunner{
		upToDate: false,
		sender:   sender,
	}
}

// Callback is when profiles updates are available (rc product NDM_DEVICE_PROFILES_CUSTOM)
func (rc *SnmpwalkRunner) Callback(updates map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
	log.Info("[RC Callback] RC Callback")

	// TODO: Do not collect snmp-listener configs
	snmpConfigList, err := parse.GetConfigCheckSnmp()
	if err != nil {
		log.Infof("[RC Callback] Couldn't parse the SNMP config: %v", err)
		return
	}
	log.Infof("[RC Callback] snmpConfigList len=%d", len(snmpConfigList))

	for _, config := range snmpConfigList {
		log.Infof("[RC Callback] SNMP config: %+v", config)
	}

	var profiles []profiledefinition.ProfileDefinition
	for path, rawConfig := range updates {
		log.Infof("[RC Callback] Path: %s", path)

		profileDef := profiledefinition.DeviceProfileRcConfig{}
		json.Unmarshal(rawConfig.Config, &profileDef)

		log.Infof("[RC Callback] Profile Name: %s", profileDef.Profile.Name)
		log.Infof("[RC Callback] Profile Desc: %s", profileDef.Profile.Description)

		profiles = append(profiles, profileDef.Profile)

		for _, config := range snmpConfigList {
			ipaddr := config.IPAddress
			if ipaddr != "" && strings.Contains(profileDef.Profile.Description, ipaddr) {
				log.Infof("[RC Callback] Run Device OID Scan for: %s", ipaddr)

				rc.collectDeviceOIDs(config)
			}
		}
	}
}

func (rc *SnmpwalkRunner) collectDeviceOIDs(config parse.SNMPConfig) {
	namespace := "default" // TODO: CHANGE PLACEHOLDER
	deviceId := namespace + ":" + config.IPAddress

	session := createSession(config)
	log.Infof("[RC Callback] session: %+v", session)

	// Establish connection
	err := session.Connect()
	if err != nil {
		log.Errorf("[RC Callback] Connect err: %v\n", err)
		os.Exit(1)
		return
	}
	defer session.Conn.Close()

	variables := FetchAllFirstRowOIDsVariables(session)
	log.Infof("[RC Callback] Variables: %d", len(variables))

	for _, variable := range variables {
		log.Infof("[RC Callback] Variable Name: %s", variable.Name)
	}

	deviceOIDs := buildDeviceScanMetadata(deviceId, variables)
	metadataPayloads := metadata.BatchPayloads(namespace,
		"",
		time.Now(),
		metadata.PayloadMetadataBatchSize,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		deviceOIDs,
	)
	for _, payload := range metadataPayloads {
		payloadBytes, err := json.Marshal(payload)
		if err != nil {
			log.Errorf("[RC Callback] Error marshalling device metadata: %s", err)
			continue
		}
		log.Debugf("[RC Callback] Device OID metadata payload: %s", string(payloadBytes))
		rc.sender.EventPlatformEvent(payloadBytes, epforwarder.EventTypeNetworkDevicesMetadata)
		if err != nil {
			log.Errorf("[RC Callback] Error sending event platform event for Device OID metadata: %s", err)
		}
	}
}

func buildDeviceScanMetadata(deviceId string, oidsValues []gosnmp.SnmpPDU) []metadata.DeviceOid {
	var deviceOids []metadata.DeviceOid
	if oidsValues == nil {
		return deviceOids
	}
	for _, variablePdu := range oidsValues {
		_, value, err := valuestore.GetResultValueFromPDU(variablePdu)
		if err != nil {
			log.Debugf("GetValueFromPDU error: %s", err)
			continue
		}

		// TODO: How to store different types? Use Base64?
		strValue, err := value.ToString()
		if err != nil {
			log.Debugf("ToString error: %s", err)
			continue
		}

		deviceOids = append(deviceOids, metadata.DeviceOid{
			DeviceID:    deviceId,
			Oid:         strings.TrimLeft(variablePdu.Name, "."),
			Type:        variablePdu.Type.String(), // TODO: Map to internal types?
			ValueString: strValue,
		})
	}
	return deviceOids
}
