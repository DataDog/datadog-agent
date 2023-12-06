// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package fetch

import (
	parse "github.com/DataDog/datadog-agent/pkg/snmp/snmpparse"
	"github.com/gosnmp/gosnmp"
	"time"
)

func createSession(config parse.SNMPConfig) gosnmp.GoSNMP {
	return gosnmp.GoSNMP{
		Target:    config.IPAddress,
		Port:      config.Port,
		Community: config.CommunityString,
		Transport: "udp",
		Version:   gosnmp.Version2c,
		Timeout:   time.Duration(5 * int(time.Second)),
		Retries:   3,
		//// v3
		//SecurityModel: gosnmp.UserSecurityModel,
		//ContextName:   cliParams.snmpContext,
		//MsgFlags:      msgFlags,
		//SecurityParameters: &gosnmp.UsmSecurityParameters{
		//	UserName:                 cliParams.user,
		//	AuthenticationProtocol:   authProtocol,
		//	AuthenticationPassphrase: cliParams.authKey,
		//	PrivacyProtocol:          privProtocol,
		//	PrivacyPassphrase:        cliParams.privKey,
		//},
		//UseUnconnectedUDPSocket: cliParams.unconnectedUDPSocket,
	}
}
