// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package containers

import (
	"time"
)

func (suite *BaseSuite[Env]) BeforeTest(suiteName, testName string) {
	suite.T().Logf("START  %s/%s %s", suiteName, testName, time.Now())
	suite.BaseSuite.BeforeTest(suiteName, testName)
}

func (suite *BaseSuite[Env]) AfterTest(suiteName, testName string) {
	suite.T().Logf("FINISH %s/%s %s", suiteName, testName, time.Now())
	suite.BaseSuite.AfterTest(suiteName, testName)
}
