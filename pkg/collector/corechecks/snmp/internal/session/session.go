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

	"go.uber.org/atomic"

	"github.com/gosnmp/gosnmp"

	"github.com/DataDog/datadog-agent/pkg/snmp/gosnmplib"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/checkconfig"
)

// Factory will create a new Session
type Factory func(config *checkconfig.CheckConfig) (Session, error)

// Session interface for connecting to a snmp device
type Session interface {
	Connect() error
	Close() error
	Get(oids []string) (result *gosnmp.SnmpPacket, err error)
	GetBulk(oids []string, bulkMaxRepetitions uint32) (result *gosnmp.SnmpPacket, err error)
	GetNext(oids []string) (result *gosnmp.SnmpPacket, err error)
	GetSnmpRequestCounts() SnmpRequestCounts
	GetVersion() gosnmp.SnmpVersion
}

type SnmpRequestCounts struct {
	GetCount     uint32
	GetBulkCount uint32
	GetNextCount uint32
}

// GosnmpSession is used to connect to a snmp device
type GosnmpSession struct {
	gosnmpInst       gosnmp.GoSNMP
	snmpGetCount     *atomic.Uint32
	snmpGetBulkCount *atomic.Uint32
	snmpGetNextCount *atomic.Uint32
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
	s.snmpGetCount.Inc()
	return s.gosnmpInst.Get(oids)
}

// GetBulk will send a SNMP BULKGET command
func (s *GosnmpSession) GetBulk(oids []string, bulkMaxRepetitions uint32) (result *gosnmp.SnmpPacket, err error) {
	s.snmpGetBulkCount.Inc()
	return s.gosnmpInst.GetBulk(oids, 0, bulkMaxRepetitions)
}

// GetNext will send a SNMP GETNEXT command
func (s *GosnmpSession) GetNext(oids []string) (result *gosnmp.SnmpPacket, err error) {
	s.snmpGetNextCount.Inc()
	return s.gosnmpInst.GetNext(oids)
}

// GetSnmpRequestCounts returns the number SNMP requests that has been done for each request type
func (s *GosnmpSession) GetSnmpRequestCounts() SnmpRequestCounts {
	return SnmpRequestCounts{
		GetCount:     s.snmpGetCount.Load(),
		GetBulkCount: s.snmpGetBulkCount.Load(),
		GetNextCount: s.snmpGetNextCount.Load(),
	}
}

// GetVersion returns the snmp version used
func (s *GosnmpSession) GetVersion() gosnmp.SnmpVersion {
	return s.gosnmpInst.Version
}

// NewGosnmpSession creates a new session
func NewGosnmpSession(config *checkconfig.CheckConfig) (Session, error) {
	s := &GosnmpSession{
		snmpGetCount:     atomic.NewUint32(0),
		snmpGetBulkCount: atomic.NewUint32(0),
		snmpGetNextCount: atomic.NewUint32(0),
	}
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
		if config.AuthKey != "" && config.AuthProtocol == "" {
			config.AuthProtocol = "md5"
		}
		if config.PrivKey != "" && config.PrivProtocol == "" {
			config.PrivProtocol = "des"
		}

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
		if lvl == log.TraceLvl {
			TraceLevelLogWriter := gosnmplib.TraceLevelLogWriter{}
			s.gosnmpInst.Logger = gosnmp.NewLogger(stdlog.New(&TraceLevelLogWriter, "", stdlog.Lshortfile))
		}
	}
	return s, nil
}

// FetchAllOIDsUsingGetNext fetches all available OIDs
// Fetch all scalar OIDs and first row of table OIDs.
func FetchAllOIDsUsingGetNext(session Session) []string {
	var savedOIDs []string
	curRequestOid := "1.0"
	alreadySeenOIDs := make(map[string]bool)

	for {
		results, err := session.GetNext([]string{curRequestOid})
		if err != nil {
			log.Debugf("GetNext error: %s", err)
			break
		}
		if len(results.Variables) != 1 {
			log.Debugf("Expect 1 variable, but got %d: %+v", len(results.Variables), results.Variables)
			break
		}
		variable := results.Variables[0]
		if variable.Type == gosnmp.EndOfContents || variable.Type == gosnmp.EndOfMibView {
			log.Debug("No more OIDs to fetch")
			break
		}
		oid := strings.TrimLeft(variable.Name, ".")
		if strings.HasSuffix(oid, ".0") { // check if it's a scalar OID
			curRequestOid = oid
		} else {
			nextColumn, err := GetNextColumnOidNaive(oid)
			if err != nil {
				log.Debugf("Invalid column oid: %s", oid)
				curRequestOid = oid // fallback on continuing by using the response oid as next oid to request
			} else {
				curRequestOid = nextColumn
			}
		}

		if alreadySeenOIDs[curRequestOid] {
			// breaking on already seen OIDs prevent infinite loop if the device mis behave by responding with non-sequential OIDs when called with GETNEXT
			log.Debug("error: received non sequential OIDs")
			break
		}
		alreadySeenOIDs[curRequestOid] = true

		savedOIDs = append(savedOIDs, oid)
	}
	return savedOIDs
}
