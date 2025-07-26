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
	sysconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config"
	"github.com/stretchr/testify/assert"
)

func TestStatusFundamentals(t *testing.T) {
	is, _ := newSoftwareInventory(t, true)
	assert.Equal(t, "Software Inventory Metadata", is.Name())
	assert.Equal(t, 4, is.Index())
}

func TestGetPayloadRefreshesCachedValues(t *testing.T) {
	mockData := []software.Entry{
		{DisplayName: "FooApp", ProductCode: "foo"},
		{DisplayName: "BarApp", ProductCode: "bar"},
	}
	is, sp := newSoftwareInventory(t, true)
	sp.On("GetCheck", sysconfig.SoftwareInventory).Return(mockData, nil)

	// Status JSON should trigger a refresh of cached values
	stats := make(map[string]interface{})
	err := is.JSON(false /* verbose */, stats)

	// Assert that the cached values were refreshed
	assert.NoError(t, err)
	assert.Len(t, stats, 1)
	assert.Contains(t, stats, "software_inventory_metadata")
	// Note: The exact structure of stats depends on how the JSON is marshaled
	// This test may need adjustment based on the actual output format
	sp.AssertNumberOfCalls(t, "GetCheck", 1)
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
			is, sp := newSoftwareInventory(t, true)
			sp.On("GetCheck", sysconfig.SoftwareInventory).Return(tt.mockData, nil)

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
			sp.AssertNumberOfCalls(t, "GetCheck", 1)
		})
	}
}

func TestStatusTemplateWithNoSoftwareInventoryMetadata(t *testing.T) {
	is, sp := newSoftwareInventory(t, true)
	sp.On("GetCheck", sysconfig.SoftwareInventory).Return([]software.Entry{}, nil)

	// Test Text template
	var buf bytes.Buffer
	err := is.Text(false, &buf)
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "No software inventory metadata - is System Probe running?")

	// Test HTML template
	buf.Reset()
	err = is.HTML(false, &buf)
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "No software inventory metadata - is System Probe running?")

	// The populateStatus caches the values once.
	sp.AssertNumberOfCalls(t, "GetCheck", 1)
}
