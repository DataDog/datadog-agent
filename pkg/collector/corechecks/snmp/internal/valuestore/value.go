// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package valuestore

import (
	"fmt"
	"regexp"
	"strconv"
)

// ResultValue represent a snmp value
type ResultValue struct {
	SubmissionType string      `json:"sub_type,omitempty"` // used when sending the metric
	Value          interface{} `json:"value"`              // might be a `string`, `[]byte` or `float64` type
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
	switch sv.Value.(type) {
	case float64:
		return strconv.Itoa(int(sv.Value.(float64))), nil
	case string:
		return sv.Value.(string), nil
	case []byte:
		bytesValue := sv.Value.([]byte)
		var strValue string
		if !isString(bytesValue) {
			// We hexify like Python/pysnmp impl (keep compatibility) if the value contains non ascii letters:
			// https://github.com/etingof/pyasn1/blob/db8f1a7930c6b5826357646746337dafc983f953/pyasn1/type/univ.py#L950-L953
			// hexifying like pysnmp prettyPrint might lead to unpredictable results since `[]byte` might or might not have
			// elements outside of 32-126 range. New lines, tabs and carriage returns are also stripped from the string.
			// An alternative solution is to explicitly force the conversion to specific type using profile config.
			strValue = fmt.Sprintf("%#x", bytesValue)
		} else {
			strValue = string(bytesValue)
		}
		return strValue, nil
	}
	return "", fmt.Errorf("invalid type %T for value %#v", sv.Value, sv.Value)
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
	switch value.(type) {
	case string:
		return value.(string)
	case []byte:
		return string(value.([]byte))
	}
	return ""
}
