// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build linux

package probe

import (
	"reflect"
	"sort"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/security/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/eval"
)

func TestPathValidation(t *testing.T) {
	model := &Model{}
	if err := model.ValidateField("open.file.path", eval.FieldValue{Value: "/var/log/*"}); err != nil {
		t.Fatalf("shouldn't return an error: %s", err)
	}
	if err := model.ValidateField("open.file.path", eval.FieldValue{Value: "~/apache/httpd.conf"}); err == nil {
		t.Fatal("should return an error")
	}
	if err := model.ValidateField("open.file.path", eval.FieldValue{Value: "../../../etc/apache/httpd.conf"}); err == nil {
		t.Fatal("should return an error")
	}
	if err := model.ValidateField("open.file.path", eval.FieldValue{Value: "/etc/apache/./httpd.conf"}); err == nil {
		t.Fatal("should return an error")
	}
	if err := model.ValidateField("open.file.path", eval.FieldValue{Value: "*/"}); err == nil {
		t.Fatal("should return an error")
	}
	if err := model.ValidateField("open.file.path", eval.FieldValue{Value: "/1/2/3/4/5/6/7/8/9/10/11/12/13/14/15/16"}); err == nil {
		t.Fatal("should return an error")
	}
	if err := model.ValidateField("open.file.path", eval.FieldValue{Value: "f59226f52267c120c1accfe3d158aa2f201ff02f45692a1c574da29c07fb985ef59226f52267c120c1accfe3d158aa2f201ff02f45692a1c574da29c07fb985e"}); err == nil {
		t.Fatal("should return an error")
	}

	if err := model.ValidateField("open.file.path", eval.FieldValue{Value: ".*", Type: eval.RegexpValueType}); err == nil {
		t.Fatal("should return an error")
	}

	if err := model.ValidateField("open.file.path", eval.FieldValue{Value: "/etc/*", Type: eval.PatternValueType}); err != nil {
		t.Fatal("shouldn't return an error")
	}
}

func TestSetFieldValue(t *testing.T) {
	event := &Event{}

	for _, field := range event.GetFields() {
		kind, err := event.GetFieldType(field)
		if err != nil {
			t.Fatal(err)
		}

		switch kind {
		case reflect.String:
			if err = event.SetFieldValue(field, "aaa"); err != nil {
				t.Fatal(err)
			}
		case reflect.Int:
			if err = event.SetFieldValue(field, 123); err != nil {
				t.Fatal(err)
			}
		case reflect.Bool:
			if err = event.SetFieldValue(field, true); err != nil {
				t.Fatal(err)
			}
		default:
			t.Fatalf("type unknown: %v", kind)
		}
	}
}

func TestExecArgsFlags(t *testing.T) {
	e := Event{
		Event: model.Event{
			Exec: model.ExecEvent{
				Process: model.Process{
					ArgsArray: []string{
						"-abc", "--verbose", "test",
						"-v=1", "--host=myhost",
						"-9", "-", "--",
					},
				},
			},
		},
	}

	flags := e.ResolveExecArgsFlags(&e.Exec)
	sort.Sort(sort.StringSlice(flags))

	hasFlag := func(flags []string, flag string) bool {
		i := sort.SearchStrings(flags, flag)
		return i < len(flags) && flags[i] == flag
	}

	if !hasFlag(flags, "a") {
		t.Error("flags 'a' not found")
	}

	if !hasFlag(flags, "b") {
		t.Error("flags 'b' not found")
	}

	if hasFlag(flags, "v") {
		t.Error("flags 'v' found")
	}

	if !hasFlag(flags, "9") {
		t.Error("flags '9' not found found")
	}

	if !hasFlag(flags, "abc") {
		t.Error("flags 'abc' not found")
	}

	if !hasFlag(flags, "verbose") {
		t.Error("flags 'verbose' not found")
	}

	if len(flags) != 6 {
		t.Errorf("expected 6 flags, got %d", len(flags))
	}
}

func TestExecArgsOptions(t *testing.T) {
	e := Event{
		Event: model.Event{
			Exec: model.ExecEvent{
				Process: model.Process{
					ArgsArray: []string{
						"--config", "/etc/myfile", "--host=myhost", "--verbose",
						"-c", "/etc/myfile", "-h=myhost", "-v",
						"--", "---", "-9",
					},
				},
			},
		},
	}

	options := e.ResolveExecArgsOptions(&e.Exec)
	sort.Sort(sort.StringSlice(options))

	hasOption := func(options []string, option string) bool {
		i := sort.SearchStrings(options, option)
		return i < len(options) && options[i] == option
	}

	if !hasOption(options, "config=/etc/myfile") {
		t.Error("option 'config=/etc/myfile' not found")
	}

	if !hasOption(options, "c=/etc/myfile") {
		t.Error("option 'c=/etc/myfile' not found")
	}

	if !hasOption(options, "host=myhost") {
		t.Error("option 'host=myhost' not found")
	}

	if !hasOption(options, "h=myhost") {
		t.Error("option 'h=myhost' not found")
	}

	if hasOption(options, "verbose=") {
		t.Error("option 'verbose=' found")
	}

	if len(options) != 4 {
		t.Errorf("expected 4 options, got %d", len(options))
	}
}
