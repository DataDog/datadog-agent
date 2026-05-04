// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

package listeners

import (
	"github.com/gosnmp/gosnmp"

	"github.com/DataDog/datadog-agent/pkg/snmp"
)

// snmpSessionFactory creates SNMP sessions from authentication params.
type snmpSessionFactory func(auth snmp.Authentication, deviceIP string, port uint16) (snmpSession, error)

// snmpSession abstracts SNMP operations needed by the listener for device discovery.
// Note: A similar, more complete interface exists at pkg/collector/corechecks/snmp/internal/session.Session
type snmpSession interface {
	Connect() error
	Close() error
	Get(oids []string) (result *gosnmp.SnmpPacket, err error)
	GetNext(oids []string) (result *gosnmp.SnmpPacket, err error)
}

// gosnmpSession wraps *gosnmp.GoSNMP to implement snmpSession.
type gosnmpSession struct {
	gosnmpInst *gosnmp.GoSNMP
}

func (s *gosnmpSession) Connect() error {
	return s.gosnmpInst.Connect()
}

func (s *gosnmpSession) Close() error {
	if s.gosnmpInst.Conn == nil {
		return nil
	}
	return s.gosnmpInst.Conn.Close()
}

func (s *gosnmpSession) Get(oids []string) (*gosnmp.SnmpPacket, error) {
	return s.gosnmpInst.Get(oids)
}

func (s *gosnmpSession) GetNext(oids []string) (*gosnmp.SnmpPacket, error) {
	return s.gosnmpInst.GetNext(oids)
}

// newGosnmpSession is the production session factory.
func newGosnmpSession(auth snmp.Authentication, deviceIP string, port uint16) (snmpSession, error) {
	params, err := auth.BuildSNMPParams(deviceIP, port)
	if err != nil {
		return nil, err
	}
	return &gosnmpSession{gosnmpInst: params}, nil
}
