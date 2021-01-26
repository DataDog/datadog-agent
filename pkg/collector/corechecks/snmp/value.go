package snmp

import (
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"strconv"
)

type snmpValueType struct {
	submissionType metrics.MetricType // used when sending the metric
	value          interface{}        // might be a `string` or `float64` type
}

func (sv *snmpValueType) toFloat64() float64 {
	var retValue float64

	switch sv.value.(type) {
	case float64:
		retValue = sv.value.(float64)
	case string:
		val, err := strconv.ParseInt(sv.value.(string), 10, 64)
		if err != nil {
			return float64(0)
		}
		retValue = float64(val)
	}
	// only float64/string are expected

	return retValue
}

func (sv snmpValueType) toString() string {
	var retValue string

	switch sv.value.(type) {
	case float64:
		retValue = strconv.Itoa(int(sv.value.(float64)))
	case string:
		retValue = sv.value.(string)
	}
	// only float64/string are expected

	return retValue
}
