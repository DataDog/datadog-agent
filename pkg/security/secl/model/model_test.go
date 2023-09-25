// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package model holds model related files
package model

import (
	"errors"
	"net"
	"reflect"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
)

func TestPathValidation(t *testing.T) {
	mod := &Model{}

	var maxDepthPath string
	for i := 0; i <= MaxPathDepth; i++ {
		maxDepthPath += "a/"
	}

	var maxSegmentPath string
	for i := 0; i <= MaxSegmentLength; i++ {
		maxSegmentPath += "a"
	}

	tests := []struct {
		val            string
		errMessage     string
		fieldValueType eval.FieldValueType
	}{
		{
			val: "/var/log/*",
		},
		{
			val:        "~/apache/httpd.conf",
			errMessage: ErrPathMustBeAbsolute,
		},
		{
			val:        "../../../etc/apache/httpd.conf",
			errMessage: ErrPathMustBeAbsolute,
		},
		{
			val:        "/etc/apache/./httpd.conf",
			errMessage: ErrPathMustBeAbsolute,
		},
		{
			val:        "*/",
			errMessage: ErrPathMustBeAbsolute,
		},
		{
			val:        "~/",
			errMessage: ErrPathMustBeAbsolute,
		},
		{
			val:            "/run/..data",
			fieldValueType: eval.PatternValueType,
		},
		{
			val:            "./data",
			fieldValueType: eval.PatternValueType,
			errMessage:     ErrPathMustBeAbsolute,
		},
		{
			val:            "../data",
			fieldValueType: eval.PatternValueType,
			errMessage:     ErrPathMustBeAbsolute,
		},
		{
			val:            "/data/../a",
			fieldValueType: eval.PatternValueType,
			errMessage:     ErrPathMustBeAbsolute,
		},
		{
			val:            "*/data",
			fieldValueType: eval.PatternValueType,
		},
		{
			val:            ".*",
			fieldValueType: eval.RegexpValueType,
			errMessage:     ErrPathMustBeAbsolute,
		},
		{
			val:            "/etc/*",
			fieldValueType: eval.PatternValueType,
		},
		{
			val:            "*",
			fieldValueType: eval.PatternValueType,
			errMessage:     ErrPathMustBeAbsolute,
		},
		{
			val:        maxDepthPath,
			errMessage: ErrPathMustBeAbsolute,
		},
		{
			val:        maxSegmentPath,
			errMessage: ErrPathMustBeAbsolute,
		},
	}

	for _, test := range tests {
		err := mod.ValidateField("open.file.path", eval.FieldValue{Value: test.val})
		if err != nil && test.errMessage == "" {
			t.Errorf("shouldn't return an error: %s", err)
		}
		if err != nil && !strings.Contains(err.Error(), test.errMessage) {
			t.Errorf("Error message is `%s`, wanted it to contain `%s`", err.Error(), test.errMessage)
		}
	}
}

func TestSetFieldValue(t *testing.T) {
	var readOnlyError *eval.ErrFieldReadOnly
	var fieldNotSupportedError *eval.ErrNotSupported

	event := NewDefaultEvent()
	for _, field := range event.GetFields() {
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
			value, err := event.GetFieldValue(field)
			if err != nil {
				if errors.As(err, &fieldNotSupportedError) {
					continue
				} else {
					t.Errorf("unable to get the expected `%s` value: %v", field, err)
				}
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
				if errors.As(err, &fieldNotSupportedError) {
					continue
				} else {
					t.Errorf("unable to get the expected `%s` value: %v", field, err)
				}
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
				if errors.As(err, &fieldNotSupportedError) {
					continue
				} else {
					t.Errorf("unable to get the expected `%s` value: %v", field, err)
				}
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
