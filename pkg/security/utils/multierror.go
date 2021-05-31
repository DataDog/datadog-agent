// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import "github.com/hashicorp/go-multierror"

/// FilterMultiError creates a new *multierror.Error filtering an existing one with a list of error
func FilterMultiError(multi *multierror.Error, filter []error) *multierror.Error {
	var res *multierror.Error

	filterMsgs := make([]string, 0, len(filter))
	for _, ferr := range filter {
		if ferr != nil {
			filterMsgs = append(filterMsgs, ferr.Error())
		}
	}

	for _, current := range multi.Errors {
		if current == nil {
			continue
		}

		isFiltered := false
		for _, errMsg := range filterMsgs {
			if current.Error() == errMsg {
				isFiltered = true
			}
		}

		if !isFiltered {
			res = multierror.Append(res, current)
		}
	}

	return res
}
