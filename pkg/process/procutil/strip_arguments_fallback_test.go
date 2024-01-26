package procutil

import (

	// "testing"
	// "github.com/stretchr/testify/assert"
)

//go:build !linux && !windows

// func TestStripArguments(b *testing.B) {

// 	cases := []struct {
// 		cmdline      []string
// 		striplessCmdline []string
// 	}{	
// 		{[]string{"agent", "-password", "1234"}, []string{"agent"}},
// 		{[]string{"fitz", "-consul_token", "1234567890"}, []string{"fitz"}},
// 		{[]string{"fitz", "--consul_token", "1234567890"}, []string{"fitz"}},
// 		{[]string{"python ~/test/run.py -open_password=admin -consul_token 2345 -blocked_from_yaml=1234 &"},[]string{"python"},},
// 		{[]string{"java -password      1234"}, []string{"java"}},
// 		{[]string{"agent password:1234"}, []string{"agent"}},
// 	}

	// for i := range cases {
	// 	fp := &Process{Cmdline: cases[i].cmdline}
	// 	cases[i].cmdline.stripArguments
	// 	assert.Equal(t, cases[i].triplessCmdline, cases[i].cmdline)
	// }
// }