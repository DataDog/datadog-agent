// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

//nolint:revive // TODO(APM) Fix revive linter
package pb

import (
	"google.golang.org/protobuf/runtime/protoiface"
)

//nolint:revive // TODO(APM) Fix revive linter
func PbToStringSlice(s []protoiface.MessageV1) []string {
	slice := []string{}
	for _, s := range s {
		if s == nil {
			continue
		}
		slice = append(slice, s.String())
	}

	return slice
}
