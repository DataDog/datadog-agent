package rcsnmpprofiles

import (
	parse "github.com/DataDog/datadog-agent/pkg/snmp/snmpparse"
	"github.com/gosnmp/gosnmp"
)

func createSession(config parse.SNMPConfig) gosnmp.GoSNMP {
	return gosnmp.GoSNMP{
		Target:    config.IPAddress,
		Port:      config.Port,
		Community: config.CommunityString,
		Transport: "udp",
		Version:   gosnmp.Version2c,
		//Timeout:   time.Duration(cliParams.timeout * int(time.Second)),
		//Retries:   cliParams.retries,
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
