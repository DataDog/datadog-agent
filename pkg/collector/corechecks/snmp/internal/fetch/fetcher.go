// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package fetch

import (
	"github.com/gosnmp/gosnmp"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/checkconfig"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/session"
	coresnmp "github.com/DataDog/datadog-agent/pkg/snmp"
)

type Fetcher struct {
	sessionFactory     session.Factory
	session            session.Session
	oidBatchSize       int
	bulkMaxRepetitions uint32
}

func NewFetcher(sessionFactory session.Factory, oidBatchSize int, bulkMaxRepetitions uint32) *Fetcher {
	return &Fetcher{
		sessionFactory:     sessionFactory,
		oidBatchSize:       oidBatchSize,
		bulkMaxRepetitions: bulkMaxRepetitions,
	}
}

func (f *Fetcher) CreateSession(config *checkconfig.CheckConfig) error {
	sess, err := f.sessionFactory(config)
	if err != nil {
		return err
	}

	f.session = sess
	return nil
}

func (f *Fetcher) Connect() error {
	return f.session.Connect()
}

func (f *Fetcher) Close() error {
	return f.session.Close()
}

func (f *Fetcher) GetNextValue() (*gosnmp.SnmpPacket, error) {
	return f.session.GetNext([]string{coresnmp.DeviceReachableGetNextOid})
}

func (f *Fetcher) GetSnmpRequestCounts() session.SnmpRequestCounts {
	return f.session.GetSnmpRequestCounts()
}
