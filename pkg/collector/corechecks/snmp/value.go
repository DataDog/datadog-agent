package snmp

import (
	"fmt"
	"strconv"
)

type snmpValueType struct {
	submissionType string      // used when sending the metric
	value          interface{} // might be a `string` or `float64` type
}

func (sv *snmpValueType) toFloat64() (float64, error) {
	switch sv.value.(type) {
	case float64:
		return sv.value.(float64), nil
	case string:
		val, err := strconv.ParseInt(sv.value.(string), 10, 64)
		if err != nil {
			return 0, fmt.Errorf("failed to parse `%s`: %s", sv.value, err.Error())
		}
		return float64(val), nil
	}
	return 0, fmt.Errorf("invalid type %T for value %#v", sv.value, sv.value)
}

func (sv snmpValueType) toString() (string, error) {
	switch sv.value.(type) {
	case float64:
		return strconv.Itoa(int(sv.value.(float64))), nil
	case string:
		return sv.value.(string), nil
	}
	return "", fmt.Errorf("invalid type %T for value %#v", sv.value, sv.value)
}
