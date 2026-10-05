// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !python && !test

package collector

import (
	"errors"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
)

type shadowCandidate struct{}

func shadowCandidatesByInstance(integration.Config) map[int]shadowCandidate {
	// Metric lookback shadow checks are intentionally excluded from non-Python Agent binaries.
	return nil
}

func (s *CheckScheduler) loadShadowCheck(shadowCandidate, check.Loader, checkid.ID) (check.Check, error) {
	return nil, errors.New("metric lookback shadow checks are disabled in non-Python Agent builds")
}
