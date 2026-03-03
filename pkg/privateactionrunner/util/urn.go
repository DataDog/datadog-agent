// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package util

import (
	"fmt"
	"strconv"
	"strings"
)

type RunnerURNParts struct {
	Region   string
	OrgID    int64
	RunnerID string
}

func ParseRunnerURN(urn string) (RunnerURNParts, error) {
	urnParts := strings.Split(urn, ":")
	if len(urnParts) != 7 {
		return RunnerURNParts{}, fmt.Errorf("invalid URN format: %s", urn)
	}
	orgId, err := strconv.ParseInt(urnParts[5], 10, 64)
	if err != nil {
		return RunnerURNParts{}, fmt.Errorf("invalid orgId in URN: %s", urnParts[5])
	}
	return RunnerURNParts{
		Region:   urnParts[4],
		OrgID:    orgId,
		RunnerID: urnParts[6],
	}, nil
}

func MakeRunnerURN(region string, orgID int64, runnerID string) string {
	return fmt.Sprintf("urn:dd:apps:on-prem-runner:%s:%d:%s", region, orgID, runnerID)
}
