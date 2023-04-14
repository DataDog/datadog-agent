// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package dump

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"golang.org/x/exp/slices"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

func Test_ExtractFirstParent(t *testing.T) {
	type args struct {
		path string
	}
	tests := []struct {
		name  string
		args  args
		want  string
		want1 int
	}{
		{
			name:  "empty path",
			args:  args{},
			want:  "",
			want1: 0,
		},
		{
			name:  "root path",
			args:  args{"/"},
			want:  "",
			want1: 0,
		},
		{
			name:  "one char with slash",
			args:  args{"/h"},
			want:  "h",
			want1: 2,
		},
		{
			name:  "one char no slash",
			args:  args{"h"},
			want:  "h",
			want1: 1,
		},
		{
			name:  "single node with slash",
			args:  args{"/hello"},
			want:  "hello",
			want1: 6,
		},
		{
			name:  "single node no slash",
			args:  args{"hello"},
			want:  "hello",
			want1: 5,
		},
		{
			name:  "parent with slash",
			args:  args{"/hello/there/"},
			want:  "hello",
			want1: 6,
		},
		{
			name:  "parent no slash",
			args:  args{"hello/there/"},
			want:  "hello",
			want1: 5,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got1 := ExtractFirstParent(tt.args.path)
			assert.Equalf(t, tt.want, got, "ExtractFirstParent(%v)", tt.args.path)
			assert.Equalf(t, tt.want1, got1, "ExtractFirstParent(%v)", tt.args.path)
		})
	}
}

func TestInsertFileEvent(t *testing.T) {
	pan := ProcessActivityNode{
		Files: make(map[string]*FileActivityNode),
	}
	pan.Process.FileEvent.PathnameStr = "/test/pan"

	pathToInserts := []string{
		"/tmp/foo",
		"/tmp/bar",
		"/test/a/b/c/d/e/",
		"/hello",
		"/tmp/bar/test",
	}
	expectedDebugOuput := strings.TrimSpace(`
- process: /test/pan
  files:
    - hello
    - test
        - a
            - b
                - c
                    - d
                        - e
    - tmp
        - bar
            - test
        - foo
`)

	ad := NewEmptyActivityDump()

	for _, path := range pathToInserts {
		event := &model.Event{
			Open: model.OpenEvent{
				File: model.FileEvent{
					IsPathnameStrResolved: true,
					PathnameStr:           path,
				},
			},
			FieldHandlers: &model.DefaultFieldHandlers{},
		}
		ad.InsertFileEventInProcess(&pan, &event.Open.File, event, Unknown)
	}

	var builder strings.Builder
	pan.debug(&builder, "")
	debugOutput := strings.TrimSpace(builder.String())

	assert.Equal(t, expectedDebugOuput, debugOutput)
}

func TestActivityDump_computeSyscallsList(t *testing.T) {
	tests := []struct {
		name string
		dump ActivityDump
		want []uint32
	}{
		{
			name: "empty",
			dump: ActivityDump{},
			want: []uint32{},
		},
		{
			name: "one_node",
			dump: ActivityDump{
				ProcessActivityTree: []*ProcessActivityNode{
					{Syscalls: []int{2, 4, 7}},
				},
			},
			want: []uint32{2, 4, 7},
		},
		{
			name: "two_nodes",
			dump: ActivityDump{
				ProcessActivityTree: []*ProcessActivityNode{
					{Syscalls: []int{2, 4, 7}},
					{Syscalls: []int{350, 7, 70}},
				},
			},
			want: []uint32{2, 4, 7, 70, 350},
		},
		{
			name: "two_nodes",
			dump: ActivityDump{
				ProcessActivityTree: []*ProcessActivityNode{
					{Syscalls: []int{2, 4, 7}},
					{Syscalls: []int{350, 7, 70}},
				},
			},
			want: []uint32{2, 4, 7, 70, 350},
		},
		{
			name: "children_nodes",
			dump: ActivityDump{
				ProcessActivityTree: []*ProcessActivityNode{
					{Syscalls: []int{2, 4, 7}, Children: []*ProcessActivityNode{{Syscalls: []int{123, 44, 64, 65}}, {Syscalls: []int{123, 44, 64, 65, 22}}, {}}},
					{Syscalls: []int{350, 7, 70}},
				},
			},
			want: []uint32{2, 4, 7, 22, 44, 64, 65, 70, 123, 350},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := tt.dump.computeSyscallsList()
			slices.Sort(output)
			assert.Equalf(t, tt.want, output, "computeSyscallsList()")
		})
	}
}
