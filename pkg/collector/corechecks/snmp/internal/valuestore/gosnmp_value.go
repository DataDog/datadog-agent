// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package valuestore

import (
	"math"
	"strings"

	"github.com/gosnmp/gosnmp"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/networkdevice/profile/profiledefinition"
	"github.com/DataDog/datadog-agent/pkg/snmp/gosnmplib"
)

// GetResultValueFromPDU converts gosnmp.SnmpPDU to ResultValue
// See possible types here: https://github.com/gosnmp/gosnmp/blob/master/helper.go#L59-L271
//
// - gosnmp.Opaque: No support for gosnmp.Opaque since the type is processed recursively and never returned:
// is never returned https://github.com/gosnmp/gosnmp/blob/dc320dac5b53d95a366733fd95fb5851f2099387/helper.go#L195-L205
// - gosnmp.Boolean: seems not exist anymore and not handled by gosnmp
func GetResultValueFromPDU(pduVariable gosnmp.SnmpPDU) (string, ResultValue, error) {
	name := strings.TrimLeft(pduVariable.Name, ".") // remove leading dot
	value, err := gosnmplib.GetValueFromPDU(pduVariable)
	if err != nil {
		return name, ResultValue{}, err
	}
	submissionType := getSubmissionType(pduVariable.Type)
	return name, ResultValue{SubmissionType: submissionType, Value: value}, nil
}

// ResultToScalarValues converts result to scalar values
func ResultToScalarValues(result *gosnmp.SnmpPacket) ScalarResultValuesType {
	returnValues := make(map[string]ResultValue, len(result.Variables))
	for _, pduVariable := range result.Variables {
		if shouldSkip(pduVariable.Type) {
			continue
		}
		name, value, err := GetResultValueFromPDU(pduVariable)
		if err != nil {
			log.Debugf("cannot get value for variable `%v` with type `%v` and value `%v`", pduVariable.Name, pduVariable.Type, pduVariable.Value)
			continue
		}
		returnValues[name] = value
	}
	return returnValues
}

// ResultToColumnValues builds column values
// - ColumnResultValuesType: column values
// - nextOidsMap: represent the oids that can be used to retrieve following rows/values
func ResultToColumnValues(columnOids []string, snmpPacket *gosnmp.SnmpPacket) (ColumnResultValuesType, map[string]string) {
	returnValues := make(ColumnResultValuesType, len(columnOids))
	nextOidsMap := make(map[string]string, len(columnOids))
	maxRowsPerCol := int(math.Ceil(float64(len(snmpPacket.Variables)) / float64(len(columnOids))))
	for i, pduVariable := range snmpPacket.Variables {
		if shouldSkip(pduVariable.Type) {
			continue
		}

		oid, value, err := GetResultValueFromPDU(pduVariable)
		if err != nil {
			log.Debugf("Cannot get value for variable `%v` with type `%v` and value `%v`", pduVariable.Name, pduVariable.Type, pduVariable.Value)
			continue
		}
		// the snmpPacket might contain multiple row values for a single column
		// and the columnOid can be derived from the index of the PDU variable.
		columnOid := columnOids[i%len(columnOids)]
		if _, ok := returnValues[columnOid]; !ok {
			returnValues[columnOid] = make(map[string]ResultValue, maxRowsPerCol)
		}

		prefix := columnOid + "."
		if strings.HasPrefix(oid, prefix) {
			index := oid[len(prefix):]
			returnValues[columnOid][index] = value
			nextOidsMap[columnOid] = oid
		} else {
			// If oid is not prefixed by columnOid, it means it's not part of the column
			// and we can stop requesting the next row of this column. This is expected.
			delete(nextOidsMap, columnOid)
		}
	}
	return returnValues, nextOidsMap
}

func shouldSkip(berType gosnmp.Asn1BER) bool {
	switch berType {
	case gosnmp.EndOfContents, gosnmp.EndOfMibView, gosnmp.NoSuchInstance, gosnmp.NoSuchObject:
		return true
	}
	return false
}

// getSubmissionType converts gosnmp.Asn1BER type to submission type
//
// ZeroBasedCounter64: We don't handle ZeroBasedCounter64 since it's not a type currently provided by gosnmp.
// This type is currently supported by python impl: https://github.com/DataDog/integrations-core/blob/d6add1dfcd99c3610f45390b8d4cd97390af1f69/snmp/datadog_checks/snmp/pysnmp_inspect.py#L37-L38
func getSubmissionType(gosnmpType gosnmp.Asn1BER) profiledefinition.ProfileMetricType {
	switch gosnmpType {
	// Counter Types: From the snmp doc: The Counter32 type represents a non-negative integer which monotonically increases until it reaches a maximum
	// value of 2^32-1 (4294967295 decimal), when it wraps around and starts increasing again from zero.
	// We convert snmp counters by default to `rate` submission type, but sometimes `monotonic_count` might be more appropriate.
	// To achieve that, we can use `metric_type: monotonic_count` or `metric_type: monotonic_count_and_rate`.
	case gosnmp.Counter32, gosnmp.Counter64:
		return profiledefinition.ProfileMetricTypeCounter
	}
	return ""
}
