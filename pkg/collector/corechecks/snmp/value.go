package snmp

import (
	"fmt"
	"regexp"
	"strconv"
)

// ResultValue represent a snmp value
type ResultValue struct {
	SubmissionType string      // used when sending the metric
	ResultValue    interface{} // might be a `string` or `float64` type
}

func (sv *ResultValue) toFloat64() (float64, error) {
	switch sv.ResultValue.(type) {
	case float64:
		return sv.ResultValue.(float64), nil
	case string:
		val, err := strconv.ParseFloat(sv.ResultValue.(string), 64)
		if err != nil {
			return 0, fmt.Errorf("failed to parse `%s`: %s", sv.ResultValue, err.Error())
		}
		return val, nil
	}
	return 0, fmt.Errorf("invalid type %T for value %#v", sv.ResultValue, sv.ResultValue)
}

func (sv ResultValue) toString() (string, error) {
	switch sv.ResultValue.(type) {
	case float64:
		return strconv.Itoa(int(sv.ResultValue.(float64))), nil
	case string:
		return sv.ResultValue.(string), nil
	}
	return "", fmt.Errorf("invalid type %T for value %#v", sv.ResultValue, sv.ResultValue)
}

func (sv ResultValue) extractStringValue(extractValuePattern *regexp.Regexp) (ResultValue, error) {
	switch sv.ResultValue.(type) {
	case string:
		srcValue := sv.ResultValue.(string)
		matches := extractValuePattern.FindStringSubmatch(srcValue)
		if matches == nil {
			return ResultValue{}, fmt.Errorf("extract value extractValuePattern does not match (extractValuePattern=%v, srcValue=%v)", extractValuePattern, srcValue)
		}
		matchedValue := matches[1] // use first matching group
		return ResultValue{SubmissionType: sv.SubmissionType, ResultValue: matchedValue}, nil
	default:
		return sv, nil
	}
}
