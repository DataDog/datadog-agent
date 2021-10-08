// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

// StringSliceTransform returns a new slice `s` where s[i] = fct(values[i])
func StringSliceTransform(values []string, fct func(string) string) []string {
	s := make([]string, len(values))
	for i := 0; i < len(values); i++ {
		s[i] = fct(values[i])
	}
	return s
}
