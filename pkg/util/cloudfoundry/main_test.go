// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build clusterchecks

package cloudfoundry

import (
	"context"
	"os"
	"regexp"
	"testing"
	"time"
)

type testBBSClient struct {
}

type testCCClient struct {
}

var (
	bc *BBSCache
	cc *CCCache
)

func TestMain(m *testing.M) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	bc, _ = ConfigureGlobalBBSCache(ctx, "url", "", "", "", time.Second, []*regexp.Regexp{}, []*regexp.Regexp{}, &testBBSClient{})
	cc, _ = ConfigureGlobalCCCache(ctx, "url", "", "", false, time.Second, &testCCClient{})
	for i := 1; i <= 10; i++ {
		if cc.GetPollSuccesses() == 0 || bc.GetPollSuccesses() == 0 {
			time.Sleep(time.Second)
		}
	}
	code := m.Run()
	os.Exit(code)
}
