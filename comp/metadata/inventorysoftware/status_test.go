// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package inventorysoftware

import (
	"bytes"
	"regexp"
	"testing"

	sysconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config"
	"github.com/stretchr/testify/assert"
)

func TestStatusFundamentals(t *testing.T) {
	is, _ := newInventorySoftware(t, nil)
	assert.Equal(t, "Software Inventory Metadata", is.Name())
	assert.Equal(t, 4, is.Index())
}

func TestGetPayloadRefreshesCachedValues(t *testing.T) {
	mockData := SoftwareInventoryMap{
		"foo": {"DisplayName": "FooApp"},
		"bar": {"DisplayName": "BarApp"},
	}
	is, sp := newInventorySoftware(t, nil)
	sp.On("GetCheck", sysconfig.InventorySoftwareModule).Return(mockData, nil)

	// Status JSON should trigger a refresh of cached values
	stats := make(map[string]interface{})
	err := is.JSON(false /* verbose */, stats)

	// Assert that the cached values were refreshed
	assert.NoError(t, err)
	assert.Len(t, stats, 1)
	assert.Contains(t, stats, "software_inventory_metadata")
	assert.Equal(t, "FooApp", stats["software_inventory_metadata"].(map[string]interface{})["foo"].(map[string]string)["DisplayName"])
	assert.Equal(t, "BarApp", stats["software_inventory_metadata"].(map[string]interface{})["bar"].(map[string]string)["DisplayName"])
	sp.AssertNumberOfCalls(t, "GetCheck", 1)
}

func TestStatusTemplates(t *testing.T) {
	tests := []struct {
		name     string
		mockData SoftwareInventoryMap
		wantText string
		wantHTML []string
	}{
		{
			name: "software with display name",
			mockData: SoftwareInventoryMap{
				"test": {"DisplayName": "TestApp", "Version": "1.0", "ProductCode": "test"},
			},
			wantText: "Detected 1 installed software entries",
			wantHTML: []string{
				`<summary>\s*TestApp\s*</summary>`,
				`<li><strong>ProductCode:</strong>\s*test\s*</li>`,
				`<li><strong>Version:</strong>\s*1\.0\s*</li>`,
			},
		},
		{
			name: "software without display name",
			mockData: SoftwareInventoryMap{
				"test-product": {"Version": "2.0", "ProductCode": "test-product"},
			},
			wantText: "Detected 1 installed software entries",
			wantHTML: []string{
				`<summary>\s*test-product\s*</summary>`,
				`<li><strong>ProductCode:</strong>\s*test-product\s*</li>`,
				`<li><strong>Version:</strong>\s*2\.0\s*</li>`,
			},
		},
		{
			name: "empty display name",
			mockData: SoftwareInventoryMap{
				"test-empty": {"DisplayName": "", "Publisher": "Test Corp", "ProductCode": "test-empty"},
			},
			wantText: "Detected 1 installed software entries",
			wantHTML: []string{
				`<summary>\s*test-empty\s*</summary>`,
				`<li><strong>ProductCode:</strong>\s*test-empty\s*</li>`,
				`<li><strong>Publisher:</strong>\s*Test Corp\s*</li>`,
			},
		},
		{
			name: "multiple software entries",
			mockData: SoftwareInventoryMap{
				"product1": {"DisplayName": "App One", "Version": "1.0", "ProductCode": "product1"},
				"product2": {"Version": "2.0", "ProductCode": "product2"},
			},
			wantText: "Detected 2 installed software entries",
			wantHTML: []string{
				`<summary>\s*App One\s*</summary>`,
				`<li><strong>ProductCode:</strong>\s*product1\s*</li>`,
				`<li><strong>Version:</strong>\s*1\.0\s*</li>`,
				`<summary>\s*product2\s*</summary>`,
				`<li><strong>ProductCode:</strong>\s*product2\s*</li>`,
				`<li><strong>Version:</strong>\s*2\.0\s*</li>`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			is, sp := newInventorySoftware(t, nil)
			sp.On("GetCheck", sysconfig.InventorySoftwareModule).Return(tt.mockData, nil)

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
	is, sp := newInventorySoftware(t, nil)
	sp.On("GetCheck", sysconfig.InventorySoftwareModule).Return(SoftwareInventoryMap{}, nil)

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

	// When the software inventory list is empty, we call GetCheck every time
	sp.AssertNumberOfCalls(t, "GetCheck", 2)
}
