// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package statusimpl

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/status"
	pkgConfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/version"
	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"
)

type mockProvider struct {
	data    map[string]interface{}
	text    string
	html    string
	name    string
	section string
}

func (m mockProvider) Name() string {
	return m.name
}

func (m mockProvider) Section() string {
	return m.section
}

func (m mockProvider) JSON(stats map[string]interface{}) error {
	for key, value := range m.data {
		stats[key] = value
	}

	return nil
}

func (m mockProvider) Text(buffer io.Writer) error {
	_, err := buffer.Write([]byte(m.text))
	return err
}

func (m mockProvider) HTML(buffer io.Writer) error {
	_, err := buffer.Write([]byte(m.html))
	return err
}

type mockHeaderProvider struct {
	data  map[string]interface{}
	text  string
	html  string
	index int
	name  string
}

func (m mockHeaderProvider) Index() int {
	return m.index
}

func (m mockHeaderProvider) Name() string {
	return m.name
}

func (m mockHeaderProvider) JSON(stats map[string]interface{}) error {
	for key, value := range m.data {
		stats[key] = value
	}

	return nil
}

func (m mockHeaderProvider) Text(buffer io.Writer) error {
	_, err := buffer.Write([]byte(m.text))
	return err
}

func (m mockHeaderProvider) HTML(buffer io.Writer) error {
	_, err := buffer.Write([]byte(m.html))
	return err
}

type errorMockProvider struct{}

func (m errorMockProvider) Name() string {
	return "error mock"
}

func (m errorMockProvider) Section() string {
	return "error section"
}

func (m errorMockProvider) JSON(map[string]interface{}) error {
	return fmt.Errorf("testing JSON errors")
}

func (m errorMockProvider) Text(io.Writer) error {
	return fmt.Errorf("testing Text errors")
}

func (m errorMockProvider) HTML(io.Writer) error {
	return fmt.Errorf("testing HTML errors")
}

var (
	humanReadbaleFlavor = flavor.GetHumanReadableFlavor()
	agentVersion        = version.AgentVersion
	pid                 = os.Getpid()
	goVersion           = runtime.Version()
	arch                = runtime.GOARCH
	agentFlavor         = flavor.GetFlavor()
	testTitle           = fmt.Sprintf("%s (v%s)", humanReadbaleFlavor, agentVersion)
)

var testTextHeader = fmt.Sprintf(`%s
%s
%s`, status.PrintDashes(testTitle, "="), testTitle, status.PrintDashes(testTitle, "="))

var expectedStatusTextOutput = fmt.Sprintf(`%s
  Status date: 2018-01-05 11:25:15 UTC (1515151515000)
  Agent start: 2018-01-05 11:25:15 UTC (1515151515000)
  Pid: %d
  Go Version: %s
  Python Version: n/a
  Build arch: %s
  Agent flavor: %s
  Log Level: info

==========
Header Foo
==========
  header foo: header bar
  header foo2: header bar 2

=========
Collector
=========
 text from a
 text from b

=========
A Section
=========
 text from a

=========
X Section
=========
 text from a
 text from x

`, testTextHeader, pid, goVersion, arch, agentFlavor)

