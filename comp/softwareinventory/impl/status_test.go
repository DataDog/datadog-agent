// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package softwareinventoryimpl

import (
	"bytes"
	"regexp"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/inventory/software"
	"github.com/stretchr/testify/assert"
)

func TestStatusFundamentals(t *testing.T) {
	f := newFixtureWithData(t, true, nil)
	is := f.sut()

	assert.Equal(t, "Software Inventory Metadata", is.Name())
	assert.Equal(t, 4, is.Index())
}

func TestGetPayloadRefreshesCachedValues(t *testing.T) {
	f := newFixtureWithData(t, true, []software.Entry{
		{DisplayName: "FooApp", ProductCode: "foo", Source: "app"},
		{DisplayName: "BarApp", ProductCode: "bar", Source: "pkg"},
	})
	is := f.sut().WaitForSystemProbe()

	// Status JSON should trigger a refresh of cached values
	stats := make(map[string]interface{})
	err := is.JSON(false /* verbose */, stats)

	// Assert that the cached values were refreshed
	assert.NoError(t, err)
	// Now includes: software_inventory_metadata, software_inventory_stats, software_inventory_total
	assert.Contains(t, stats, "software_inventory_metadata")
	assert.Contains(t, stats, "software_inventory_stats")
	assert.Contains(t, stats, "software_inventory_total")
	assert.Equal(t, 2, stats["software_inventory_total"])
	// Note: The exact structure of stats depends on how the JSON is marshaled
	// This test may need adjustment based on the actual output format
	f.sysProbeClient.AssertNumberOfCalls(t, "GetCheck", 1)
}

func TestStatusTemplates(t *testing.T) {
	tests := []struct {
		name     string
		mockData []software.Entry
		wantText string
		wantHTML []string
	}{
		{
			name: "software with display name",
			mockData: []software.Entry{
				{DisplayName: "TestApp", Version: "1.0", ProductCode: "test"},
			},
			wantText: "Detected 1 installed software entries",
			wantHTML: []string{
				`<summary>\s*TestApp 1\.0\s*</summary>`,
				`<li><strong>Display Name:</strong>\s*TestApp\s*</li>`,
				`<li><strong>Version:</strong>\s*1\.0\s*</li>`,
				`<li><strong>Product code:</strong>\s*test\s*</li>`,
			},
		},
		{
			name: "software without display name",
			mockData: []software.Entry{
				{Version: "2.0", ProductCode: "test-product"},
			},
			wantText: "Detected 1 installed software entries",
			wantHTML: []string{
				`<summary>\s*test-product\s*</summary>`,
				`<li><strong>Version:</strong>\s*2\.0\s*</li>`,
				`<li><strong>Product code:</strong>\s*test-product\s*</li>`,
			},
		},
		{
			name: "empty display name",
			mockData: []software.Entry{
				{DisplayName: "", Publisher: "Test Corp", ProductCode: "test-empty"},
			},
			wantText: "Detected 1 installed software entries",
			wantHTML: []string{
				`<summary>\s*test-empty\s*</summary>`,
				`<li><strong>Publisher:</strong>\s*Test Corp\s*</li>`,
				`<li><strong>Product code:</strong>\s*test-empty\s*</li>`,
			},
		},
		{
			name: "multiple software entries",
			mockData: []software.Entry{
				{DisplayName: "App One", Version: "1.0", ProductCode: "product1"},
				{Version: "2.0", ProductCode: "product2"},
			},
			wantText: "Detected 2 installed software entries",
			wantHTML: []string{
				`<summary>\s*App One 1\.0\s*</summary>`,
				`<li><strong>Display Name:</strong>\s*App One\s*</li>`,
				`<li><strong>Version:</strong>\s*1\.0\s*</li>`,
				`<li><strong>Product code:</strong>\s*product1\s*</li>`,
				`<summary>\s*product2\s*</summary>`,
				`<li><strong>Version:</strong>\s*2\.0\s*</li>`,
				`<li><strong>Product code:</strong>\s*product2\s*</li>`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixtureWithData(t, true, tt.mockData)
			is := f.sut().WaitForSystemProbe()

			// Test Text template
			var buf bytes.Buffer
			err := is.Text(false, &buf)
			assert.NoError(t, err)
			assert.Contains(t, buf.String(), tt.wantText)

			// Test HTML template
			buf.Reset()
			err = is.HTML(false, &buf)
			assert.NoError(t, err)
			html := buf.String()
			for _, want := range tt.wantHTML {
				assert.Regexp(t, regexp.MustCompile(want), html)
			}

			// Verify that we only call GetCheck once per test case
			f.sysProbeClient.AssertNumberOfCalls(t, "GetCheck", 1)
		})
	}
}

