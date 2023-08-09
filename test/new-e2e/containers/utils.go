// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package containers

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"

	"github.com/samber/lo"
)

const float64EqualityThreshold = 1e-9

func almostEqual(a, b float64) bool {
	return math.Abs(a-b) <= float64EqualityThreshold
}

func assertTags(actualTags []string, expectedTags []*regexp.Regexp) error {
	unexpectedTags := []string{}

	for _, actualTag := range actualTags {
		found := false
		for i, expectedTag := range expectedTags {
			if expectedTag.MatchString(actualTag) {
				found = true
				expectedTags[i] = expectedTags[len(expectedTags)-1]
				expectedTags = expectedTags[:len(expectedTags)-1]
				break
			}
		}
		if !found {
			unexpectedTags = append(unexpectedTags, actualTag)
		}
	}

	if len(unexpectedTags) > 0 || len(expectedTags) > 0 {
		errs := make([]error, 0, 2)
		if len(unexpectedTags) > 0 {
			errs = append(errs, fmt.Errorf("unexpected tags: %s", strings.Join(unexpectedTags, ", ")))
		}
		if len(expectedTags) > 0 {
			errs = append(errs, fmt.Errorf("missing tags: %s", strings.Join(lo.Map(expectedTags, func(re *regexp.Regexp, _ int) string { return re.String() }), ", ")))
		}
		// replace with
		// errors.Join(errs...)
		// once migrated to go 1.20
		errMsgs := make([]string, 0, 2)
		for _, err := range errs {
			if err == nil {
				continue
			}
			errMsgs = append(errMsgs, err.Error())
		}
		return errors.New(strings.Join(errMsgs, "\n"))
	}

	return nil
}
