// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright © 2015 Kentaro Kuribayashi <kentarok@gmail.com>
// Copyright 2014-present Datadog, Inc.

//go:build linux || darwin

package gops

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUsernames(t *testing.T) {
	png := getProcessNameGroup()
	usernames := png.Usernames()
	assert.Equal(t, []string{"foo_user", "sample_user", "test_user", "user_foo"}, usernames)
}

func TestProcessNameGroupGetters(t *testing.T) {
	png := getProcessNameGroup()
	assert.Equal(t, []int32{1, 3, 56, 234, 784}, png.Pids())
	assert.Equal(t, "pgroup1", png.Name())
	assert.Equal(t, uint64(54328), png.RSS())
	assert.InDelta(t, 56.9, png.PctMem(), 0.001)
	assert.Equal(t, uint64(2515828), png.VMS())
}

func TestGroupByName(t *testing.T) {
	procs := []*ProcessInfo{
		{PID: 1, Name: "nginx", RSS: 100, PctMem: 1.0, VMS: 200, Username: "root"},
		{PID: 2, Name: "nginx", RSS: 150, PctMem: 1.5, VMS: 300, Username: "www"},
		{PID: 3, Name: "redis", RSS: 500, PctMem: 5.0, VMS: 800, Username: "redis"},
	}

	groups := GroupByName(procs)
	assert.Len(t, groups, 2)

	// find nginx group
	var nginxGroup, redisGroup *ProcessNameGroup
	for _, g := range groups {
		switch g.Name() {
		case "nginx":
			nginxGroup = g
		case "redis":
			redisGroup = g
		}
	}

	assert.NotNil(t, nginxGroup)
	assert.Equal(t, []int32{1, 2}, nginxGroup.Pids())
	assert.Equal(t, uint64(250), nginxGroup.RSS())
	assert.InDelta(t, 2.5, nginxGroup.PctMem(), 0.001)
	assert.Equal(t, uint64(500), nginxGroup.VMS())

	assert.NotNil(t, redisGroup)
	assert.Equal(t, []int32{3}, redisGroup.Pids())
}

func TestProcessNameGroupsSortByRSS(t *testing.T) {
	procs := []*ProcessInfo{
		{PID: 1, Name: "small", RSS: 10, Username: "u"},
		{PID: 2, Name: "big", RSS: 1000, Username: "u"},
	}
	groups := GroupByName(procs)

	sorter := ByRSSDesc{groups}
	assert.Equal(t, 2, sorter.Len())
	// The group with more RSS should sort first
	if groups[0].Name() == "small" {
		assert.True(t, sorter.Less(1, 0))
	} else {
		assert.True(t, sorter.Less(0, 1))
	}
}

func getProcessNameGroup() *ProcessNameGroup {
	return &ProcessNameGroup{
		pids:   []int32{1, 3, 56, 234, 784},
		rss:    uint64(54328),
		pctMem: 56.9,
		vms:    uint64(2515828),
		name:   "pgroup1",
		usernames: map[string]bool{
			"sample_user": true,
			"user_foo":    true,
			"foo_user":    true,
			"test_user":   true,
		},
	}
}
