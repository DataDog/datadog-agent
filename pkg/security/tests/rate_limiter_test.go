// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build functionaltests
// +build functionaltests

package tests

import (
	"fmt"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/stretchr/testify/assert"
	"golang.org/x/time/rate"
)

func TestRateLimiter(t *testing.T) {
	emptyLimits := make(map[string]map[string]utils.Limit)
	starterLimits := map[string]map[string]utils.Limit{
		"group1": {
			"id1": {Limit: 1, Burst: 1},
			"id2": {Limit: 2, Burst: 2},
		},
		"group2": {
			"id3": {Limit: 3, Burst: 3},
			"id4": {Limit: 4, Burst: 4},
		},
	}
	defaultLimit, defaultBurst := utils.GetDefaultLimitBurst()

	t.Run("rate-limiter-new-rate-limiter", func(t *testing.T) {
		rl := utils.NewRateLimiter(nil, utils.LimiterOpts{Limits: emptyLimits})
		if rl == nil {
			t.Fatal()
		}
		assert.Equal(t, len(rl.GetGroups()), 0)
		opts := rl.GetLimiterOpts()
		assert.Equal(t, len(opts.Limits), 0)

		rl = utils.NewRateLimiter(nil, utils.LimiterOpts{Limits: starterLimits})
		if rl == nil {
			t.Fatal()
		}
		assert.Equal(t, len(rl.GetGroups()), 0)
		opts = rl.GetLimiterOpts()
		assert.Equal(t, len(opts.Limits), 2)
	})

	t.Run("rate-limiter-new-limiter", func(t *testing.T) {
		l := utils.NewLimiter(rate.Limit(0), 0)
		if l == nil {
			t.Fatal()
		}

		l = utils.NewLimiter(rate.Limit(1), 1)
		if l == nil {
			t.Fatal()
		}

		l = utils.NewLimiter(rate.Inf, 0)
		if l == nil {
			t.Fatal()
		}

		l = utils.NewLimiter(rate.Every(time.Second), 1)
		if l == nil {
			t.Fatal()
		}
	})

	t.Run("rate-limiter-set-group-limiters-without-opts", func(t *testing.T) {
		rl := utils.NewRateLimiter(nil, utils.LimiterOpts{Limits: emptyLimits})

		assert.Equal(t, len(rl.GetGroups()), 0)

		rl.SetGroupLimiters("group1", []string{})
		assert.Equal(t, len(rl.GetGroups()), 1)
		assert.Equal(t, len(rl.GetGroupIDs("group1")), 0)

		rl.SetGroupLimiters("group1", []string{"id1", "id2"})
		assert.Equal(t, len(rl.GetGroups()), 1)
		assert.Equal(t, len(rl.GetGroupIDs("group1")), 2)

		rl.SetGroupLimiters("group1", []string{"id1"})
		assert.Equal(t, len(rl.GetGroups()), 1)
		assert.Equal(t, len(rl.GetGroupIDs("group1")), 1)

		rl.SetGroupLimiters("group2", []string{"id3", "id4"})
		assert.Equal(t, len(rl.GetGroups()), 2)
		assert.Equal(t, len(rl.GetGroupIDs("group1")), 1)
		assert.Equal(t, len(rl.GetGroupIDs("group2")), 2)

		rl.SetGroupLimiters("group2", []string{})
		assert.Equal(t, len(rl.GetGroups()), 2)
		assert.Equal(t, len(rl.GetGroupIDs("group1")), 1)
		assert.Equal(t, len(rl.GetGroupIDs("group2")), 0)

		limit, burst, err := rl.GetLimit("group1", "id1")
		if err != nil {
			t.Error()
		}
		assert.Equal(t, limit, defaultLimit)
		assert.Equal(t, burst, defaultBurst)
	})

	t.Run("rate-limiter-set-group-limiters-with-opts", func(t *testing.T) {
		rl := utils.NewRateLimiter(nil, utils.LimiterOpts{Limits: starterLimits})

		assert.Equal(t, len(rl.GetGroups()), 0)

		rl.SetGroupLimiters("group1", []string{"id1", "id2"})
		assert.Equal(t, len(rl.GetGroups()), 1)
		assert.Equal(t, len(rl.GetGroupIDs("group1")), 2)

		rl.SetGroupLimiters("group2", []string{"id3", "id4"})
		assert.Equal(t, len(rl.GetGroups()), 2)
		assert.Equal(t, len(rl.GetGroupIDs("group1")), 2)
		assert.Equal(t, len(rl.GetGroupIDs("group2")), 2)

		limit, burst, _ := rl.GetLimit("group1", "id1")
		assert.Equal(t, limit, rate.Limit(1))
		assert.Equal(t, burst, 1)
		limit, burst, _ = rl.GetLimit("group1", "id2")
		assert.Equal(t, limit, rate.Limit(2))
		assert.Equal(t, burst, 2)
		limit, burst, _ = rl.GetLimit("group2", "id3")
		assert.Equal(t, limit, rate.Limit(3))
		assert.Equal(t, burst, 3)
		limit, burst, _ = rl.GetLimit("group2", "id4")
		assert.Equal(t, limit, rate.Limit(4))
		assert.Equal(t, burst, 4)

		rl.SetGroupLimiters("foo", []string{"bar"})
		assert.Equal(t, len(rl.GetGroups()), 3)
		assert.Equal(t, len(rl.GetGroupIDs("foo")), 1)
		limit, burst, _ = rl.GetLimit("foo", "bar")
		assert.Equal(t, limit, defaultLimit)
		assert.Equal(t, burst, defaultBurst)
	})

	t.Run("rate-limiter-add-new-limiter", func(t *testing.T) {
		rl := utils.NewRateLimiter(nil, utils.LimiterOpts{Limits: emptyLimits})

		if err := rl.AddNewLimiter("foo", "bar", rate.Limit(42), 51); err != nil {
			t.Fatal()
		}
		assert.Equal(t, len(rl.GetGroups()), 1)
		assert.Equal(t, len(rl.GetGroupIDs("foo")), 1)
		limit, burst, _ := rl.GetLimit("foo", "bar")
		assert.Equal(t, limit, rate.Limit(42))
		assert.Equal(t, burst, 51)

		if err := rl.AddNewLimiter("foo", "bar2", rate.Limit(42), 51); err != nil {
			t.Fatal()
		}
		assert.Equal(t, len(rl.GetGroups()), 1)
		assert.Equal(t, len(rl.GetGroupIDs("foo")), 2)

		if err := rl.AddNewLimiter("foo2", "bar2", rate.Limit(42), 51); err != nil {
			t.Fatal()
		}
		assert.Equal(t, len(rl.GetGroups()), 2)
		assert.Equal(t, len(rl.GetGroupIDs("foo")), 2)
		assert.Equal(t, len(rl.GetGroupIDs("foo2")), 1)

		err := rl.AddNewLimiter("foo", "bar", rate.Limit(1), 2)
		if err == nil {
			t.Fatal()
		}
		if err.Error() != "EEXIST" {
			fmt.Printf("err: %+v\n", err)
			t.Fatal()
		}
		assert.Equal(t, len(rl.GetGroups()), 2)
		assert.Equal(t, len(rl.GetGroupIDs("foo")), 2)

		err = rl.AddNewLimiter("foo", "baz", rate.Limit(1), -2)
		if err == nil {
			t.Fatal()
		}
		if err.Error() != "EINVAL" {
			fmt.Printf("err: %+v\n", err)
			t.Fatal()
		}
		assert.Equal(t, len(rl.GetGroups()), 2)
		assert.Equal(t, len(rl.GetGroupIDs("foo")), 2)
	})

	t.Run("rate-limiter-remove-limiter", func(t *testing.T) {
		rl := utils.NewRateLimiter(nil, utils.LimiterOpts{Limits: emptyLimits})

		rl.AddNewLimiter("foo", "bar", rate.Limit(42), 51)
		assert.Equal(t, len(rl.GetGroups()), 1)
		assert.Equal(t, len(rl.GetGroupIDs("foo")), 1)

		rl.RemoveLimiter("foo", "bar")
		assert.Equal(t, len(rl.GetGroups()), 1)
		assert.Equal(t, len(rl.GetGroupIDs("foo")), 0)

		err := rl.RemoveLimiter("foo", "bar")
		if err.Error() != "ENOENT" {
			t.Fatal()
		}
	})

	t.Run("rate-limiter-allow-simple", func(t *testing.T) {
		rl := utils.NewRateLimiter(nil, utils.LimiterOpts{Limits: emptyLimits})

		rl.AddNewLimiter("foo", "allow", rate.Limit(50), 50)
		if !rl.Allow("foo", "allow") {
			t.Fatal()
		}
		stats, _ := rl.GetLimiterStats("foo", "allow", true)
		assert.Equal(t, stats.Allowed, int64(1))
		assert.Equal(t, stats.Dropped, int64(0))

		rl.AddNewLimiter("foo", "inf", rate.Inf, 0)
		if !rl.Allow("foo", "inf") {
			t.Fatal()
		}
		stats, _ = rl.GetLimiterStats("foo", "inf", true)
		assert.Equal(t, stats.Allowed, int64(1))
		assert.Equal(t, stats.Dropped, int64(0))

		rl.AddNewLimiter("foo", "deny", rate.Limit(50), 0)
		if rl.Allow("foo", "deny") {
			t.Fatal()
		}
		stats, _ = rl.GetLimiterStats("foo", "deny", true)
		assert.Equal(t, stats.Allowed, int64(0))
		assert.Equal(t, stats.Dropped, int64(1))
	})

	t.Run("rate-limiter-allow-single-volley", func(t *testing.T) {
		rl := utils.NewRateLimiter(nil, utils.LimiterOpts{Limits: emptyLimits})

		rl.AddNewLimiter("foo", "allow", rate.Limit(50), 50)
		for i := 0; i < 50; i++ {
			rl.Allow("foo", "allow")
		}
		stats, _ := rl.GetLimiterStats("foo", "allow", true)
		assert.Equal(t, stats.Allowed, int64(50))
		assert.Equal(t, stats.Dropped, int64(0))

		rl.AddNewLimiter("foo", "mix", rate.Limit(50), 50)
		for i := 0; i < 100; i++ {
			rl.Allow("foo", "mix")
		}
		stats, _ = rl.GetLimiterStats("foo", "mix", true)
		assert.Equal(t, stats.Allowed, int64(50))
		assert.Equal(t, stats.Dropped, int64(50))

		rl.AddNewLimiter("foo", "burst", rate.Limit(50), 100)
		for i := 0; i < 150; i++ {
			rl.Allow("foo", "burst")
		}
		stats, _ = rl.GetLimiterStats("foo", "burst", true)
		assert.Equal(t, stats.Allowed, int64(100))
		assert.Equal(t, stats.Dropped, int64(50))

		rl.AddNewLimiter("foo", "inf", rate.Inf, 0)
		for i := 0; i < 5000; i++ {
			rl.Allow("foo", "inf")
		}
		stats, _ = rl.GetLimiterStats("foo", "inf", true)
		assert.Equal(t, stats.Allowed, int64(5000))
		assert.Equal(t, stats.Dropped, int64(0))
	})

	t.Run("rate-limiter-allow-at-rate", func(t *testing.T) {
		rl := utils.NewRateLimiter(nil, utils.LimiterOpts{Limits: emptyLimits})
		if rl == nil {
			t.Fatal()
		}

		rl.AddNewLimiter("foo", "allow", rate.Limit(50), 50)
		for i := 0; i < 50; i++ {
			rl.Allow("foo", "allow")
		}
		time.Sleep(time.Second * 1)
		for i := 0; i < 50; i++ {
			rl.Allow("foo", "allow")
		}
		stats, _ := rl.GetLimiterStats("foo", "allow", true)
		assert.Equal(t, stats.Allowed, int64(100))
		assert.Equal(t, stats.Dropped, int64(0))

		rl.AddNewLimiter("foo", "mix", rate.Limit(50), 50)
		for i := 0; i < 100; i++ {
			rl.Allow("foo", "mix")
		}
		time.Sleep(time.Second * 1)
		for i := 0; i < 100; i++ {
			rl.Allow("foo", "mix")
		}
		stats, _ = rl.GetLimiterStats("foo", "mix", true)
		assert.Equal(t, stats.Allowed, int64(100))
		assert.Equal(t, stats.Dropped, int64(100))

		rl.AddNewLimiter("foo", "burst", rate.Limit(50), 100)
		for i := 0; i < 100; i++ {
			rl.Allow("foo", "burst")
		}
		time.Sleep(time.Second * 1)
		for i := 0; i < 100; i++ {
			rl.Allow("foo", "burst")
		}
		time.Sleep(time.Second * 2)
		for i := 0; i < 100; i++ {
			rl.Allow("foo", "burst")
		}
		stats, _ = rl.GetLimiterStats("foo", "burst", true)
		assert.Equal(t, stats.Allowed, int64(250))
		assert.Equal(t, stats.Dropped, int64(50))
	})

	t.Run("rate-limiter-update-limit", func(t *testing.T) {
		rl := utils.NewRateLimiter(nil, utils.LimiterOpts{Limits: emptyLimits})

		rl.AddNewLimiter("foo", "bar", rate.Limit(10), 20)
		limit, burst, _ := rl.GetLimit("foo", "bar")
		assert.Equal(t, limit, rate.Limit(10))
		assert.Equal(t, burst, 20)

		err := rl.UpdateLimit("foo", "bar", rate.Limit(20), 10)
		if err != nil {
			t.Fatal()
		}
		limit, burst, _ = rl.GetLimit("foo", "bar")
		assert.Equal(t, limit, rate.Limit(20))
		assert.Equal(t, burst, 10)

		err = rl.UpdateLimit("foo", "baz", rate.Limit(20), 10)
		if err.Error() != "ENOENT" {
			t.Fatal()
		}

		err = rl.UpdateLimit("foo", "bar", rate.Limit(20), -10)
		if err.Error() != "EINVAL" {
			t.Fatal()
		}
	})

	t.Run("rate-limiter-update-group-limit", func(t *testing.T) {
		rl := utils.NewRateLimiter(nil, utils.LimiterOpts{Limits: emptyLimits})

		rl.AddNewLimiter("foo", "bar", rate.Limit(10), 10)
		rl.AddNewLimiter("foo", "baz", rate.Limit(20), 20)

		err := rl.UpdateGroupLimit("foo", rate.Limit(30), 30)
		if err != nil {
			t.Fatal()
		}
		limit, burst, _ := rl.GetLimit("foo", "bar")
		assert.Equal(t, limit, rate.Limit(30))
		assert.Equal(t, burst, 30)
		limit, burst, _ = rl.GetLimit("foo", "baz")
		assert.Equal(t, limit, rate.Limit(30))
		assert.Equal(t, burst, 30)

		err = rl.UpdateGroupLimit("foo", rate.Limit(40), -1)
		if err.Error() != "EINVAL" {
			t.Fatal()
		}
		limit, burst, _ = rl.GetLimit("foo", "bar")
		assert.Equal(t, limit, rate.Limit(30))
		assert.Equal(t, burst, 30)
		limit, burst, _ = rl.GetLimit("foo", "baz")
		assert.Equal(t, limit, rate.Limit(30))
		assert.Equal(t, burst, 30)

		err = rl.UpdateGroupLimit("FOO", rate.Limit(50), 50)
		if err.Error() != "ENOENT" {
			t.Fatal()
		}
		limit, burst, _ = rl.GetLimit("foo", "bar")
		assert.Equal(t, limit, rate.Limit(30))
		assert.Equal(t, burst, 30)
		limit, burst, _ = rl.GetLimit("foo", "baz")
		assert.Equal(t, limit, rate.Limit(30))
		assert.Equal(t, burst, 30)
	})

	t.Run("rate-limiter-get-limiter-stats", func(t *testing.T) {
		rl := utils.NewRateLimiter(nil, utils.LimiterOpts{Limits: emptyLimits})

		stats, err := rl.GetLimiterStats("foo", "bar", true)
		if err.Error() != "ENOENT" {
			t.Fatal()
		}

		rl.AddNewLimiter("foo", "bar", rate.Limit(50), 50)
		for i := 0; i < 100; i++ {
			rl.Allow("foo", "bar")
		}
		stats, err = rl.GetLimiterStats("foo", "bar", false)
		if err != nil {
			t.Fatal()
		}
		assert.Equal(t, stats.Allowed, int64(50))
		assert.Equal(t, stats.Dropped, int64(50))

		for i := 0; i < 100; i++ {
			rl.Allow("foo", "bar")
		}
		stats, _ = rl.GetLimiterStats("foo", "bar", true)
		assert.Equal(t, stats.Allowed, int64(50))
		assert.Equal(t, stats.Dropped, int64(150))

		for i := 0; i < 100; i++ {
			rl.Allow("foo", "bar")
		}
		stats, _ = rl.GetLimiterStats("foo", "bar", true)
		assert.Equal(t, stats.Allowed, int64(0))
		assert.Equal(t, stats.Dropped, int64(100))
		stats, _ = rl.GetLimiterStats("foo", "bar", true)
		assert.Equal(t, stats.Allowed, int64(0))
		assert.Equal(t, stats.Dropped, int64(0))
	})

	t.Run("rate-limiter-get-global-group-stats", func(t *testing.T) {
		rl := utils.NewRateLimiter(nil, utils.LimiterOpts{Limits: emptyLimits})

		stats, err := rl.GetGlobalGroupStats("foo", false)
		if err.Error() != "ENOENT" {
			t.Fatal()
		}

		rl.AddNewLimiter("foo", "bar", rate.Limit(50), 50)
		rl.AddNewLimiter("foo", "baz", rate.Limit(50), 100)
		for i := 0; i < 100; i++ {
			rl.Allow("foo", "bar")
			rl.Allow("foo", "baz")
		}
		stats, err = rl.GetGlobalGroupStats("foo", false)
		if err != nil {
			t.Fatal()
		}
		assert.Equal(t, stats.Allowed, int64(150))
		assert.Equal(t, stats.Dropped, int64(50))

		for i := 0; i < 100; i++ {
			rl.Allow("foo", "bar")
			rl.Allow("foo", "baz")
		}
		stats, _ = rl.GetGlobalGroupStats("foo", true)
		assert.Equal(t, stats.Allowed, int64(150))
		assert.Equal(t, stats.Dropped, int64(250))

		stats, _ = rl.GetGlobalGroupStats("foo", true)
		assert.Equal(t, stats.Allowed, int64(0))
		assert.Equal(t, stats.Dropped, int64(0))
	})

	t.Run("rate-limiter-get-all-group-stats", func(t *testing.T) {
		rl := utils.NewRateLimiter(nil, utils.LimiterOpts{Limits: emptyLimits})

		_, err := rl.GetAllGroupStats("foo")
		if err.Error() != "ENOENT" {
			t.Fatal()
		}

		rl.AddNewLimiter("foo", "bar", rate.Limit(50), 50)
		rl.AddNewLimiter("foo", "baz", rate.Limit(50), 100)
		for i := 0; i < 100; i++ {
			rl.Allow("foo", "bar")
			rl.Allow("foo", "baz")
		}
		stats, err := rl.GetAllGroupStats("foo")
		if err != nil {
			t.Fatal()
		}
		bar := stats["bar"]
		assert.Equal(t, bar.Allowed, int64(50))
		assert.Equal(t, bar.Dropped, int64(50))
		baz := stats["baz"]
		assert.Equal(t, baz.Allowed, int64(100))
		assert.Equal(t, baz.Dropped, int64(0))

		stats, _ = rl.GetAllGroupStats("foo")
		bar = stats["bar"]
		assert.Equal(t, bar.Allowed, int64(0))
		assert.Equal(t, bar.Dropped, int64(0))
		baz = stats["baz"]
		assert.Equal(t, baz.Allowed, int64(0))
		assert.Equal(t, baz.Dropped, int64(0))
	})

}
