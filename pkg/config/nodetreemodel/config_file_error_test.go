// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package nodetreemodel

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

// sevenLineYAML is addressed by line number in the tests below.
var sevenLineYAML = []byte("l1: a\nl2: b\nl3: c\nl4: d\nl5: e\nl6: f\nl7: g\n")

func TestFormatYAMLErrorContextMarksOffendingLine(t *testing.T) {
	got := formatYAMLErrorContext(sevenLineYAML, errors.New("yaml: line 4: did not find expected key"))

	assert.Contains(t, got, "> 4 | l4: d", "offending line must be marked")
	assert.Contains(t, got, "  2 | l2: b")
	assert.Contains(t, got, "  6 | l6: f")
	assert.NotContains(t, got, "l1: a", "context is limited to two lines either side")
	assert.NotContains(t, got, "l7: g")
}

func TestFormatYAMLErrorContextClampsAtStartOfFile(t *testing.T) {
	got := formatYAMLErrorContext(sevenLineYAML, errors.New("yaml: line 1: did not find expected key"))

	assert.Contains(t, got, "> 1 | l1: a")
	assert.Contains(t, got, "  3 | l3: c")
	assert.NotContains(t, got, "l4: d")
}

func TestFormatYAMLErrorContextClampsAtEndOfFile(t *testing.T) {
	got := formatYAMLErrorContext(sevenLineYAML, errors.New("yaml: line 7: did not find expected key"))

	assert.Contains(t, got, "> 7 | l7: g")
	assert.Contains(t, got, "  5 | l5: e")
	assert.NotContains(t, got, "l4: d")
}

func TestFormatYAMLErrorContextWithoutLineNumber(t *testing.T) {
	got := formatYAMLErrorContext(sevenLineYAML, errors.New("yaml: could not determine encoding"))

	assert.Empty(t, got)
}

func TestFormatYAMLErrorContextLineBeyondEndOfFile(t *testing.T) {
	got := formatYAMLErrorContext(sevenLineYAML, errors.New("yaml: line 99: did not find expected key"))

	assert.Empty(t, got)
}

// TestFormatYAMLErrorContextHandlesCRLF covers Notepad-edited files, which is
// how this class of breakage usually arrives.
func TestFormatYAMLErrorContextHandlesCRLF(t *testing.T) {
	got := formatYAMLErrorContext([]byte("l1: a\r\nl2: b\r\nl3: c\r\n"),
		errors.New("yaml: line 2: did not find expected key"))

	assert.Contains(t, got, "> 2 | l2: b")
	assert.NotContains(t, got, "\r", "carriage returns must not reach the message")
}

// TestFormatYAMLErrorContextAlignsLineNumbers guards readability when the
// offending line sits either side of a digit-width change.
func TestFormatYAMLErrorContextAlignsLineNumbers(t *testing.T) {
	content := []byte("l1\nl2\nl3\nl4\nl5\nl6\nl7\nl8\nl9\nl10\nl11\nl12\n")

	got := formatYAMLErrorContext(content, errors.New("yaml: line 10: did not find expected key"))

	assert.Contains(t, got, "> 10 | l10")
	assert.Contains(t, got, "   8 | l8")
	assert.Contains(t, got, "  12 | l12")
}
