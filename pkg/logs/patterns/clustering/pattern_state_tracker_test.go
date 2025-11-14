// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clustering

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/logs/patterns/token"
)

// Test-only helper functions

// wasSent returns true if this pattern has been sent at least once.
func wasSent(p *Pattern) bool {
	if p == nil {
		return false
	}
	return !p.LastSentAt.IsZero()
}

// templateChanged returns true if the template has changed since last send.
func templateChanged(p *Pattern) bool {
	if p == nil {
		return false
	}
	if p.LastSentAt.IsZero() {
		return false // Never sent, so no baseline to compare
	}
	currentTemplate := p.GetPatternString()
	return p.SentTemplate != currentTemplate
}

// getSentTemplate returns the template that was last sent.
// Returns empty string if never sent.
func getSentTemplate(p *Pattern) string {
	if p == nil {
		return ""
	}
	return p.SentTemplate
}

// TestNeedsResend_NeverSent tests that a pattern that has never been sent needs sending as PatternDefine
func TestNeedsResend_NeverSent(t *testing.T) {
	tl := token.NewTokenList()
	tl.Add(token.NewToken(token.TokenWord, "Service", token.NotWildcard))
	tl.Add(token.NewToken(token.TokenWord, " ", token.NotWildcard))
	tl.Add(token.NewToken(token.TokenWord, "started", token.PotentialWildcard))

	pattern := newPattern(tl, 12345)

	needsSend, templateState := pattern.NeedsResend()
	assert.True(t, needsSend, "Pattern should need sending")
	assert.Equal(t, TemplateIsNew, templateState, "Should be TemplateIsNew for first send")
}

// TestNeedsResend_AlreadySent_NoChange tests that a sent pattern with no changes doesn't need resending
func TestNeedsResend_AlreadySent_NoChange(t *testing.T) {
	tl := token.NewTokenList()
	tl.Add(token.NewToken(token.TokenWord, "Service", token.NotWildcard))
	tl.Add(token.NewToken(token.TokenWord, " ", token.NotWildcard))
	tl.Add(token.NewToken(token.TokenWord, "started", token.PotentialWildcard))

	pattern := newPattern(tl, 12345)
	pattern.MarkAsSent()

	needsSend, templateState := pattern.NeedsResend()
	assert.False(t, needsSend, "Pattern should not need sending")
	assert.Equal(t, TemplateNotNeeded, templateState, "Should be TemplateNotNeeded")
}

// TestNeedsResend_TemplateChanged tests that a pattern with changed template needs sending as PatternUpdate
func TestNeedsResend_TemplateChanged(t *testing.T) {
	// Create initial pattern
	tl := token.NewTokenList()
	tl.Add(token.NewToken(token.TokenWord, "Service", token.NotWildcard))
	tl.Add(token.NewToken(token.TokenWord, " ", token.NotWildcard))
	tl.Add(token.NewToken(token.TokenWord, "started", token.PotentialWildcard))

	pattern := newPattern(tl, 12345)
	pattern.MarkAsSent()

	// Simulate template evolution (add wildcard)
	template := token.NewTokenList()
	template.Add(token.NewToken(token.TokenWord, "Service", token.NotWildcard))
	template.Add(token.NewToken(token.TokenWord, " ", token.NotWildcard))
	template.Add(token.NewToken(token.TokenWord, "value", token.IsWildcard))

	pattern.Template = template
	pattern.Positions = []int{2}
	pattern.UpdatedAt = time.Now()

	needsSend, templateState := pattern.NeedsResend()
	assert.True(t, needsSend, "Pattern should need sending after template change")
	assert.Equal(t, TemplateChanged, templateState, "Should be TemplateChanged for template change")
}

// TestNeedsResend_NilPattern tests that a nil pattern doesn't need sending
func TestNeedsResend_NilPattern(t *testing.T) {
	var pattern *Pattern = nil

	needsSend, templateState := pattern.NeedsResend()
	assert.False(t, needsSend, "Nil pattern should not need sending")
	assert.Equal(t, TemplateNotNeeded, templateState, "Should be TemplateNotNeeded for nil")
}

