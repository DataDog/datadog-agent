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
	cc, _ = ConfigureGlobalCCCache(ctx, CCCacheConfig{
		CCClient:        &testCCClient{},
		PollInterval:    time.Second,
		AppsBatchSize:   1,
		ServeNozzleData: true,
		SidecarsTags:    true,
		SegmentsTags:    true,
	})
	<-cc.UpdatedOnce()
	bc, _ = ConfigureGlobalBBSCache(ctx, BBSCacheConfig{
		BBSClient:    &testBBSClient{},
		PollInterval: time.Second,
		IncludeList:  []*regexp.Regexp{},
		ExcludeList:  []*regexp.Regexp{},
		CCCache:      cc, // Inject CCCache dependency
	})
	<-bc.UpdatedOnce()

	code := m.Run()
	os.Exit(code)
}
