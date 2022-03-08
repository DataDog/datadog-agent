/*
Copyright 2021 The logr Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package logr

import (
	"context"
	"fmt"
	"reflect"
	"testing"
)

// testLogSink is a Logger just for testing that calls optional hooks on each method.
type testLogSink struct {
	fnInit       func(ri RuntimeInfo)
	fnEnabled    func(lvl int) bool
	fnInfo       func(lvl int, msg string, kv ...interface{})
	fnError      func(err error, msg string, kv ...interface{})
	fnWithValues func(kv ...interface{})
	fnWithName   func(name string)
}

var _ LogSink = &testLogSink{}

func (l *testLogSink) Init(ri RuntimeInfo) {
	if l.fnInit != nil {
		l.fnInit(ri)
	}
}

func (l *testLogSink) Enabled(lvl int) bool {
	if l.fnEnabled != nil {
		return l.fnEnabled(lvl)
	}
	return false
}

func (l *testLogSink) Info(lvl int, msg string, kv ...interface{}) {
	if l.fnInfo != nil {
		l.fnInfo(lvl, msg, kv...)
	}
}

func (l *testLogSink) Error(err error, msg string, kv ...interface{}) {
	if l.fnError != nil {
		l.fnError(err, msg, kv...)
	}
}

func (l *testLogSink) WithValues(kv ...interface{}) LogSink {
	if l.fnWithValues != nil {
		l.fnWithValues(kv...)
	}
	out := *l
	return &out
}

func (l *testLogSink) WithName(name string) LogSink {
	if l.fnWithName != nil {
		l.fnWithName(name)
	}
	out := *l
	return &out
}

type testCallDepthLogSink struct {
	testLogSink
	callDepth       int
	fnWithCallDepth func(depth int)
}

var _ CallDepthLogSink = &testCallDepthLogSink{}

func (l *testCallDepthLogSink) WithCallDepth(depth int) LogSink {
	if l.fnWithCallDepth != nil {
		l.fnWithCallDepth(depth)
	}
	out := *l
	out.callDepth += depth
	return &out
}

func TestNew(t *testing.T) {
	calledInit := 0

	sink := &testLogSink{}
	sink.fnInit = func(ri RuntimeInfo) {
		if ri.CallDepth != 1 {
			t.Errorf("expected runtimeInfo.CallDepth = 1, got %d", ri.CallDepth)
		}
		calledInit++
	}
	logger := New(sink)

	if logger.sink == nil {
		t.Errorf("expected sink to be set, got %v", logger.sink)
	}
	if calledInit != 1 {
		t.Errorf("expected sink.Init() to be called once, got %d", calledInit)
	}
	if _, ok := logger.sink.(CallDepthLogSink); ok {
		t.Errorf("expected conversion to CallDepthLogSink to fail")
	}
}

func TestNewCachesCallDepthInterface(t *testing.T) {
	sink := &testCallDepthLogSink{}
	logger := New(sink)

	if _, ok := logger.sink.(CallDepthLogSink); !ok {
		t.Errorf("expected conversion to CallDepthLogSink to succeed")
	}
}

func TestEnabled(t *testing.T) {
	calledEnabled := 0

	sink := &testLogSink{}
	sink.fnEnabled = func(lvl int) bool {
		calledEnabled++
		return true
	}
	logger := New(sink)

	if en := logger.Enabled(); en != true {
		t.Errorf("expected true")
	}
	if calledEnabled != 1 {
		t.Errorf("expected sink.Enabled() to be called once, got %d", calledEnabled)
	}
}

func TestError(t *testing.T) {
	calledError := 0
	errInput := fmt.Errorf("error")
	msgInput := "msg"
	kvInput := []interface{}{0, 1, 2}

	sink := &testLogSink{}
	sink.fnError = func(err error, msg string, kv ...interface{}) {
		calledError++
		if err != errInput {
			t.Errorf("unexpected err input, got %v", err)
		}
		if msg != msgInput {
			t.Errorf("unexpected msg input, got %q", msg)
		}
		if !reflect.DeepEqual(kv, kvInput) {
			t.Errorf("unexpected kv input, got %v", kv)
		}
	}
	logger := New(sink)

	logger.Error(errInput, msgInput, kvInput...)
	if calledError != 1 {
		t.Errorf("expected sink.Error() to be called once, got %d", calledError)
	}
}

func TestV(t *testing.T) {
	sink := &testLogSink{}
	logger := New(sink)

	if l := logger.V(0); l.level != 0 {
		t.Errorf("expected level 0, got %d", l.level)
	}
	if l := logger.V(93); l.level != 93 {
		t.Errorf("expected level 93, got %d", l.level)
	}
	if l := logger.V(70).V(6); l.level != 76 {
		t.Errorf("expected level 76, got %d", l.level)
	}
	if l := logger.V(-1); l.level != 0 {
		t.Errorf("expected level 0, got %d", l.level)
	}
	if l := logger.V(1).V(-1); l.level != 1 {
		t.Errorf("expected level 1, got %d", l.level)
	}
}

func TestInfo(t *testing.T) {
	calledEnabled := 0
	calledInfo := 0
	lvlInput := 0
	msgInput := "msg"
	kvInput := []interface{}{0, 1, 2}

	sink := &testLogSink{}
	sink.fnEnabled = func(lvl int) bool {
		calledEnabled++
		return lvl < 100
	}
	sink.fnInfo = func(lvl int, msg string, kv ...interface{}) {
		calledInfo++
		if lvl != lvlInput {
			t.Errorf("unexpected lvl input, got %v", lvl)
		}
		if msg != msgInput {
			t.Errorf("unexpected msg input, got %q", msg)
		}
		if !reflect.DeepEqual(kv, kvInput) {
			t.Errorf("unexpected kv input, got %v", kv)
		}
	}
	logger := New(sink)

	calledEnabled = 0
	calledInfo = 0
	lvlInput = 0
	logger.Info(msgInput, kvInput...)
	if calledEnabled != 1 {
		t.Errorf("expected sink.Enabled() to be called once, got %d", calledEnabled)
	}
	if calledInfo != 1 {
		t.Errorf("expected sink.Info() to be called once, got %d", calledInfo)
	}

	calledEnabled = 0
	calledInfo = 0
	lvlInput = 0
	logger.V(0).Info(msgInput, kvInput...)
	if calledEnabled != 1 {
		t.Errorf("expected sink.Enabled() to be called once, got %d", calledEnabled)
	}
	if calledInfo != 1 {
		t.Errorf("expected sink.Info() to be called once, got %d", calledInfo)
	}

	calledEnabled = 0
	calledInfo = 0
	lvlInput = 93
	logger.V(93).Info(msgInput, kvInput...)
	if calledEnabled != 1 {
		t.Errorf("expected sink.Enabled() to be called once, got %d", calledEnabled)
	}
	if calledInfo != 1 {
		t.Errorf("expected sink.Info() to be called once, got %d", calledInfo)
	}

	calledEnabled = 0
	calledInfo = 0
	lvlInput = 100
	logger.V(100).Info(msgInput, kvInput...)
	if calledEnabled != 1 {
		t.Errorf("expected sink.Enabled() to be called once, got %d", calledEnabled)
	}
	if calledInfo != 0 {
		t.Errorf("expected sink.Info() to not be called, got %d", calledInfo)
	}
}

func TestWithValues(t *testing.T) {
	calledWithValues := 0
	kvInput := []interface{}{"zero", 0, "one", 1, "two", 2}

	sink := &testLogSink{}
	sink.fnWithValues = func(kv ...interface{}) {
		calledWithValues++
		if !reflect.DeepEqual(kv, kvInput) {
			t.Errorf("unexpected kv input, got %v", kv)
		}
	}
	logger := New(sink)

	out := logger.WithValues(kvInput...)
	if calledWithValues != 1 {
		t.Errorf("expected sink.WithValues() to be called once, got %d", calledWithValues)
	}
	if p, _ := out.sink.(*testLogSink); p == sink {
		t.Errorf("expected output to be different from input, got in=%p, out=%p", sink, p)
	}
}

func TestWithName(t *testing.T) {
	calledWithName := 0
	nameInput := "name"

	sink := &testLogSink{}
	sink.fnWithName = func(name string) {
		calledWithName++
		if name != nameInput {
			t.Errorf("unexpected name input, got %q", name)
		}
	}
	logger := New(sink)

	out := logger.WithName(nameInput)
	if calledWithName != 1 {
		t.Errorf("expected sink.WithName() to be called once, got %d", calledWithName)
	}
	if p, _ := out.sink.(*testLogSink); p == sink {
		t.Errorf("expected output to be different from input, got in=%p, out=%p", sink, p)
	}
}

func TestWithCallDepthNotImplemented(t *testing.T) {
	depthInput := 7

	sink := &testLogSink{}
	logger := New(sink)

	out := logger.WithCallDepth(depthInput)
	if p, _ := out.sink.(*testLogSink); p != sink {
		t.Errorf("expected output to be the same as input, got in=%p, out=%p", sink, p)
	}
}

func TestWithCallDepthImplemented(t *testing.T) {
	calledWithCallDepth := 0
	depthInput := 7

	sink := &testCallDepthLogSink{}
	sink.fnWithCallDepth = func(depth int) {
		calledWithCallDepth++
		if depth != depthInput {
			t.Errorf("unexpected depth input, got %d", depth)
		}
	}
	logger := New(sink)

	out := logger.WithCallDepth(depthInput)
	if calledWithCallDepth != 1 {
		t.Errorf("expected sink.WithCallDepth() to be called once, got %d", calledWithCallDepth)
	}
	p, _ := out.sink.(*testCallDepthLogSink)
	if p == sink {
		t.Errorf("expected output to be different from input, got in=%p, out=%p", sink, p)
	}
	if p.callDepth != depthInput {
		t.Errorf("expected sink to have call depth %d, got %d", depthInput, p.callDepth)
	}
}

func TestWithCallDepthIncremental(t *testing.T) {
	calledWithCallDepth := 0
	depthInput := 7

	sink := &testCallDepthLogSink{}
	sink.fnWithCallDepth = func(depth int) {
		calledWithCallDepth++
		if depth != 1 {
			t.Errorf("unexpected depth input, got %d", depth)
		}
	}
	logger := New(sink)

	out := logger
	for i := 0; i < depthInput; i++ {
		out = out.WithCallDepth(1)
	}
	if calledWithCallDepth != depthInput {
		t.Errorf("expected sink.WithCallDepth() to be called %d times, got %d", depthInput, calledWithCallDepth)
	}
	p, _ := out.sink.(*testCallDepthLogSink)
	if p == sink {
		t.Errorf("expected output to be different from input, got in=%p, out=%p", sink, p)
	}
	if p.callDepth != depthInput {
		t.Errorf("expected sink to have call depth %d, got %d", depthInput, p.callDepth)
	}
}

func TestContext(t *testing.T) {
	ctx := context.TODO()

	if out, err := FromContext(ctx); err == nil {
		t.Errorf("expected error, got %#v", out)
	} else if _, ok := err.(notFoundError); !ok {
		t.Errorf("expected a notFoundError, got %#v", err)
	}

	out := FromContextOrDiscard(ctx)
	if _, ok := out.sink.(discardLogSink); !ok {
		t.Errorf("expected a discardLogSink, got %#v", out)
	}

	sink := &testLogSink{}
	logger := New(sink)
	lctx := NewContext(ctx, logger)
	if out, err := FromContext(lctx); err != nil {
		t.Errorf("unexpected error: %v", err)
	} else if p, _ := out.sink.(*testLogSink); p != sink {
		t.Errorf("expected output to be the same as input, got in=%p, out=%p", sink, p)
	}
	out = FromContextOrDiscard(lctx)
	if p, _ := out.sink.(*testLogSink); p != sink {
		t.Errorf("expected output to be the same as input, got in=%p, out=%p", sink, p)
	}
}
