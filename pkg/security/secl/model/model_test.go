// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package model

import (
	"errors"
	"net"
	"reflect"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
)

func TestPathValidation(t *testing.T) {
	mod := &Model{}
	if err := mod.ValidateField("open.file.path", eval.FieldValue{Value: "/var/log/*"}); err != nil {
		t.Errorf("shouldn't return an error: %s", err)
	}
	if err := mod.ValidateField("open.file.path", eval.FieldValue{Value: "~/apache/httpd.conf"}); err == nil {
		t.Error("should return an error")
	}
	if err := mod.ValidateField("open.file.path", eval.FieldValue{Value: "../../../etc/apache/httpd.conf"}); err == nil {
		t.Error("should return an error")
	}
	if err := mod.ValidateField("open.file.path", eval.FieldValue{Value: "/etc/apache/./httpd.conf"}); err == nil {
		t.Error("should return an error")
	}
	if err := mod.ValidateField("open.file.path", eval.FieldValue{Value: "*/"}); err == nil {
		t.Error("should return an error")
	}
	if err := mod.ValidateField("open.file.path", eval.FieldValue{Value: "~/"}); err == nil {
		t.Error("should return an error")
	}

	var val string
	for i := 0; i <= MaxPathDepth; i++ {
		val += "a/"
	}
	if err := mod.ValidateField("open.file.path", eval.FieldValue{Value: val}); err == nil {
		t.Error("should return an error")
	}

	val = ""
	for i := 0; i <= MaxSegmentLength; i++ {
		val += "a"
	}
	if err := mod.ValidateField("open.file.path", eval.FieldValue{Value: val}); err == nil {
		t.Error("should return an error")
	}

	if err := mod.ValidateField("open.file.path", eval.FieldValue{Value: "/run/..data", Type: eval.PatternValueType}); err != nil {
		t.Error("shouldn't return an error")
	}

	if err := mod.ValidateField("open.file.path", eval.FieldValue{Value: "./data", Type: eval.PatternValueType}); err == nil {
		t.Error("should return an error")
	}

	if err := mod.ValidateField("open.file.path", eval.FieldValue{Value: "../data", Type: eval.PatternValueType}); err == nil {
		t.Error("should return an error")
	}

	if err := mod.ValidateField("open.file.path", eval.FieldValue{Value: "/data/../a", Type: eval.PatternValueType}); err == nil {
		t.Error("should return an error")
	}

	if err := mod.ValidateField("open.file.path", eval.FieldValue{Value: "*/data", Type: eval.PatternValueType}); err != nil {
		t.Error("shouldn't return an error")
	}

	if err := mod.ValidateField("open.file.path", eval.FieldValue{Value: ".*", Type: eval.RegexpValueType}); err == nil {
		t.Error("should return an error")
	}

	if err := mod.ValidateField("open.file.path", eval.FieldValue{Value: "/etc/*", Type: eval.PatternValueType}); err != nil {
		t.Error("shouldn't return an error")
	}

	if err := mod.ValidateField("open.file.path", eval.FieldValue{Value: "*", Type: eval.PatternValueType}); err == nil {
		t.Error("should return an error")
	}
}

func TestSetFieldValue(t *testing.T) {
	var readOnlyError *eval.ErrFieldReadOnly

	event := NewDefaultEvent()
	for _, field := range event.(*Event).GetFields() {
		kind, err := event.GetFieldType(field)
		if err != nil {
			t.Fatal(err)
		}

		switch kind {
		case reflect.String:
			err = event.SetFieldValue(field, "aaa")
			if err != nil {
				if errors.As(err, &readOnlyError) {
					continue
				} else {
					t.Error(err)
				}
			}
			if err = event.SetFieldValue(field, "aaa"); err != nil && !errors.As(err, &readOnlyError) {
				t.Error(err)
			}
			value, err := event.GetFieldValue(field)
			if err != nil {
				t.Errorf("unable to get the expected `%s` value: %v", field, err)
			}
			switch v := value.(type) {
			case string:
				if v != "aaa" {
					t.Errorf("unable to get the expected `%s` value: %v", field, v)
				}
			case []string:
				if v[0] != "aaa" {
					t.Errorf("unable to get the expected `%s` value: %v", field, v)
				}
			default:
				t.Errorf("unable to get the expected `%s` value: %v", field, v)
			}
		case reflect.Int:
			err = event.SetFieldValue(field, 123)
			if err != nil {
				if errors.As(err, &readOnlyError) {
					continue
				} else {
					t.Error(err)
				}
			}
			value, err := event.GetFieldValue(field)
			if err != nil {
				t.Errorf("unable to get the expected `%s` value: %v", field, err)
			}
			switch v := value.(type) {
			case int:
				if v != 123 {
					t.Errorf("unable to get the expected `%s` value: %v", field, v)
				}
			case []int:
				if v[0] != 123 {
					t.Errorf("unable to get the expected `%s` value: %v", field, v)
				}
			default:
				t.Errorf("unable to get the expected `%s` value: %v", field, v)
			}
		case reflect.Bool:
			err = event.SetFieldValue(field, true)
			if err != nil {
				if errors.As(err, &readOnlyError) {
					continue
				} else {
					t.Error(err)
				}
			}
			value, err := event.GetFieldValue(field)
			if err != nil {
				t.Errorf("unable to get the expected `%s` value: %v", field, err)
			}
			switch v := value.(type) {
			case bool:
				if !v {
					t.Errorf("unable to get the expected `%s` value: %v", field, v)
				}
			case []bool:
				if !v[0] {
					t.Errorf("unable to get the expected `%s` value: %v", field, v)
				}
			default:
				t.Errorf("unable to get the expected `%s` value: %v", field, v)
			}
		case reflect.Struct:
			switch field {
			case "network.destination.ip", "network.source.ip":
				ip, ipnet, err := net.ParseCIDR("127.0.0.1/24")
				if err != nil {
					t.Error(err)
				}

				if err = event.SetFieldValue(field, *ipnet); err != nil {
					t.Error(err)
				}
				if value, err := event.GetFieldValue(field); err != nil || value.(net.IPNet).IP.Equal(ip) {
					t.Errorf("unable to get the expected value: %v", err)
				}
			}
		default:
			t.Errorf("type of field %s unknown: %v", field, kind)
		}
	}
}