// TestMarkAsSent_UpdatesTimestampAndTemplate tests that MarkAsSent properly records state
func TestMarkAsSent_UpdatesTimestampAndTemplate(t *testing.T) {
	tl := token.NewTokenList()
	tl.Add(token.NewToken(token.TokenWord, "Service", token.NotWildcard))
	tl.Add(token.NewToken(token.TokenWord, " ", token.NotWildcard))
	tl.Add(token.NewToken(token.TokenWord, "started", token.PotentialWildcard))

	pattern := newPattern(tl, 12345)
	assert.True(t, pattern.LastSentAt.IsZero(), "LastSentAt should be zero initially")
	assert.Equal(t, "", pattern.SentTemplate, "SentTemplate should be empty initially")

	pattern.MarkAsSent()

	assert.False(t, pattern.LastSentAt.IsZero(), "LastSentAt should be set")
	assert.Equal(t, "Service started", pattern.SentTemplate, "SentTemplate should match current template")
}

// TestMarkAsSent_NilPattern tests that MarkAsSent handles nil gracefully
func TestMarkAsSent_NilPattern(t *testing.T) {
	var pattern *Pattern = nil
	// Should not panic
	pattern.MarkAsSent()
}

// TestWasSent_NeverSent tests that WasSent returns false for unsent patterns
func TestWasSent_NeverSent(t *testing.T) {
	tl := token.NewTokenList()
	tl.Add(token.NewToken(token.TokenWord, "Test", token.NotWildcard))

	pattern := newPattern(tl, 12345)
	assert.False(t, wasSent(pattern), "wasSent should return false for new pattern")
}

// TestWasSent_AfterSend tests that WasSent returns true after sending
func TestWasSent_AfterSend(t *testing.T) {
	tl := token.NewTokenList()
	tl.Add(token.NewToken(token.TokenWord, "Test", token.NotWildcard))

	pattern := newPattern(tl, 12345)
	pattern.MarkAsSent()

	assert.True(t, wasSent(pattern), "wasSent should return true after MarkAsSent")
}

// TestWasSent_NilPattern tests that WasSent handles nil gracefully
func TestWasSent_NilPattern(t *testing.T) {
	var pattern *Pattern = nil
	assert.False(t, wasSent(pattern), "wasSent should return false for nil pattern")
}

// TestTemplateChanged_NeverSent tests that TemplateChanged returns false for unsent patterns
func TestTemplateChanged_NeverSent(t *testing.T) {
	tl := token.NewTokenList()
	tl.Add(token.NewToken(token.TokenWord, "Test", token.NotWildcard))

	pattern := newPattern(tl, 12345)
	assert.False(t, templateChanged(pattern), "templateChanged should return false if never sent")
}

// TestTemplateChanged_NoChange tests that TemplateChanged returns false when template hasn't changed
func TestTemplateChanged_NoChange(t *testing.T) {
	tl := token.NewTokenList()
	tl.Add(token.NewToken(token.TokenWord, "Test", token.NotWildcard))

	pattern := newPattern(tl, 12345)
	pattern.MarkAsSent()

	assert.False(t, templateChanged(pattern), "templateChanged should return false when unchanged")
}

// TestTemplateChanged_Changed tests that TemplateChanged returns true when template changed
func TestTemplateChanged_Changed(t *testing.T) {
	tl := token.NewTokenList()
	tl.Add(token.NewToken(token.TokenWord, "Service", token.NotWildcard))
	tl.Add(token.NewToken(token.TokenWord, " ", token.NotWildcard))
	tl.Add(token.NewToken(token.TokenWord, "started", token.PotentialWildcard))

	pattern := newPattern(tl, 12345)
	pattern.MarkAsSent()

	// Change template
	template := token.NewTokenList()
	template.Add(token.NewToken(token.TokenWord, "Service", token.NotWildcard))
	template.Add(token.NewToken(token.TokenWord, " ", token.NotWildcard))
	template.Add(token.NewToken(token.TokenWord, "value", token.IsWildcard))
	pattern.Template = template
	pattern.Positions = []int{2}

	assert.True(t, templateChanged(pattern), "templateChanged should return true when template changed")
}

