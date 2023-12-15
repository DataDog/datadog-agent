// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package containers

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/samber/lo"
)

func assertTags(actualTags []string, expectedTags []*regexp.Regexp) error {
	missingTags := make([]*regexp.Regexp, len(expectedTags))
	copy(missingTags, expectedTags)
	unexpectedTags := []string{}

	for _, actualTag := range actualTags {
		found := false
		for i, expectedTag := range missingTags {
			if expectedTag.MatchString(actualTag) {
				found = true
				missingTags[i] = missingTags[len(missingTags)-1]
				missingTags = missingTags[:len(missingTags)-1]
				break
			}
		}
		if !found {
			unexpectedTags = append(unexpectedTags, actualTag)
		}
	}

	if len(unexpectedTags) > 0 || len(missingTags) > 0 {
		errs := make([]error, 0, 2)
		if len(unexpectedTags) > 0 {
			errs = append(errs, fmt.Errorf("unexpected tags: %s", strings.Join(unexpectedTags, ", ")))
		}
		if len(missingTags) > 0 {
			errs = append(errs, fmt.Errorf("missing tags: %s", strings.Join(lo.Map(missingTags, func(re *regexp.Regexp, _ int) string { return re.String() }), ", ")))
		}
		return errors.Join(errs...)
	}

	return nil
}
