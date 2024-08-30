// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package workloadmetadef

import (
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
)

func mapToString(m map[string]string) string {
	var sb strings.Builder
	for k, v := range m {
		fmt.Fprintf(&sb, "%s:%s ", k, v)
	}

	return sb.String()
}

func mapToScrubbedJSONString(m map[string]string) string {
	var sb strings.Builder
	for k, v := range m {
		scrubbed, err := scrubber.ScrubJSONString(v)
		if err == nil {
			fmt.Fprintf(&sb, "%s:%s ", k, scrubbed)
		} else {
			fmt.Fprintf(&sb, "%s:%s ", k, v)
		}
	}
	return sb.String()
}

func sliceToString(s []string) string {
	return strings.Join(s, " ")
}
