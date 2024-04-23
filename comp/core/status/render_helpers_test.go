// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package status

import (
	"bytes"
	"html/template"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNtpWarning(t *testing.T) {
	require.False(t, ntpWarning(1))
	require.False(t, ntpWarning(-1))
	require.True(t, ntpWarning(3601))
	require.True(t, ntpWarning(-601))
}

func TestMkHuman(t *testing.T) {
	f := 1695783.0
	fStr := mkHuman(f)
	assert.Equal(t, "1,695,783", fStr, "Large number formatting is incorrectly adding commas in agent statuses")

	assert.Equal(t, "1", mkHuman(1))

	// assert.Equal(t, "1", textFuncMap["humanize"]("1"))
	// assert.Equal(t, "1.5", mkHuman(float32(1.5)))
}

func TestUntypeFuncMap(t *testing.T) {
	templateText := `
	{{ doNotEscape .DoNotEscapeValue }}
	{{ lastError .LastErrorValue }}
	{{ configError .ConfigErrorValue }}
	{{ printDashes .PrintDashesValue .PrintDashesValue2 }}
	{{ formatUnixTime .FormatUnixTimeValue }}
	{{ humanize .HumanizeValue }}
	{{ humanizeDuration .HumanizeDurationValue .HumanizeDurationValue2 }}
	{{ toUnsortedList .ToUnsortedListValue }}
	{{ formatTitle .FormatTitleValue }}
	{{ add .AddValue1 .AddValue2 }}
	{{ redText .RedTextValue }}
	{{ yellowText .YellowTextValue }}
	{{ greenText .GreenTextValue }}
	{{ ntpWarning .NtpWarningValue }}
	{{ version .VersionValue }}
	{{ percent .PercentValue }}
	{{ complianceResult .ComplianceResultValue }}
	{{ lastErrorTraceback .LastErrorTracebackValue }}
	{{ lastErrorMessage .LastErrorMessageValue }}
	{{ pythonLoaderError .PythonLoaderErrorValue }}
	`
	// {{ status .StatusValue }}
	valueStruct := struct {
		DoNotEscapeValue        string
		LastErrorValue          string
		ConfigErrorValue        string
		PrintDashesValue        string
		PrintDashesValue2       string
		FormatUnixTimeValue     int64
		HumanizeValue           string
		HumanizeDurationValue   time.Duration
		HumanizeDurationValue2  string
		ToUnsortedListValue     map[string]interface{}
		FormatTitleValue        string
		AddValue1               int
		AddValue2               int
		RedTextValue            string
		YellowTextValue         string
		GreenTextValue          string
		NtpWarningValue         string
		VersionValue            string
		PercentValue            float64
		ComplianceResultValue   int
		LastErrorTracebackValue string
		LastErrorMessageValue   string
		PythonLoaderErrorValue  string
		StatusValue             string
	}{
		DoNotEscapeValue:        "mockDoNotEscape",
		LastErrorValue:          "mockLastError",
		ConfigErrorValue:        "mockConfigError",
		PrintDashesValue:        "mockPrintDashes1",
		PrintDashesValue2:       "mockPrintDashes2",
		FormatUnixTimeValue:     1617459250, // example Unix timestamp
		HumanizeValue:           "mockHumanize",
		HumanizeDurationValue:   1 * time.Hour,
		HumanizeDurationValue2:  "",
		ToUnsortedListValue:     map[string]interface{}{"key1": "mockToUnsortedList1", "key2": 123},
		FormatTitleValue:        "mockFormatTitle",
		AddValue1:               1,
		AddValue2:               2,
		RedTextValue:            "mockRedText",
		YellowTextValue:         "mockYellowText",
		GreenTextValue:          "mockGreenText",
		NtpWarningValue:         "mockNtpWarning",
		VersionValue:            "mockVersion",
		PercentValue:            0.5,
		ComplianceResultValue:   12,
		LastErrorTracebackValue: "mockLastErrorTraceback",
		LastErrorMessageValue:   "mockLastErrorMessage",
		PythonLoaderErrorValue:  "mockPythonLoaderError",
		StatusValue:             "mockStatus",
	}
	// Create a template, add the function map, and parse the text.
	tmpl, err := template.New("titleTest").Funcs(HTMLFmap()).Parse(templateText)
	assert.NoError(t, err)

	var buf bytes.Buffer

	// Run the template to verify the output.
	err = tmpl.Execute(&buf, valueStruct)
	assert.NoError(t, err)
}
