package snmp

import (
	"fmt"
	"time"

	"github.com/gosnmp/gosnmp"
)

const sysObjectIDOid = "1.3.6.1.2.1.1.2.0"

// Java SNMP uses 50, snmp-net uses 10
// Same max repetition as gosnmp.defaultMaxRepetitions
const bulkMaxRepetition = 50

type sessionAPI interface {
	Configure(config snmpConfig) error
	Connect() error
	Close() error
	Get(oids []string) (result *gosnmp.SnmpPacket, err error)
	GetBulk(oids []string) (result *gosnmp.SnmpPacket, err error)
	GetNext(oids []string) (result *gosnmp.SnmpPacket, err error)
	GetVersion() gosnmp.SnmpVersion
}

type snmpSession struct {
	gosnmpInst gosnmp.GoSNMP
}

func (s *snmpSession) Configure(config snmpConfig) error {
	if config.oidBatchSize > gosnmp.MaxOids {
		return fmt.Errorf("config oidBatchSize (%d) cannot be higher than gosnmp.MaxOids: %d", config.oidBatchSize, gosnmp.MaxOids)
	}

	if config.communityString != "" {
		if config.snmpVersion == "1" {
			s.gosnmpInst.Version = gosnmp.Version1
		} else {
			s.gosnmpInst.Version = gosnmp.Version2c
		}
		s.gosnmpInst.Community = config.communityString
	} else if config.user != "" {
		authProtocol, err := getAuthProtocol(config.authProtocol)
		if err != nil {
			return err
		}

		privProtocol, err := getPrivProtocol(config.privProtocol)
		if err != nil {
			return err
		}

		msgFlags := gosnmp.NoAuthNoPriv
		if privProtocol != gosnmp.NoPriv {
			// Auth is needed if privacy is used.
			// "The User-based Security Model also prescribes that a message needs to be authenticated if privacy is in use."
			// https://tools.ietf.org/html/rfc3414#section-1.4.3
			msgFlags = gosnmp.AuthPriv
		} else if authProtocol != gosnmp.NoAuth {
			msgFlags = gosnmp.AuthNoPriv
		}

		s.gosnmpInst.Version = gosnmp.Version3
		s.gosnmpInst.MsgFlags = msgFlags
		s.gosnmpInst.ContextName = config.contextName
		s.gosnmpInst.SecurityModel = gosnmp.UserSecurityModel
		s.gosnmpInst.SecurityParameters = &gosnmp.UsmSecurityParameters{
			UserName:                 config.user,
			AuthenticationProtocol:   authProtocol,
			AuthenticationPassphrase: config.authKey,
			PrivacyProtocol:          privProtocol,
			PrivacyPassphrase:        config.privKey,
		}
	} else {
		return fmt.Errorf("an authentication method needs to be provided")
	}

	s.gosnmpInst.Target = config.ipAddress
	s.gosnmpInst.Port = config.port
	s.gosnmpInst.Timeout = time.Duration(config.timeout) * time.Second
	s.gosnmpInst.Retries = config.retries

	// Uncomment following line for debugging
	// s.gosnmpInst.Logger = log.New(os.Stdout, "", 0)
	return nil
}

func (s *snmpSession) Connect() error {
	return s.gosnmpInst.Connect()
}

func (s *snmpSession) Close() error {
	return s.gosnmpInst.Conn.Close()
}

func (s *snmpSession) Get(oids []string) (result *gosnmp.SnmpPacket, err error) {
	return s.gosnmpInst.Get(oids)
}

func (s *snmpSession) GetBulk(oids []string) (result *gosnmp.SnmpPacket, err error) {
	return s.gosnmpInst.GetBulk(oids, 0, bulkMaxRepetition)
}

func (s *snmpSession) GetNext(oids []string) (result *gosnmp.SnmpPacket, err error) {
	return s.gosnmpInst.GetNext(oids)
}

func (s *snmpSession) GetVersion() gosnmp.SnmpVersion {
	return s.gosnmpInst.Version
}

func fetchSysObjectID(session sessionAPI) (string, error) {
	result, err := session.Get([]string{sysObjectIDOid})
	if err != nil {
		return "", fmt.Errorf("cannot get sysobjectid: %s", err)
	}
	if len(result.Variables) != 1 {
		return "", fmt.Errorf("expected 1 value, but got %d: variables=%v", len(result.Variables), result.Variables)
	}
	pduVar := result.Variables[0]
	_, value, err := getValueFromPDU(pduVar)
	if err != nil {
		return "", fmt.Errorf("error getting value from pdu: %s", err)
	}
	strValue, err := value.toString()
	if err != nil {
		return "", fmt.Errorf("error converting value (%#v) to string : %v", value, err)
	}
	return strValue, err
}
