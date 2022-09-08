// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package provider

import (
	"github.com/DataDog/datadog-agent/pkg/util/retry"
)

// ConvertRetrierErr converts a retrier error into a metrics error
func ConvertRetrierErr(err error) error {
	if retry.IsErrPermaFail(err) {
		return ErrPermaFail
	}

	if retry.IsErrWillRetry(err) {
		return ErrNothingYet
	}

	return err
}
