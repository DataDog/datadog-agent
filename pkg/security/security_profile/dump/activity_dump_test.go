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

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

func Test_extractFirstParent(t *testing.T) {
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
			got, got1 := extractFirstParent(tt.args.path)
			assert.Equalf(t, tt.want, got, "extractFirstParent(%v)", tt.args.path)
			assert.Equalf(t, tt.want1, got1, "extractFirstParent(%v)", tt.args.path)
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
