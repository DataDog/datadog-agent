// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package server

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewBlocklist(t *testing.T) {
	check := func(data []string) []string {
		b := newBlocklist(data, true)
		return *(b.data.Load())
	}

	assert.Equal(t, []string{}, check([]string{}))
	assert.Equal(t, []string{"a"}, check([]string{"a"}))
	assert.Equal(t, []string{"a"}, check([]string{"a", "aa"}))
	assert.Equal(t, []string{"a", "b"}, check([]string{"a", "aa", "b", "bb"}))
	assert.Equal(t, []string{"a", "b"}, check([]string{"a", "b", "bb"}))
}

func TestIsMetricBlocklisted(t *testing.T) {
	cases := []struct {
		result      bool
		name        string
		blocklist   []string
		matchPrefix bool
	}{
		{false, "some", []string{}, false},
		{false, "some", []string{}, true},
		{false, "foo", []string{"bar", "baz"}, false},
		{false, "foo", []string{"bar", "baz"}, true},
		{false, "bar", []string{"foo", "baz"}, false},
		{false, "bar", []string{"foo", "baz"}, true},
		{true, "baz", []string{"foo", "baz"}, false},
		{true, "baz", []string{"foo", "baz"}, true},
		{false, "foobar", []string{"foo", "baz"}, false},
		{true, "foobar", []string{"foo", "baz"}, true},
	}
	for _, c := range cases {
		t.Run(fmt.Sprintf("%v-%v-%v", c.name, c.blocklist, c.matchPrefix),
			func(t *testing.T) {
				b := newBlocklist(c.blocklist, c.matchPrefix)
				assert.Equal(t, c.result, b.test(c.name))
			})
	}
}

// TestUpdateBlocklist validates (with the -race flag) that there is no
// racy behaviour while updating the blocklist from a separate goroutine.
func TestUpdateBlocklist(t *testing.T) {
	metricNameToTest := "datadog.agent.metric"

	blist := newBlocklist([]string{
		"datadog", "woof", metricNameToTest,
		"foo", "bar", "foobar",
	}, false)

	var mu sync.Mutex
	mustBlock := true

	// this routine will constantly test that a metric is blocked or not
	// by the blocklist
	go func(mustBlock *bool, mu *sync.Mutex, blist *blocklist) {
		timer := time.NewTimer(200 * time.Millisecond)
		for {
			mu.Lock()
			mustBlock := *mustBlock
			mu.Unlock()
			require.Equal(t, mustBlock, blist.test(metricNameToTest), fmt.Sprintf("blocklist used: %v", *(blist.data.Load())))
			select {
			case <-timer.C:
				timer.Stop()
				return
			default:
			}
		}
	}(&mustBlock, &mu, &blist)

	// let the other routine spawn and schedule
	time.Sleep(10 * time.Millisecond)

	// with this change, the metric should still be blocked, this change
	// must not be racy despite no use of any lock
	blist.update([]string{
		"other", "strings", "but", "still",
		"the", "matching", metricNameToTest, "one",
	})

	// just make sure the other routine has time to spin
	time.Sleep(10 * time.Millisecond)

	// after this change, the metric should not be blocked anymore,
	// make sure we don't test a metric while we do this change
	// with the mutex
	mu.Lock()
	mustBlock = false
	blist.update([]string{
		"some", "strings", "and", "no",
		"matching", "one",
	})
	mu.Unlock()

	// just make sure the other routine has time to spin
	time.Sleep(10 * time.Millisecond)
}
