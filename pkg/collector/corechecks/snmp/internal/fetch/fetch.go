// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(NDM) Fix revive linter
package fetch

import (
	"fmt"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/valuestore"
	coresnmp "github.com/DataDog/datadog-agent/pkg/snmp"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type columnFetchStrategy int

const (
	useGetBulk columnFetchStrategy = iota
	useGetNext
)

func (c columnFetchStrategy) String() string {
	switch c {
	case useGetBulk:
		return "useGetBulk"
	case useGetNext:
		return "useGetNext"
	default:
		return strconv.Itoa(int(c))
	}
}

// Fetch oid values from device
func (f *Fetcher) Fetch(scalarOIDs, columnOIDs []string) (*valuestore.ResultValueStore, error) {
	// fetch scalar values
	scalarResults, err := f.fetchScalarOidsWithBatching(scalarOIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch scalar oids with batching: %v", err)
	}

	columnResults, err := f.fetchColumnOidsWithBatching(columnOIDs, useGetBulk)
	if err != nil {
		log.Debugf("failed to fetch oids with GetBulk batching: %v", err)

		columnResults, err = f.fetchColumnOidsWithBatching(columnOIDs, useGetNext)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch oids with GetNext batching: %v", err)
		}
	}

	return &valuestore.ResultValueStore{ScalarValues: scalarResults, ColumnValues: columnResults}, nil
}

// FetchSysObjectID fetches the sys object id from the device
func (f *Fetcher) FetchSysObjectID() (string, error) {
	result, err := f.session.Get([]string{coresnmp.DeviceSysObjectIDOid})
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
	if oid != coresnmp.DeviceSysObjectIDOid {
		return "", fmt.Errorf("expect `%s` OID but got `%s` OID with value `%v`", coresnmp.DeviceSysObjectIDOid, oid, value)
	}
	strValue, err := value.ToString()
	if err != nil {
		return "", fmt.Errorf("error converting value (%#v) to string : %v", value, err)
	}
	return strValue, err
}