func TestGetStatus(t *testing.T) {
	nowFunc = func() time.Time { return time.Unix(1515151515, 0) }
	startTimeProvider = time.Unix(1515151515, 0)
	originalTZ := os.Getenv("TZ")
	os.Setenv("TZ", "UTC")

	defer func() {
		nowFunc = time.Now
		startTimeProvider = pkgConfig.StartTime
		os.Setenv("TZ", originalTZ)
	}()

	deps := fxutil.Test[dependencies](t, fx.Options(
		config.MockModule(),
		fx.Supply(
			status.NewInformationProvider(mockProvider{
				data: map[string]interface{}{
					"foo": "bar",
				},
				name: "a",
				text: " text from a\n",
				html: `<div class="stat">
  <span class="stat_title">Foo</span>
  <span class="stat_data">
    <br>Bar: bar
  </span>
</div>
`,
				section: status.CollectorSection,
			}),
			status.NewInformationProvider(mockProvider{
				data: map[string]interface{}{
					"foo2": "bar2",
				},
				name:    "b",
				text:    " text from b\n",
				section: status.CollectorSection,
			}),
			status.NewInformationProvider(mockProvider{
				data: map[string]interface{}{
					"foo3": "bar3",
				},
				name:    "x",
				text:    " text from x\n",
				section: "x section",
			}),
			status.NewInformationProvider(mockProvider{
				data: map[string]interface{}{
					"foo3": "bar3",
				},
				name:    "a",
				text:    " text from a\n",
				section: "a section",
			}),
			status.NewInformationProvider(mockProvider{
				data: map[string]interface{}{
					"foo3": "bar3",
				},
				name:    "a",
				text:    " text from a\n",
				section: "x section",
			}),
			status.NewHeaderInformationProvider(mockHeaderProvider{
				name: "header foo",
				data: map[string]interface{}{
					"header_foo": "header_bar",
				},
				text: `  header foo: header bar
  header foo2: header bar 2
`,
				html: `<div class="stat">
  <span class="stat_title">Header Foo</span>
  <span class="stat_data">
    <br>Header Bar: bar
  </span>
</div>
`,
				index: 2,
			}),
		),
	))

	statusComponent, err := newStatus(deps)

	assert.NoError(t, err)

	testCases := []struct {
		name       string
		format     string
		assertFunc func(*testing.T, []byte)
	}{
		{
			name:   "JSON",
			format: "json",
			assertFunc: func(t *testing.T, bytes []byte) {
				result := map[string]interface{}{}
				err = json.Unmarshal(bytes, &result)

				assert.NoError(t, err)

				assert.Equal(t, "bar", result["foo"])
				assert.Equal(t, "header_bar", result["header_foo"])
			},
		},
		{
			name:   "Text",
			format: "text",
			assertFunc: func(t *testing.T, bytes []byte) {
				// We replace windows line break by linux so the tests pass on every OS
				expectedResult := strings.Replace(expectedStatusTextOutput, "\r\n", "\n", -1)
				output := strings.Replace(string(bytes), "\r\n", "\n", -1)

				assert.Equal(t, expectedResult, output)
			},
		},
		{
			name:   "HTML",
			format: "html",
			assertFunc: func(t *testing.T, bytes []byte) {
				// We have to do this strings replacement because html/temaplte escapes the `+` sign
				// https://github.com/golang/go/issues/42506
				result := string(bytes)
				unescapedResult := strings.Replace(result, "&#43;", "+", -1)

				expectedStatusHTMLOutput := fmt.Sprintf(`<div class="stat">
  <span class="stat_title">Agent Info</span>
  <span class="stat_data">
    Version: %s
    <br>Flavor: %s
    <br>PID: %d
    <br>Agent start: 2018-01-05 11:25:15 UTC (1515151515000)
    <br>Log Level: info
    <br>Config File: There is no config file
    <br>Conf.d Path: %s
    <br>Checks.d Path: %s
  </span>
</div>

<div class="stat">
  <span class="stat_title">System Info</span>
  <span class="stat_data">
    System time: 2018-01-05 11:25:15 UTC (1515151515000)
    <br>Go Version: %s
    <br>Python Version: n/a
    <br>Build arch: %s
  </span>
</div>
<div class="stat">
  <span class="stat_title">Header Foo</span>
  <span class="stat_data">
    <br>Header Bar: bar
  </span>
</div>
<div class="stat">
  <span class="stat_title">Foo</span>
  <span class="stat_data">
    <br>Bar: bar
  </span>
</div>
`, agentVersion, agentFlavor, pid, deps.Config.GetString("confd_path"), deps.Config.GetString("additional_checksd"), goVersion, arch)

				// We replace windows line break by linux so the tests pass on every OS
				expectedResult := strings.Replace(expectedStatusHTMLOutput, "\r\n", "\n", -1)
				output := strings.Replace(unescapedResult, "\r\n", "\n", -1)

				assert.Equal(t, expectedResult, output)
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			bytesResult, err := statusComponent.GetStatus(testCase.format, false)

			assert.NoError(t, err)

			testCase.assertFunc(t, bytesResult)
		})
	}
}

