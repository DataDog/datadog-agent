// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package procutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestProcessIdentity(t *testing.T) {
	// Basic identity generation
	identity := ProcessIdentity(1234, 1000000, []string{"bash", "-c", "echo hello"})
	assert.Contains(t, identity, "pid:1234")
	assert.Contains(t, identity, "createTime:1000000")
	assert.Contains(t, identity, "cmdHash:")
}

func TestProcessIdentityDifferentCmdline(t *testing.T) {
	// Same PID and createTime, different cmdline should produce different identity
	identity1 := ProcessIdentity(1234, 1000000, []string{"bash"})
	identity2 := ProcessIdentity(1234, 1000000, []string{"htop"})

	assert.NotEqual(t, identity1, identity2, "Different cmdlines should produce different identities")
}

func TestProcessIdentitySameCmdline(t *testing.T) {
	// Same PID, createTime, and cmdline should produce same identity
	identity1 := ProcessIdentity(1234, 1000000, []string{"bash", "-c", "echo"})
	identity2 := ProcessIdentity(1234, 1000000, []string{"bash", "-c", "echo"})

	assert.Equal(t, identity1, identity2, "Same inputs should produce same identity")
}

func TestProcessIdentityArgOrderMatters(t *testing.T) {
	// Argument order should matter
	identity1 := ProcessIdentity(1234, 1000000, []string{"cmd", "a", "b"})
	identity2 := ProcessIdentity(1234, 1000000, []string{"cmd", "b", "a"})

	assert.NotEqual(t, identity1, identity2, "Different arg order should produce different identities")
}

func TestProcessIdentityTruncatesAt100Args(t *testing.T) {
	// Create cmdline with 150 args
	cmdline150 := make([]string, 150)
	for i := range cmdline150 {
		cmdline150[i] = "arg"
	}

	// Same first 100 args, but different args after 100
	cmdline150Different := make([]string, 150)
	copy(cmdline150Different, cmdline150)
	cmdline150Different[120] = "DIFFERENT" // Change arg at position 120

	identity1 := ProcessIdentity(1234, 1000000, cmdline150)
	identity2 := ProcessIdentity(1234, 1000000, cmdline150Different)

	// Should be equal because we only hash first 100 args
	assert.Equal(t, identity1, identity2, "Should only hash first 100 args, differences after should be ignored")
}

func TestProcessIdentityDifferenceWithin100Args(t *testing.T) {
	// Create cmdline with 150 args
	cmdline150 := make([]string, 150)
	for i := range cmdline150 {
		cmdline150[i] = "arg"
	}

	// Different arg within first 100
	cmdline150Different := make([]string, 150)
	copy(cmdline150Different, cmdline150)
	cmdline150Different[50] = "DIFFERENT" // Change arg at position 50

	identity1 := ProcessIdentity(1234, 1000000, cmdline150)
	identity2 := ProcessIdentity(1234, 1000000, cmdline150Different)

	// Should be different because change is within first 100 args
	assert.NotEqual(t, identity1, identity2, "Differences within first 100 args should produce different identities")
}

func TestProcessIdentityEmptyCmdline(t *testing.T) {
	// Empty cmdline should work
	identity1 := ProcessIdentity(1234, 1000000, []string{})
	identity2 := ProcessIdentity(1234, 1000000, nil)

	// Both empty, should be equal
	assert.Equal(t, identity1, identity2, "Empty and nil cmdlines should produce same identity")
}

func TestProcessIdentityArgBoundaryDistinction(t *testing.T) {
	// Ensure ["ab", "c"] and ["a", "bc"] hash differently (null separator works)
	identity1 := ProcessIdentity(1234, 1000000, []string{"ab", "c"})
	identity2 := ProcessIdentity(1234, 1000000, []string{"a", "bc"})

	assert.NotEqual(t, identity1, identity2, "Different arg boundaries should produce different identities")
}