func TestStatusTemplateWithNoSoftwareInventoryMetadata(t *testing.T) {
	f := newFixtureWithData(t, true, []software.Entry{})
	is := f.sut().WaitForSystemProbe()

	// Test Text template
	var buf bytes.Buffer
	err := is.Text(false, &buf)
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "Detected 0 installed software entries")

	// Test HTML template
	buf.Reset()
	err = is.HTML(false, &buf)
	assert.NoError(t, err)
	// Just verify the basic structure is present, since it will be empty
	assert.Contains(t, buf.String(), `class="stat_data inventory-scrollbox"`)
	// Summary should show 0 entries
	assert.Contains(t, buf.String(), "<strong>Summary:</strong> 0 entries")

	// The populateStatus caches the values once.
	f.sysProbeClient.AssertNumberOfCalls(t, "GetCheck", 1)
}

func TestStatusStatsComputation(t *testing.T) {
	// Test that stats are correctly computed by software type
	f := newFixtureWithData(t, true, []software.Entry{
		{DisplayName: "Safari", ProductCode: "safari", Source: "app"},
		{DisplayName: "Chrome", ProductCode: "chrome", Source: "app"},
		{DisplayName: "git", ProductCode: "git", Source: "homebrew"},
		{DisplayName: "wget", ProductCode: "wget", Source: "homebrew"},
		{DisplayName: "curl", ProductCode: "curl", Source: "homebrew"},
		{DisplayName: "Python Driver", ProductCode: "python-driver", Source: "pkg"},
	})
	is := f.sut().WaitForSystemProbe()

	stats := make(map[string]interface{})
	err := is.JSON(false, stats)
	assert.NoError(t, err)

	// Verify total count
	assert.Equal(t, 6, stats["software_inventory_total"])

	// Verify stats by type
	typeStats, ok := stats["software_inventory_stats"].(map[string]int)
	assert.True(t, ok, "software_inventory_stats should be map[string]int")
	assert.Equal(t, 2, typeStats["app"])
	assert.Equal(t, 3, typeStats["homebrew"])
	assert.Equal(t, 1, typeStats["pkg"])

	// Verify no broken count when no broken entries
	_, hasBroken := stats["software_inventory_broken"]
	assert.False(t, hasBroken, "software_inventory_broken should not be present when no broken entries")
}

func TestStatusBrokenCount(t *testing.T) {
	// Test that broken entries are correctly counted
	f := newFixtureWithData(t, true, []software.Entry{
		{DisplayName: "GoodApp", ProductCode: "good", Source: "app", Status: "installed"},
		{DisplayName: "BrokenApp", ProductCode: "broken1", Source: "app", Status: "broken"},
		{DisplayName: "AnotherGood", ProductCode: "good2", Source: "pkg", Status: "installed"},
		{DisplayName: "AnotherBroken", ProductCode: "broken2", Source: "pkg", Status: "broken"},
	})
	is := f.sut().WaitForSystemProbe()

	stats := make(map[string]interface{})
	err := is.JSON(false, stats)
	assert.NoError(t, err)

	// Verify total count
	assert.Equal(t, 4, stats["software_inventory_total"])

	// Verify broken count is present and correct
	brokenCount, hasBroken := stats["software_inventory_broken"]
	assert.True(t, hasBroken, "software_inventory_broken should be present when there are broken entries")
	assert.Equal(t, 2, brokenCount)
}

func TestStatusTextTemplateWithStats(t *testing.T) {
	f := newFixtureWithData(t, true, []software.Entry{
		{DisplayName: "App1", ProductCode: "app1", Source: "app"},
		{DisplayName: "App2", ProductCode: "app2", Source: "app"},
		{DisplayName: "Brew1", ProductCode: "brew1", Source: "homebrew"},
	})
	is := f.sut().WaitForSystemProbe()

	var buf bytes.Buffer
	err := is.Text(false, &buf)
	assert.NoError(t, err)

	text := buf.String()
	// Verify total count in text output
	assert.Contains(t, text, "Detected 3 installed software entries")
	// Verify "By type:" section header
	assert.Contains(t, text, "By type:")
}

func TestStatusTextTemplateWithBrokenEntries(t *testing.T) {
	f := newFixtureWithData(t, true, []software.Entry{
		{DisplayName: "GoodApp", ProductCode: "good", Source: "app", Status: "installed"},
		{DisplayName: "BrokenApp", ProductCode: "broken", Source: "app", Status: "broken"},
	})
	is := f.sut().WaitForSystemProbe()

	var buf bytes.Buffer
	err := is.Text(false, &buf)
	assert.NoError(t, err)

	text := buf.String()
	// Verify broken count appears in text output
	assert.Contains(t, text, "Detected 2 installed software entries (1 broken)")
}

func TestStatusHTMLTemplateWithStats(t *testing.T) {
	f := newFixtureWithData(t, true, []software.Entry{
		{DisplayName: "App1", ProductCode: "app1", Source: "app"},
		{DisplayName: "Brew1", ProductCode: "brew1", Source: "homebrew"},
	})
	is := f.sut().WaitForSystemProbe()

	var buf bytes.Buffer
	err := is.HTML(false, &buf)
	assert.NoError(t, err)

	html := buf.String()
	// Verify summary section
	assert.Contains(t, html, "<strong>Summary:</strong> 2 entries")
	// Verify "By type:" section
	assert.Contains(t, html, "<strong>By type:</strong>")
}