// TestTemplateChanged_NilPattern tests that TemplateChanged handles nil gracefully
func TestTemplateChanged_NilPattern(t *testing.T) {
	var pattern *Pattern = nil
	assert.False(t, templateChanged(pattern), "templateChanged should return false for nil pattern")
}

// TestGetSentTemplate_NeverSent tests that GetSentTemplate returns empty for unsent patterns
func TestGetSentTemplate_NeverSent(t *testing.T) {
	tl := token.NewTokenList()
	tl.Add(token.NewToken(token.TokenWord, "Test", token.NotWildcard))

	pattern := newPattern(tl, 12345)
	assert.Equal(t, "", getSentTemplate(pattern), "getSentTemplate should return empty for new pattern")
}

// TestGetSentTemplate_AfterSend tests that GetSentTemplate returns the sent template
func TestGetSentTemplate_AfterSend(t *testing.T) {
	tl := token.NewTokenList()
	tl.Add(token.NewToken(token.TokenWord, "Service", token.NotWildcard))
	tl.Add(token.NewToken(token.TokenWord, " ", token.NotWildcard))
	tl.Add(token.NewToken(token.TokenWord, "started", token.PotentialWildcard))

	pattern := newPattern(tl, 12345)
	pattern.MarkAsSent()

	assert.Equal(t, "Service started", getSentTemplate(pattern), "getSentTemplate should return sent template")
}

// TestGetSentTemplate_NilPattern tests that GetSentTemplate handles nil gracefully
func TestGetSentTemplate_NilPattern(t *testing.T) {
	var pattern *Pattern = nil
	assert.Equal(t, "", getSentTemplate(pattern), "getSentTemplate should return empty for nil pattern")
}

// TestPatternLifecycle_FullFlow tests the complete pattern lifecycle
func TestPatternLifecycle_FullFlow(t *testing.T) {
	// 1. Create new pattern
	tl := token.NewTokenList()
	tl.Add(token.NewToken(token.TokenWord, "Service", token.NotWildcard))
	tl.Add(token.NewToken(token.TokenWord, " ", token.NotWildcard))
	tl.Add(token.NewToken(token.TokenWord, "started", token.PotentialWildcard))

	pattern := newPattern(tl, 12345)

	// 2. Check initial state - needs Define
	needsSend, templateState := pattern.NeedsResend()
	assert.True(t, needsSend)
	assert.Equal(t, TemplateIsNew, templateState)
	assert.False(t, wasSent(pattern))
	assert.Equal(t, "", getSentTemplate(pattern))

	// 3. Mark as sent
	pattern.MarkAsSent()
	assert.True(t, wasSent(pattern))
	assert.Equal(t, "Service started", getSentTemplate(pattern))

	// 4. Check no resend needed
	needsSend, templateState = pattern.NeedsResend()
	assert.False(t, needsSend)
	assert.Equal(t, TemplateNotNeeded, templateState)

	// 5. Simulate template evolution
	time.Sleep(1 * time.Millisecond)
	template := token.NewTokenList()
	template.Add(token.NewToken(token.TokenWord, "Service", token.NotWildcard))
	template.Add(token.NewToken(token.TokenWord, " ", token.NotWildcard))
	template.Add(token.NewToken(token.TokenWord, "value", token.IsWildcard))
	pattern.Template = template
	pattern.Positions = []int{2}
	pattern.UpdatedAt = time.Now()

	// 6. Check needs Update
	assert.True(t, templateChanged(pattern))
	needsSend, templateState = pattern.NeedsResend()
	assert.True(t, needsSend)
	assert.Equal(t, TemplateChanged, templateState)

	// 7. Mark as sent again
	pattern.MarkAsSent()
	assert.Equal(t, "Service *", getSentTemplate(pattern))

	// 8. Check no resend needed again
	needsSend, templateState = pattern.NeedsResend()
	assert.False(t, needsSend)
	assert.Equal(t, TemplateNotNeeded, templateState)
}

