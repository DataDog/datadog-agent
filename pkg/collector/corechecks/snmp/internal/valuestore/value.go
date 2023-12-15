// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package valuestore

import (
	"fmt"
	"regexp"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/networkdevice/profile/profiledefinition"

	"github.com/DataDog/datadog-agent/pkg/snmp/gosnmplib"
)

// ResultValue represent a snmp value
type ResultValue struct {
	SubmissionType profiledefinition.ProfileMetricType `json:"sub_type,omitempty"` // used when sending the metric
	Value          interface{}                         `json:"value"`              // might be a `string`, `[]byte` or `float64` type
}

// ToFloat64 converts value to float64
func (sv *ResultValue) ToFloat64() (float64, error) {
	switch sv.Value.(type) {
	case float64:
		return sv.Value.(float64), nil
	case string, []byte:
		strValue := bytesOrStringToString(sv.Value)
		val, err := strconv.ParseFloat(strValue, 64)
		if err != nil {
			return 0, fmt.Errorf("failed to parse `%s`: %s", sv.Value, err.Error())
		}
		return val, nil
	}
	return 0, fmt.Errorf("invalid type %T for value %#v", sv.Value, sv.Value)
}

// ToString converts value to string
func (sv ResultValue) ToString() (string, error) {
	return gosnmplib.StandardTypeToString(sv.Value)
}

// ExtractStringValue extract value using a regex
func (sv ResultValue) ExtractStringValue(extractValuePattern *regexp.Regexp) (ResultValue, error) {
	switch sv.Value.(type) {
	case string, []byte:
		srcValue := bytesOrStringToString(sv.Value)
		matches := extractValuePattern.FindStringSubmatch(srcValue)
		if matches == nil {
			return ResultValue{}, fmt.Errorf("extract value extractValuePattern does not match (extractValuePattern=%v, srcValue=%v)", extractValuePattern, srcValue)
		}
		if len(matches) < 2 {
			return ResultValue{}, fmt.Errorf("extract value pattern des not contain any matching group (extractValuePattern=%v, srcValue=%v)", extractValuePattern, srcValue)
		}
		matchedValue := matches[1] // use first matching group
		return ResultValue{SubmissionType: sv.SubmissionType, Value: matchedValue}, nil
	default:
		return sv, nil
	}
}

func bytesOrStringToString(value interface{}) string {
	switch value := value.(type) {
	case string:
		return value
	case []byte:
		return string(value)
	}
	return ""
}
