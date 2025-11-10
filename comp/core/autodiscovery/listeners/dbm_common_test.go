// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

//go:build ec2

package listeners

import (
	"context"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/databasemonitoring/aws"
	"github.com/golang/mock/gomock"
)

type mockRdsClientConfigurer func(k *aws.MockRdsClient)

const defaultADTag = "datadoghq.com/scrape:true"
const defaultDbmTag = "datadoghq.com/dbm:true"

func contextWithTimeout(t time.Duration) gomock.Matcher {
	return contextWithTimeoutMatcher{
		timeout: t,
	}
}

type contextWithTimeoutMatcher struct {
	timeout time.Duration
}

func (m contextWithTimeoutMatcher) Matches(x interface{}) bool {
	ctx, ok := x.(context.Context)
	if !ok {
		return false
	}
	deadline, ok := ctx.Deadline()
	if !ok {
		return false
	}

	delta := time.Until(deadline) - m.timeout
	return delta < time.Millisecond*50
}

func (m contextWithTimeoutMatcher) String() string {
	return fmt.Sprintf("have a deadline from a timeout of %d milliseconds", m.timeout.Milliseconds())
}
