// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package workloadmeta

import (
	"fmt"
	"strings"
)

func mapToString(m map[string]string) string {
	var sb strings.Builder
	for k, v := range m {
		fmt.Fprintf(&sb, "%s:%s ", k, v)
	}

	return sb.String()
}

func sliceToString(s []string) string {
	return strings.Join(s, " ")
}
