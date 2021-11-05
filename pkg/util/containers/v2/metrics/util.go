// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/retry"
)

func convertField(s *uint64, t **float64) {
	if s != nil {
		*t = util.Float64Ptr(float64(*s))
	}
}

func convertRetrierErr(err error) *retry.Error {
	if retry.IsErrPermaFail(err) {
		return ErrPermaFail
	}

	return ErrNothingYet
}
