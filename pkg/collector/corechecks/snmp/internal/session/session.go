// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package session

import (
	"fmt"
	stdlog "log"
	"strings"
	"time"

	"github.com/cihub/seelog"
	"github.com/gosnmp/gosnmp"

	"github.com/DataDog/datadog-agent/pkg/snmp/gosnmplib"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/checkconfig"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/valuestore"
)

const sysObjectIDOid = "1.3.6.1.2.1.1.2.0"

// Factory will create a new Session
type Factory func(config *checkconfig.CheckConfig) (Session, error)

// Session interface for connecting to a snmp device
type Session interface {
	Connect() error
	Close() error
	Get(oids []string) (result *gosnmp.SnmpPacket, err error)
	GetBulk(oids []string, bulkMaxRepetitions uint32) (result *gosnmp.SnmpPacket, err error)
	GetNext(oids []string) (result *gosnmp.SnmpPacket, err error)
	Walk(rootOid string, walkFn gosnmp.WalkFunc) error
	GetVersion() gosnmp.SnmpVersion
}

// GosnmpSession is used to connect to a snmp device
type GosnmpSession struct {
	gosnmpInst gosnmp.GoSNMP
}

// Connect is used to create a new connection
func (s *GosnmpSession) Connect() error {
	return s.gosnmpInst.Connect()
}

// Close is used to close the connection
func (s *GosnmpSession) Close() error {
	return s.gosnmpInst.Conn.Close()
}

// Get will send a SNMPGET command
func (s *GosnmpSession) Get(oids []string) (result *gosnmp.SnmpPacket, err error) {
	return s.gosnmpInst.Get(oids)
}

// GetBulk will send a SNMP BULKGET command
func (s *GosnmpSession) GetBulk(oids []string, bulkMaxRepetitions uint32) (result *gosnmp.SnmpPacket, err error) {
	return s.gosnmpInst.GetBulk(oids, 0, bulkMaxRepetitions)
}

// GetNext will send a SNMP GETNEXT command
func (s *GosnmpSession) GetNext(oids []string) (result *gosnmp.SnmpPacket, err error) {
	return s.gosnmpInst.GetNext(oids)
}

// Walk retrieves a subtree of values using GETNEXT.
func (s *GosnmpSession) Walk(rootOid string, walkFn gosnmp.WalkFunc) error {
	return s.gosnmpInst.Walk(rootOid, walkFn)
}

// GetVersion returns the snmp version used
func (s *GosnmpSession) GetVersion() gosnmp.SnmpVersion {
	return s.gosnmpInst.Version
}

// NewGosnmpSession creates a new session
func NewGosnmpSession(config *checkconfig.CheckConfig) (Session, error) {
	s := &GosnmpSession{}
	if config.OidBatchSize > gosnmp.MaxOids {
		return nil, fmt.Errorf("config oidBatchSize (%d) cannot be higher than gosnmp.MaxOids: %d", config.OidBatchSize, gosnmp.MaxOids)
	}

	if config.CommunityString != "" {
		if config.SnmpVersion == "1" {
			s.gosnmpInst.Version = gosnmp.Version1
		} else {
			s.gosnmpInst.Version = gosnmp.Version2c
		}
		s.gosnmpInst.Community = config.CommunityString
	} else if config.User != "" {
		authProtocol, err := gosnmplib.GetAuthProtocol(config.AuthProtocol)
		if err != nil {
			return nil, err
		}

		privProtocol, err := gosnmplib.GetPrivProtocol(config.PrivProtocol)
		if err != nil {
			return nil, err
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
		s.gosnmpInst.ContextName = config.ContextName
		s.gosnmpInst.SecurityModel = gosnmp.UserSecurityModel
		s.gosnmpInst.SecurityParameters = &gosnmp.UsmSecurityParameters{
			UserName:                 config.User,
			AuthenticationProtocol:   authProtocol,
			AuthenticationPassphrase: config.AuthKey,
			PrivacyProtocol:          privProtocol,
			PrivacyPassphrase:        config.PrivKey,
		}
	} else {
		return nil, fmt.Errorf("an authentication method needs to be provided")
	}

	s.gosnmpInst.Target = config.IPAddress
	s.gosnmpInst.Port = config.Port
	s.gosnmpInst.Timeout = time.Duration(config.Timeout) * time.Second
	s.gosnmpInst.Retries = config.Retries

	lvl, err := log.GetLogLevel()
	if err != nil {
		log.Warnf("failed to get logger: %s", err)
	} else {
		if lvl == seelog.TraceLvl {
			TraceLevelLogWriter := gosnmplib.TraceLevelLogWriter{}
			s.gosnmpInst.Logger = gosnmp.NewLogger(stdlog.New(&TraceLevelLogWriter, "", stdlog.Lshortfile))
		}
	}
	return s, nil
}

// FetchSysObjectID fetches the sys object id from the device
func FetchSysObjectID(session Session) (string, error) {
	result, err := session.Get([]string{sysObjectIDOid})
	if err != nil {
		return "", fmt.Errorf("cannot get sysobjectid: %s", err)
	}
	if len(result.Variables) != 1 {
		return "", fmt.Errorf("expected 1 value, but got %d: variables=%v", len(result.Variables), result.Variables)
	}
	pduVar := result.Variables[0]
	oid, value, err := valuestore.GetResultValueFromPDU(pduVar)
	if err != nil {
		return "", fmt.Errorf("error getting value from pdu: %s", err)
	}
	if oid != sysObjectIDOid {
		return "", fmt.Errorf("expect `%s` OID but got `%s` OID with value `%v`", sysObjectIDOid, oid, value)
	}
	strValue, err := value.ToString()
	if err != nil {
		return "", fmt.Errorf("error converting value (%#v) to string : %v", value, err)
	}
	return strValue, err
}

// FetchAllOidsUsingWalk fetches all available OIDs
func FetchAllOidsUsingWalk(session Session) ([]string, error) {
	var oids []string
	fn := func(dataUnit gosnmp.SnmpPDU) error {
		oids = append(oids, dataUnit.Name)
		return nil
	}
	err := session.Walk("1.3", fn)
	return oids, err
}

// FetchAllOidsUsingGetNext fetches all available OIDs
func FetchAllOidsUsingGetNext(session Session) ([]string, error) {
	var savedOids []string
	curReqOid := "1.3"
	for {
		log.Infof("GETNEXT %s", curReqOid)
		results, err := session.GetNext([]string{curReqOid})
		if err != nil {
			log.Errorf("GetNext error: %s", err)
			continue
		}
		variable := results.Variables[0]
		if variable.Type == gosnmp.EndOfContents || variable.Type == gosnmp.EndOfMibView {
			log.Info("No more OIDs")
			break
		}
		log.Infof("variable %+v", variable)
		oid := strings.TrimLeft(variable.Name, ".")
		if strings.HasSuffix(oid, ".0") {
			curReqOid = oid
		} else {
			nextColumn, err := GetNextColumnOid(oid)
			if err != nil {
				log.Errorf("Invalid column oid: %s", oid)
				break
			}
			curReqOid = nextColumn
		}
		savedOids = append(savedOids, oid)

	}
	return savedOids, nil
}