// TestPatternLifecycle_MultipleUpdates tests multiple template updates
func TestPatternLifecycle_MultipleUpdates(t *testing.T) {
	// Initial pattern: "Service started"
	tl := token.NewTokenList()
	tl.Add(token.NewToken(token.TokenWord, "Service", token.NotWildcard))
	tl.Add(token.NewToken(token.TokenWord, " ", token.NotWildcard))
	tl.Add(token.NewToken(token.TokenWord, "started", token.PotentialWildcard))

	pattern := newPattern(tl, 12345)
	pattern.MarkAsSent()

	// First update: "Service *"
	time.Sleep(1 * time.Millisecond)
	template1 := token.NewTokenList()
	template1.Add(token.NewToken(token.TokenWord, "Service", token.NotWildcard))
	template1.Add(token.NewToken(token.TokenWord, " ", token.NotWildcard))
	template1.Add(token.NewToken(token.TokenWord, "value", token.IsWildcard))
	pattern.Template = template1
	pattern.Positions = []int{2}
	pattern.UpdatedAt = time.Now()

	needsSend, templateState := pattern.NeedsResend()
	assert.True(t, needsSend)
	assert.Equal(t, TemplateChanged, templateState)
	pattern.MarkAsSent()

	// Second update: "* *"
	time.Sleep(1 * time.Millisecond)
	template2 := token.NewTokenList()
	template2.Add(token.NewToken(token.TokenWord, "value1", token.IsWildcard))
	template2.Add(token.NewToken(token.TokenWord, " ", token.NotWildcard))
	template2.Add(token.NewToken(token.TokenWord, "value2", token.IsWildcard))
	pattern.Template = template2
	pattern.Positions = []int{0, 2}
	pattern.UpdatedAt = time.Now()

	needsSend, templateState = pattern.NeedsResend()
	assert.True(t, needsSend)
	assert.Equal(t, TemplateChanged, templateState)
	pattern.MarkAsSent()

	assert.Equal(t, "* *", getSentTemplate(pattern))
}

// TestPatternStateTracker_EdgeCases tests various edge cases
func TestPatternStateTracker_EdgeCases(t *testing.T) {
	t.Run("EmptyTemplate", func(t *testing.T) {
		tl := token.NewTokenList()
		pattern := newPattern(tl, 12345)

		needsSend, templateState := pattern.NeedsResend()
		assert.True(t, needsSend)
		assert.Equal(t, TemplateIsNew, templateState)

		pattern.MarkAsSent()
		assert.Equal(t, "", getSentTemplate(pattern))

		needsSend, templateState = pattern.NeedsResend()
		assert.False(t, needsSend)
		assert.Equal(t, TemplateNotNeeded, templateState)
	})

	t.Run("OnlyWildcards", func(t *testing.T) {
		tl := token.NewTokenList()
		tl.Add(token.NewToken(token.TokenWord, "value1", token.IsWildcard))
		tl.Add(token.NewToken(token.TokenWord, " ", token.NotWildcard))
		tl.Add(token.NewToken(token.TokenWord, "value2", token.IsWildcard))

		pattern := newPattern(tl, 12345)
		pattern.Positions = []int{0, 2}
		pattern.MarkAsSent()

		assert.Equal(t, "* *", getSentTemplate(pattern))
		assert.False(t, templateChanged(pattern))
	})

	t.Run("TemplateBecomesNil", func(t *testing.T) {
		tl := token.NewTokenList()
		tl.Add(token.NewToken(token.TokenWord, "Test", token.NotWildcard))

		pattern := newPattern(tl, 12345)
		pattern.MarkAsSent()
		assert.Equal(t, "Test", getSentTemplate(pattern))

		// Simulate template becoming nil (edge case)
		pattern.Template = nil
		needsSend, templateState := pattern.NeedsResend()
		assert.True(t, needsSend)
		assert.Equal(t, TemplateChanged, templateState)
	})
}
