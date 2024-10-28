// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks && !windows

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

	// the BBSCache depends on the CCCache, so initialize the CCache first.  In
	// production code, any discrepancies would work out after a few polls;
	// this is just needed for tests.
	cc, _ = ConfigureGlobalCCCache(ctx, "url", "", "", false, time.Second, 1, false, true, true, true, &testCCClient{})
	<-cc.UpdatedOnce()
	bc, _ = ConfigureGlobalBBSCache(ctx, "url", "", "", "", time.Second, []*regexp.Regexp{}, []*regexp.Regexp{}, &testBBSClient{})
	<-bc.UpdatedOnce()

	code := m.Run()
	os.Exit(code)
}
