package snmp

import (
	"fmt"
	"math"
	"strings"

	"github.com/gosnmp/gosnmp"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// getValueFromPDU converts gosnmp.SnmpPDU to snmpValueType
// See possible types here: https://github.com/gosnmp/gosnmp/blob/master/helper.go#L59-L271
//
// - gosnmp.Opaque: No support for gosnmp.Opaque since the type is processed recursively and never returned:
//   is never returned https://github.com/gosnmp/gosnmp/blob/dc320dac5b53d95a366733fd95fb5851f2099387/helper.go#L195-L205
// - gosnmp.Boolean: seems not exist anymore and not handled by gosnmp
func getValueFromPDU(pduVariable gosnmp.SnmpPDU) (string, snmpValueType, error) {
	var value interface{}
	name := strings.TrimLeft(pduVariable.Name, ".") // remove leading dot
	switch pduVariable.Type {
	case gosnmp.OctetString, gosnmp.BitString:
		bytesValue, ok := pduVariable.Value.([]byte)
		if !ok {
			return name, snmpValueType{}, fmt.Errorf("oid %s: OctetString/BitString should be []byte type but got %T type: %#v", pduVariable.Name, pduVariable.Value, pduVariable)
		}
		if hasNonPrintableByte(bytesValue) {
			// We hexify like Python/pysnmp impl (keep compatibility) if the value contains non ascii letters:
			// https://github.com/etingof/pyasn1/blob/db8f1a7930c6b5826357646746337dafc983f953/pyasn1/type/univ.py#L950-L953
			// hexifying like pysnmp prettyPrint might lead to unpredictable results since `[]byte` might or might not have
			// elements outside of 32-126 range
			// An alternative solution is to explicitly force the conversion to specific type using profile config.
			value = fmt.Sprintf("%#x", bytesValue)
		} else {
			value = string(bytesValue)
		}
	case gosnmp.Integer, gosnmp.Counter32, gosnmp.Gauge32, gosnmp.TimeTicks, gosnmp.Counter64, gosnmp.Uinteger32:
		value = float64(gosnmp.ToBigInt(pduVariable.Value).Int64())
	case gosnmp.OpaqueFloat:
		floatValue, ok := pduVariable.Value.(float32)
		if !ok {
			return name, snmpValueType{}, fmt.Errorf("oid %s: OpaqueFloat should be float32 type but got %T type: %#v", pduVariable.Name, pduVariable.Value, pduVariable)
		}
		value = float64(floatValue)
	case gosnmp.OpaqueDouble:
		floatValue, ok := pduVariable.Value.(float64)
		if !ok {
			return name, snmpValueType{}, fmt.Errorf("oid %s: OpaqueDouble should be float64 type but got %T type: %#v", pduVariable.Name, pduVariable.Value, pduVariable)
		}
		value = floatValue
	case gosnmp.IPAddress:
		strValue, ok := pduVariable.Value.(string)
		if !ok {
			return name, snmpValueType{}, fmt.Errorf("oid %s: IPAddress should be string type but got %T type: %#v", pduVariable.Name, pduVariable.Value, pduVariable)
		}
		value = strValue
	case gosnmp.ObjectIdentifier:
		strValue, ok := pduVariable.Value.(string)
		if !ok {
			return name, snmpValueType{}, fmt.Errorf("oid %s: ObjectIdentifier should be string type but got %T type: %#v", pduVariable.Name, pduVariable.Value, pduVariable)
		}
		value = strings.TrimLeft(strValue, ".")
	default:
		return name, snmpValueType{}, fmt.Errorf("oid %s: invalid type: %s", pduVariable.Name, pduVariable.Type.String())
	}
	submissionType := getSubmissionType(pduVariable.Type)
	return name, snmpValueType{submissionType: submissionType, value: value}, nil
}

func hasNonPrintableByte(bytesValue []byte) bool {
	hasNonPrintable := false
	for _, bit := range bytesValue {
		if bit < 32 || bit > 126 {
			hasNonPrintable = true
		}
	}
	return hasNonPrintable
}

func resultToScalarValues(result *gosnmp.SnmpPacket) scalarResultValuesType {
	returnValues := make(map[string]snmpValueType, len(result.Variables))
	for _, pduVariable := range result.Variables {
		if shouldSkip(pduVariable.Type) {
			continue
		}
		name, value, err := getValueFromPDU(pduVariable)
		if err != nil {
			log.Debugf("cannot get value for variable `%v` with type `%v` and value `%v`", pduVariable.Name, pduVariable.Type, pduVariable.Value)
			continue
		}
		returnValues[name] = value
	}
	return returnValues
}

// resultToColumnValues builds column values
// - columnResultValuesType: column values
// - nextOidsMap: represent the oids that can be used to retrieve following rows/values
func resultToColumnValues(columnOids []string, snmpPacket *gosnmp.SnmpPacket) (columnResultValuesType, map[string]string) {
	returnValues := make(columnResultValuesType, len(columnOids))
	nextOidsMap := make(map[string]string, len(columnOids))
	maxRowsPerCol := int(math.Ceil(float64(len(snmpPacket.Variables)) / float64(len(columnOids))))
	for i, pduVariable := range snmpPacket.Variables {
		if shouldSkip(pduVariable.Type) {
			continue
		}

		oid, value, err := getValueFromPDU(pduVariable)
		if err != nil {
			log.Debugf("Cannot get value for variable `%v` with type `%v` and value `%v`", pduVariable.Name, pduVariable.Type, pduVariable.Value)
			continue
		}
		// the snmpPacket might contain multiple row values for a single column
		// and the columnOid can be derived from the index of the PDU variable.
		columnOid := columnOids[i%len(columnOids)]
		if _, ok := returnValues[columnOid]; !ok {
			returnValues[columnOid] = make(map[string]snmpValueType, maxRowsPerCol)
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
func getSubmissionType(gosnmpType gosnmp.Asn1BER) string {
	switch gosnmpType {
	// Counter Types: From the snmp doc: The Counter32 type represents a non-negative integer which monotonically increases until it reaches a maximum
	// value of 2^32-1 (4294967295 decimal), when it wraps around and starts increasing again from zero.
	// We convert snmp counters by default to `rate` submission type, but sometimes `monotonic_count` might be more appropriate.
	// To achieve that, we can use `forced_type: monotonic_count` or `forced_type: monotonic_count_and_rate`.
	case gosnmp.Counter32, gosnmp.Counter64:
		return "counter"
	}
	return ""
}
