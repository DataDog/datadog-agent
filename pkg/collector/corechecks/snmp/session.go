package snmp

import (
	"fmt"
	"github.com/gosnmp/gosnmp"
	"time"
)

const sysObjectIDOid = "1.3.6.1.2.1.1.2.0"

type sessionAPI interface {
	Configure(config snmpConfig) error
	Connect() error
	Close() error
	Get(oids []string) (result *gosnmp.SnmpPacket, err error)
	GetBulk(oids []string) (result *gosnmp.SnmpPacket, err error)
}

type snmpSession struct {
	gosnmpInst gosnmp.GoSNMP
}

func (s *snmpSession) Configure(config snmpConfig) error {
	if config.oidBatchSize > gosnmp.MaxOids {
		return fmt.Errorf("config oidBatchSize (%d) cannot be higher than gosnmp.MaxOids: %d", config.oidBatchSize, gosnmp.MaxOids)
	}
	snmpVersion, err := parseVersion(config.snmpVersion)
	if err != nil {
		return err
	}

	switch snmpVersion {
	case gosnmp.Version2c, gosnmp.Version1:
		s.gosnmpInst.Community = config.communityString
	case gosnmp.Version3:
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
	}

	s.gosnmpInst.Target = config.ipAddress
	s.gosnmpInst.Port = config.port
	s.gosnmpInst.Version = snmpVersion
	s.gosnmpInst.Timeout = time.Duration(config.timeout) * time.Second
	s.gosnmpInst.Retries = config.retries

	// Uncomment following line for debugging
	// s.gosnmpInst.Logger:  defaultLog.New(os.Stdout, "", 0),
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
	if len(oids) == 0 {
		return &gosnmp.SnmpPacket{}, nil
	}
	return s.gosnmpInst.GetBulk(oids, 0, 10)
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
	return value.toString(), err
}