var expectedStatusTextErrorOutput = fmt.Sprintf(`%s
  Status date: 2018-01-05 11:25:15 UTC (1515151515000)
  Agent start: 2018-01-05 11:25:15 UTC (1515151515000)
  Pid: %d
  Go Version: %s
  Python Version: n/a
  Build arch: %s
  Agent flavor: agent
  Log Level: info

=========
Collector
=========
 text from b

=============
Error Section
=============

====================
Status render errors
====================
  - testing Text errors

`, testTextHeader, pid, goVersion, arch)

func TestGetStatusWithErrors(t *testing.T) {
	nowFunc = func() time.Time { return time.Unix(1515151515, 0) }
	startTimeProvider = time.Unix(1515151515, 0)
	originalTZ := os.Getenv("TZ")
	os.Setenv("TZ", "UTC")

	defer func() {
		nowFunc = time.Now
		startTimeProvider = pkgConfig.StartTime
		os.Setenv("TZ", originalTZ)
	}()

	deps := fxutil.Test[dependencies](t, fx.Options(
		config.MockModule(),
		fx.Supply(
			status.NewInformationProvider(errorMockProvider{}),
			status.NewInformationProvider(mockProvider{
				data: map[string]interface{}{
					"foo2": "bar2",
				},
				name:    "b",
				text:    " text from b\n",
				section: status.CollectorSection,
			}),
		),
	))

	statusComponent, err := newStatus(deps)

	assert.NoError(t, err)

	testCases := []struct {
		name       string
		format     string
		assertFunc func(*testing.T, []byte)
	}{
		{
			name:   "JSON",
			format: "json",
			assertFunc: func(t *testing.T, bytes []byte) {
				result := map[string]interface{}{}
				err = json.Unmarshal(bytes, &result)

				assert.NoError(t, err)

				assert.Equal(t, "testing JSON errors", result["errors"].([]interface{})[0].(string))
			},
		},
		{
			name:   "Text",
			format: "text",
			assertFunc: func(t *testing.T, bytes []byte) {
				// We replace windows line break by linux so the tests pass on every OS
				expectedResult := strings.Replace(expectedStatusTextErrorOutput, "\r\n", "\n", -1)
				output := strings.Replace(string(bytes), "\r\n", "\n", -1)

				assert.Equal(t, expectedResult, output)
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			bytesResult, err := statusComponent.GetStatus(testCase.format, false)

			assert.NoError(t, err)

			testCase.assertFunc(t, bytesResult)
		})
	}
}

