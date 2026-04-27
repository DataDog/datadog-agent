// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package processors

import (
	"log/slog"
	"testing"
	"time"
)

func TestNoop_PassThrough(t *testing.T) {
	p := Noop()

	r := slog.NewRecord(time.Now(), slog.LevelError, "boom", 0)
	r.AddAttrs(slog.String("k", "v"))

	out := p.Process(&r)
	if out != &r {
		t.Fatalf("Noop should return the same pointer; got %p, want %p", out, &r)
	}
	if out.Message != "boom" {
		t.Fatalf("Noop must not mutate Message; got %q want %q", out.Message, "boom")
	}
	if out.Level != slog.LevelError {
		t.Fatalf("Noop must not mutate Level; got %v want %v", out.Level, slog.LevelError)
	}

	// Verify attrs untouched.
	var keys []string
	out.Attrs(func(a slog.Attr) bool {
		keys = append(keys, a.Key)
		return true
	})
	if len(keys) != 1 || keys[0] != "k" {
		t.Fatalf("Noop must not mutate attrs; got keys=%v want=[k]", keys)
	}
}
