// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package model

import (
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