func TestGetStatusBySection(t *testing.T) {
	deps := fxutil.Test[dependencies](t, fx.Options(
		config.MockModule(),
		fx.Supply(
			status.NewInformationProvider(mockProvider{
				data: map[string]interface{}{
					"foo": "bar",
				},
				name: "a",
				text: " text from a\n",
				html: `<div class="stat">
  <span class="stat_title">Foo</span>
  <span class="stat_data">
    <br>Bar: bar
  </span>
</div>
`,
				section: status.CollectorSection,
			}),
			status.NewInformationProvider(mockProvider{
				data: map[string]interface{}{
					"foo2": "bar2",
				},
				name:    "b",
				text:    " text from b\n",
				section: status.CollectorSection,
			}),
			status.NewInformationProvider(mockProvider{
				data: map[string]interface{}{
					"foo3": "bar3",
				},
				name:    "x",
				text:    " text from x\n",
				section: "x section",
			}),
			status.NewInformationProvider(mockProvider{
				data: map[string]interface{}{
					"foo3": "bar3",
				},
				name:    "a",
				text:    " text from a\n",
				section: "a section",
			}),
			status.NewInformationProvider(mockProvider{
				data: map[string]interface{}{
					"foo3": "bar3",
				},
				name:    "a",
				text:    " text from a\n",
				section: "x section",
			}),
			status.NewHeaderInformationProvider(mockHeaderProvider{
				data: map[string]interface{}{
					"header_foo": "header_bar",
				},
				text: `  header foo: header bar
  header foo2: header bar 2
`,
				html: `<div class="stat">
  <span class="stat_title">Header Foo</span>
  <span class="stat_data">
    <br>Header Bar: bar
  </span>
</div>
`,
				index: 2,
			}),
		),
	))

	statusComponent, err := newStatus(deps)

	assert.NoError(t, err)

	testCases := []struct {
		name       string
		section    string
		format     string
		assertFunc func(*testing.T, []byte)
	}{
		{
			name:    "JSON",
			section: "header",
			format:  "json",
			assertFunc: func(t *testing.T, bytes []byte) {
				result := map[string]interface{}{}
				err = json.Unmarshal(bytes, &result)

				assert.NoError(t, err)

				assert.Nil(t, result["foo"])
				assert.Equal(t, "header_bar", result["header_foo"])
			},
		},
		{
			name:    "Text",
			format:  "text",
			section: "x section",
			assertFunc: func(t *testing.T, bytes []byte) {
				result := `=========
X Section
=========
 text from a
 text from x
`

				// We replace windows line break by linux so the tests pass on every OS
				expectedResult := strings.Replace(result, "\r\n", "\n", -1)
				output := strings.Replace(string(bytes), "\r\n", "\n", -1)

				assert.Equal(t, expectedResult, output)
			},
		},
		{
			name:    "HTML",
			section: "collector",
			format:  "html",
			assertFunc: func(t *testing.T, bytes []byte) {
				result := `<div class="stat">
  <span class="stat_title">Foo</span>
  <span class="stat_data">
    <br>Bar: bar
  </span>
</div>
`
				// We replace windows line break by linux so the tests pass on every OS
				expectedResult := strings.Replace(result, "\r\n", "\n", -1)
				output := strings.Replace(string(bytes), "\r\n", "\n", -1)

				assert.Equal(t, expectedResult, output)
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			bytesResult, err := statusComponent.GetStatusBySection(testCase.section, testCase.format, false)

			assert.NoError(t, err)

			testCase.assertFunc(t, bytesResult)
		})
	}
}

func TestGetStatusBySectionsWithErrors(t *testing.T) {
	nowFunc = func() time.Time { return time.Unix(1515151515, 0) }
	startTimeProvider = time.Unix(1515151515, 0)
	originalTZ := os.Getenv("TZ")
	os.Setenv("TZ", "UTC")

	defer func() {
		nowFunc = time.Now
		startTimeProvider = pkgConfig.StartTime
		os.Setenv("TZ", originalTZ)
	}()

	deps := fxutil.Test[dependencies](t, fx.Options(
		config.MockModule(),
		fx.Supply(
			status.NewInformationProvider(errorMockProvider{}),
			status.NewInformationProvider(mockProvider{
				data: map[string]interface{}{
					"foo2": "bar2",
				},
				name:    "b",
				text:    " text from b\n",
				section: status.CollectorSection,
			}),
		),
	))

	statusComponent, err := newStatus(deps)

	assert.NoError(t, err)

	testCases := []struct {
		name       string
		format     string
		assertFunc func(*testing.T, []byte)
	}{
		{
			name:   "JSON",
			format: "json",
			assertFunc: func(t *testing.T, bytes []byte) {
				result := map[string]interface{}{}
				err = json.Unmarshal(bytes, &result)

				assert.NoError(t, err)

				assert.Equal(t, "testing JSON errors", result["errors"].([]interface{})[0].(string))
			},
		},
		{
			name:   "Text",
			format: "text",
			assertFunc: func(t *testing.T, bytes []byte) {
				expected := `=============
Error Section
=============
====================
Status render errors
====================
  - testing Text errors

`

				// We replace windows line break by linux so the tests pass on every OS
				expectedResult := strings.Replace(expected, "\r\n", "\n", -1)
				output := strings.Replace(string(bytes), "\r\n", "\n", -1)

				assert.Equal(t, expectedResult, output)
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			bytesResult, err := statusComponent.GetStatusBySection("error section", testCase.format, false)

			assert.NoError(t, err)

			testCase.assertFunc(t, bytesResult)
		})
	}
}
