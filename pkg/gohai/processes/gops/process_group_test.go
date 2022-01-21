// +build linux darwin

package gops

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestUsernames(t *testing.T) {
	png := getProcessNameGroup()
	usernames := png.Usernames()
	assert.Equal(t, []string{"foo_user", "sample_user", "test_user", "user_foo"}, usernames)
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