func TestIsSameProcess(t *testing.T) {
	createTime := int64(1000000)

	tests := []struct {
		name     string
		procA    *Process
		procB    *Process
		expected bool
	}{
		{
			name: "identical processes",
			procA: &Process{
				Pid:     1234,
				Cmdline: []string{"bash", "-c", "echo"},
				Stats:   &Stats{CreateTime: createTime},
			},
			procB: &Process{
				Pid:     1234,
				Cmdline: []string{"bash", "-c", "echo"},
				Stats:   &Stats{CreateTime: createTime},
			},
			expected: true,
		},
		{
			name: "different PID",
			procA: &Process{
				Pid:     1234,
				Cmdline: []string{"bash"},
				Stats:   &Stats{CreateTime: createTime},
			},
			procB: &Process{
				Pid:     5678,
				Cmdline: []string{"bash"},
				Stats:   &Stats{CreateTime: createTime},
			},
			expected: false,
		},
		{
			name: "different create time",
			procA: &Process{
				Pid:     1234,
				Cmdline: []string{"bash"},
				Stats:   &Stats{CreateTime: createTime},
			},
			procB: &Process{
				Pid:     1234,
				Cmdline: []string{"bash"},
				Stats:   &Stats{CreateTime: createTime + 1000},
			},
			expected: false,
		},
		{
			name: "different cmdline (exec scenario)",
			procA: &Process{
				Pid:     1234,
				Cmdline: []string{"bash"},
				Stats:   &Stats{CreateTime: createTime},
			},
			procB: &Process{
				Pid:     1234,
				Cmdline: []string{"htop"},
				Stats:   &Stats{CreateTime: createTime},
			},
			expected: false,
		},
		{
			name: "different cmdline args",
			procA: &Process{
				Pid:     1234,
				Cmdline: []string{"cmd", "a", "b"},
				Stats:   &Stats{CreateTime: createTime},
			},
			procB: &Process{
				Pid:     1234,
				Cmdline: []string{"cmd", "b", "a"},
				Stats:   &Stats{CreateTime: createTime},
			},
			expected: false,
		},
		{
			name: "empty vs non-empty cmdline",
			procA: &Process{
				Pid:     1234,
				Cmdline: []string{},
				Stats:   &Stats{CreateTime: createTime},
			},
			procB: &Process{
				Pid:     1234,
				Cmdline: []string{"bash"},
				Stats:   &Stats{CreateTime: createTime},
			},
			expected: false,
		},
		{
			name:     "nil Stats in procA skips CreateTime check",
			procA:    &Process{Pid: 1234, Cmdline: []string{"bash"}, Stats: nil},
			procB:    &Process{Pid: 1234, Cmdline: []string{"bash"}, Stats: &Stats{CreateTime: createTime}},
			expected: true,
		},
		{
			name:     "nil Stats in procB skips CreateTime check",
			procA:    &Process{Pid: 1234, Cmdline: []string{"bash"}, Stats: &Stats{CreateTime: createTime}},
			procB:    &Process{Pid: 1234, Cmdline: []string{"bash"}, Stats: nil},
			expected: true,
		},
		{
			name:     "nil Stats in both skips CreateTime check",
			procA:    &Process{Pid: 1234, Cmdline: []string{"bash"}, Stats: nil},
			procB:    &Process{Pid: 1234, Cmdline: []string{"bash"}, Stats: nil},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsSameProcess(tt.procA, tt.procB)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestProcessIdentityAndIsSameProcessInSync ensures that ProcessIdentity and IsSameProcess
// use the same fields to determine process identity. If one is modified, this test should
// fail to remind developers to update the other.
func TestProcessIdentityAndIsSameProcessInSync(t *testing.T) {
	createTime := int64(1000000)

	// Create pairs of processes that differ in each identity field
	testCases := []struct {
		name         string
		procA        *Process
		procB        *Process
		shouldBeSame bool
		description  string
	}{
		{
			name: "same identity fields",
			procA: &Process{
				Pid:     1234,
				Cmdline: []string{"bash", "-c", "echo"},
				Stats:   &Stats{CreateTime: createTime},
			},
			procB: &Process{
				Pid:     1234,
				Cmdline: []string{"bash", "-c", "echo"},
				Stats:   &Stats{CreateTime: createTime},
			},
			shouldBeSame: true,
			description:  "processes with identical identity fields should match in both functions",
		},
		{
			name: "different pid",
			procA: &Process{
				Pid:     1234,
				Cmdline: []string{"bash"},
				Stats:   &Stats{CreateTime: createTime},
			},
			procB: &Process{
				Pid:     5678,
				Cmdline: []string{"bash"},
				Stats:   &Stats{CreateTime: createTime},
			},
			shouldBeSame: false,
			description:  "different PID should be detected by both functions",
		},
		{
			name: "different createTime",
			procA: &Process{
				Pid:     1234,
				Cmdline: []string{"bash"},
				Stats:   &Stats{CreateTime: createTime},
			},
			procB: &Process{
				Pid:     1234,
				Cmdline: []string{"bash"},
				Stats:   &Stats{CreateTime: createTime + 1000},
			},
			shouldBeSame: false,
			description:  "different createTime should be detected by both functions",
		},
		{
			name: "different cmdline (exec scenario)",
			procA: &Process{
				Pid:     1234,
				Cmdline: []string{"bash"},
				Stats:   &Stats{CreateTime: createTime},
			},
			procB: &Process{
				Pid:     1234,
				Cmdline: []string{"htop"},
				Stats:   &Stats{CreateTime: createTime},
			},
			shouldBeSame: false,
			description:  "different cmdline should be detected by both functions",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Check ProcessIdentity
			identityA := ProcessIdentity(tc.procA.Pid, tc.procA.Stats.CreateTime, tc.procA.Cmdline)
			identityB := ProcessIdentity(tc.procB.Pid, tc.procB.Stats.CreateTime, tc.procB.Cmdline)
			identitySame := identityA == identityB

			// Check IsSameProcess
			isSame := IsSameProcess(tc.procA, tc.procB)

			// Both functions should agree
			assert.Equal(t, tc.shouldBeSame, identitySame,
				"ProcessIdentity: %s - got identitySame=%v, want %v", tc.description, identitySame, tc.shouldBeSame)
			assert.Equal(t, tc.shouldBeSame, isSame,
				"IsSameProcess: %s - got isSame=%v, want %v", tc.description, isSame, tc.shouldBeSame)
			assert.Equal(t, identitySame, isSame,
				"ProcessIdentity and IsSameProcess disagree! This likely means one was modified without updating the other. %s", tc.description)
		})
	}
}
