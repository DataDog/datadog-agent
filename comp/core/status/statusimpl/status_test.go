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

	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/status"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/version"
)

type mockProvider struct {
	data        map[string]interface{}
	text        string
	html        string
	name        string
	section     string
	returnError bool
}

func (m mockProvider) Name() string {
	return m.name
}

func (m mockProvider) Section() string {
	return m.section
}

func (m mockProvider) JSON(_ bool, stats map[string]interface{}) error {
	if m.returnError {
		return fmt.Errorf("JSON error")
	}

	for key, value := range m.data {
		stats[key] = value
	}

	return nil
}

func (m mockProvider) Text(_ bool, buffer io.Writer) error {
	if m.returnError {
		return fmt.Errorf("Text error")
	}

	_, err := buffer.Write([]byte(m.text))
	return err
}

func (m mockProvider) HTML(_ bool, buffer io.Writer) error {
	if m.returnError {
		return fmt.Errorf("HTML error")
	}

	_, err := buffer.Write([]byte(m.html))
	return err
}

type mockHeaderProvider struct {
	data        map[string]interface{}
	text        string
	html        string
	index       int
	name        string
	returnError bool
}

func (m mockHeaderProvider) Index() int {
	return m.index
}

func (m mockHeaderProvider) Name() string {
	return m.name
}

func (m mockHeaderProvider) JSON(_ bool, stats map[string]interface{}) error {
	if m.returnError {
		return fmt.Errorf("JSON error")
	}

	for key, value := range m.data {
		stats[key] = value
	}

	return nil
}

func (m mockHeaderProvider) Text(_ bool, buffer io.Writer) error {
	if m.returnError {
		return fmt.Errorf("Text error")
	}

	_, err := buffer.Write([]byte(m.text))
	return err
}

func (m mockHeaderProvider) HTML(_ bool, buffer io.Writer) error {
	if m.returnError {
		return fmt.Errorf("HTML error")
	}

	_, err := buffer.Write([]byte(m.html))
	return err
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

var agentParams = status.Params{
	PythonVersionGetFunc: func() string { return "n/a" },
}

var testTextHeader = fmt.Sprintf(`%s
%s
%s`, status.PrintDashes(testTitle, "="), testTitle, status.PrintDashes(testTitle, "="))

func TestGetStatus(t *testing.T) {
	nowFunc = func() time.Time { return time.Unix(1515151515, 0) }
	startTimeProvider = time.Unix(1515151515, 0)
	originalTZ := os.Getenv("TZ")
	os.Setenv("TZ", "UTC")

	defer func() {
		nowFunc = time.Now
		startTimeProvider = pkgconfigsetup.StartTime
		os.Setenv("TZ", originalTZ)
	}()

	deps := fxutil.Test[dependencies](t, fx.Options(
		config.MockModule(),
		fx.Supply(
			agentParams,
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

	provides := newStatus(deps)
	statusComponent := provides.Comp

	testCases := []struct {
		name            string
		format          string
		excludeSections []string
		assertFunc      func(*testing.T, []byte)
	}{
		{
			name:   "JSON",
			format: "json",
			assertFunc: func(t *testing.T, bytes []byte) {
				result := map[string]interface{}{}
				err := json.Unmarshal(bytes, &result)

				assert.NoError(t, err)

				assert.Equal(t, "bar", result["foo"])
				assert.Equal(t, "header_bar", result["header_foo"])
			},
		},
		{
			name:            "JSON exclude section",
			format:          "json",
			excludeSections: []string{status.CollectorSection},
			assertFunc: func(t *testing.T, bytes []byte) {
				result := map[string]interface{}{}
				err := json.Unmarshal(bytes, &result)

				assert.NoError(t, err)

				assert.Nil(t, result["foo"])
				assert.Equal(t, "header_bar", result["header_foo"])
			},
		},
		{
			name:   "Text",
			format: "text",
			assertFunc: func(t *testing.T, bytes []byte) {
				expectedStatusTextOutput := fmt.Sprintf(`%s
  Status date: 2018-01-05 11:25:15 UTC (1515151515000)
  Agent start: 2018-01-05 11:25:15 UTC (1515151515000)
  Pid: %d
  Go Version: %s
  Python Version: n/a
  Build arch: %s
  Agent flavor: %s
  Log Level: info

  Paths
  =====
    Config File: There is no config file
    conf.d: %s
    checks.d: %s

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

`, testTextHeader, pid, goVersion, arch, agentFlavor, deps.Config.GetString("confd_path"), deps.Config.GetString("additional_checksd"))
				// We replace windows line break by linux so the tests pass on every OS
				expectedResult := strings.Replace(expectedStatusTextOutput, "\r\n", "\n", -1)
				output := strings.Replace(string(bytes), "\r\n", "\n", -1)

				assert.Equal(t, expectedResult, output)
			},
		},
		{
			name:            "Text exclude section",
			format:          "text",
			excludeSections: []string{status.CollectorSection},
			assertFunc: func(t *testing.T, bytes []byte) {
				expectedStatusTextOutput := fmt.Sprintf(`%s
  Status date: 2018-01-05 11:25:15 UTC (1515151515000)
  Agent start: 2018-01-05 11:25:15 UTC (1515151515000)
  Pid: %d
  Go Version: %s
  Python Version: n/a
  Build arch: %s
  Agent flavor: %s
  Log Level: info

  Paths
  =====
    Config File: There is no config file
    conf.d: %s
    checks.d: %s

==========
Header Foo
==========
  header foo: header bar
  header foo2: header bar 2

=========
A Section
=========
 text from a

=========
X Section
=========
 text from a
 text from x

`, testTextHeader, pid, goVersion, arch, agentFlavor, deps.Config.GetString("confd_path"), deps.Config.GetString("additional_checksd"))

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
		{
			name:            "HTML exclude section",
			format:          "html",
			excludeSections: []string{status.CollectorSection},
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
			bytesResult, err := statusComponent.GetStatus(testCase.format, false, testCase.excludeSections...)

			assert.NoError(t, err)

			testCase.assertFunc(t, bytesResult)
		})
	}
}

func TestGetStatusDoNotRenderHeaderIfNoProviders(t *testing.T) {
	nowFunc = func() time.Time { return time.Unix(1515151515, 0) }
	startTimeProvider = time.Unix(1515151515, 0)
	originalTZ := os.Getenv("TZ")
	os.Setenv("TZ", "UTC")

	defer func() {
		nowFunc = time.Now
		startTimeProvider = pkgconfigsetup.StartTime
		os.Setenv("TZ", originalTZ)
	}()

	deps := fxutil.Test[dependencies](t, fx.Options(
		config.MockModule(),
		fx.Supply(
			agentParams,
			status.NewInformationProvider(mockProvider{
				data: map[string]interface{}{
					"foo": "bar",
				},
				name:    "a",
				text:    " text from a\n",
				section: "section",
			}),
		),
	))

	provides := newStatus(deps)
	statusComponent := provides.Comp

	bytesResult, err := statusComponent.GetStatus("text", false)

	assert.NoError(t, err)

	expectedOutput := fmt.Sprintf(`%s
  Status date: 2018-01-05 11:25:15 UTC (1515151515000)
  Agent start: 2018-01-05 11:25:15 UTC (1515151515000)
  Pid: %d
  Go Version: %s
  Python Version: n/a
  Build arch: %s
  Agent flavor: %s
  Log Level: info

  Paths
  =====
    Config File: There is no config file
    conf.d: %s
    checks.d: %s

=======
Section
=======
 text from a

`, testTextHeader, pid, goVersion, arch, agentFlavor, deps.Config.GetString("confd_path"), deps.Config.GetString("additional_checksd"))

	// We replace windows line break by linux so the tests pass on every OS
	expectedResult := strings.Replace(expectedOutput, "\r\n", "\n", -1)
	output := strings.Replace(string(bytesResult), "\r\n", "\n", -1)

	assert.Equal(t, expectedResult, output)
}

func TestGetStatusWithErrors(t *testing.T) {
	nowFunc = func() time.Time { return time.Unix(1515151515, 0) }
	startTimeProvider = time.Unix(1515151515, 0)
	originalTZ := os.Getenv("TZ")
	os.Setenv("TZ", "UTC")

	defer func() {
		nowFunc = time.Now
		startTimeProvider = pkgconfigsetup.StartTime
		os.Setenv("TZ", originalTZ)
	}()

	deps := fxutil.Test[dependencies](t, fx.Options(
		config.MockModule(),
		fx.Supply(
			agentParams,
			status.NewInformationProvider(mockProvider{
				section:     "error section",
				name:        "a",
				returnError: true,
			}),
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

	provides := newStatus(deps)
	statusComponent := provides.Comp

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
				err := json.Unmarshal(bytes, &result)

				assert.NoError(t, err)

				assert.Equal(t, "JSON error", result["errors"].([]interface{})[0].(string))
			},
		},
		{
			name:   "Text",
			format: "text",
			assertFunc: func(t *testing.T, bytes []byte) {
				expectedStatusTextErrorOutput := fmt.Sprintf(`%s
  Status date: 2018-01-05 11:25:15 UTC (1515151515000)
  Agent start: 2018-01-05 11:25:15 UTC (1515151515000)
  Pid: %d
  Go Version: %s
  Python Version: n/a
  Build arch: %s
  Agent flavor: agent
  Log Level: info

  Paths
  =====
    Config File: There is no config file
    conf.d: %s
    checks.d: %s

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
  - Text error

`, testTextHeader, pid, goVersion, arch, deps.Config.GetString("confd_path"), deps.Config.GetString("additional_checksd"))

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
			agentParams,
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

	provides := newStatus(deps)
	statusComponent := provides.Comp

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
				err := json.Unmarshal(bytes, &result)

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
		{
			name:    "Text case insensitive",
			format:  "text",
			section: "X SeCtIoN",
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
		startTimeProvider = pkgconfigsetup.StartTime
		os.Setenv("TZ", originalTZ)
	}()

	deps := fxutil.Test[dependencies](t, fx.Options(
		config.MockModule(),
		fx.Supply(
			agentParams,
			status.NewInformationProvider(mockProvider{
				returnError: true,
				section:     "error section",
				name:        "a",
			}),
			status.NewHeaderInformationProvider(mockHeaderProvider{
				returnError: true,
				name:        "a",
				index:       3,
			}),
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

	provides := newStatus(deps)
	statusComponent := provides.Comp

	testCases := []struct {
		name       string
		format     string
		section    string
		assertFunc func(*testing.T, []byte)
	}{
		{
			name:    "JSON",
			format:  "json",
			section: "error section",
			assertFunc: func(t *testing.T, bytes []byte) {
				result := map[string]interface{}{}
				err := json.Unmarshal(bytes, &result)

				assert.NoError(t, err)

				assert.Equal(t, "JSON error", result["errors"].([]interface{})[0].(string))
			},
		},
		{
			name:    "Text",
			format:  "text",
			section: "error section",
			assertFunc: func(t *testing.T, bytes []byte) {
				expected := `=============
Error Section
=============
====================
Status render errors
====================
  - Text error

`

				// We replace windows line break by linux so the tests pass on every OS
				expectedResult := strings.Replace(expected, "\r\n", "\n", -1)
				output := strings.Replace(string(bytes), "\r\n", "\n", -1)

				assert.Equal(t, expectedResult, output)
			},
		},
		{
			name:    "Header section JSON format",
			format:  "json",
			section: "header",
			assertFunc: func(t *testing.T, bytes []byte) {
				result := map[string]interface{}{}
				err := json.Unmarshal(bytes, &result)

				assert.NoError(t, err)

				assert.Equal(t, "JSON error", result["errors"].([]interface{})[0].(string))
			},
		},
		{
			name:    "Header section text format",
			format:  "text",
			section: "header",
			assertFunc: func(t *testing.T, bytes []byte) {

				expectedStatusTextErrorOutput := fmt.Sprintf(`%s
  Status date: 2018-01-05 11:25:15 UTC (1515151515000)
  Agent start: 2018-01-05 11:25:15 UTC (1515151515000)
  Pid: %d
  Go Version: %s
  Python Version: n/a
  Build arch: %s
  Agent flavor: agent
  Log Level: info

  Paths
  =====
    Config File: There is no config file
    conf.d: %s
    checks.d: %s

====================
Status render errors
====================
  - Text error

`, testTextHeader, pid, goVersion, arch, deps.Config.GetString("confd_path"), deps.Config.GetString("additional_checksd"))

				// We replace windows line break by linux so the tests pass on every OS
				expectedResult := strings.Replace(expectedStatusTextErrorOutput, "\r\n", "\n", -1)
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

func TestFlareProvider(t *testing.T) {
	nowFunc = func() time.Time { return time.Unix(1515151515, 0) }
	startTimeProvider = time.Unix(1515151515, 0)
	originalTZ := os.Getenv("TZ")
	os.Setenv("TZ", "UTC")

	defer func() {
		nowFunc = time.Now
		startTimeProvider = pkgconfigsetup.StartTime
		os.Setenv("TZ", originalTZ)
	}()

	deps := fxutil.Test[dependencies](t, fx.Options(
		config.MockModule(),
		fx.Supply(agentParams),
	))

	provides := newStatus(deps)
	flareProvider := provides.FlareProvider.Provider

	assert.NotNil(t, flareProvider)
}

func TestGetStatusBySectionIncorrect(t *testing.T) {

	deps := fxutil.Test[dependencies](t, fx.Options(
		config.MockModule(),
		fx.Supply(
			agentParams,
			status.NewInformationProvider(mockProvider{
				returnError: false,
				section:     "Lorem",
				name:        "1",
			}),
			status.NewInformationProvider(mockProvider{
				returnError: false,
				section:     "ipsum",
				name:        "1",
			}),
			status.NewInformationProvider(mockProvider{
				returnError: false,
				section:     "doloR",
				name:        "1",
			}),
			status.NewInformationProvider(mockProvider{
				returnError: false,
				section:     "Sit",
				name:        "1",
			}),
			status.NewInformationProvider(mockProvider{
				returnError: false,
				section:     "AmEt",
				name:        "1",
			}),
		),
	))

	provides := newStatus(deps)
	statusComponent := provides.Comp

	bytesResult, err := statusComponent.GetStatusBySection("consectetur", "json", false)

	assert.Nil(t, bytesResult)
	assert.EqualError(t, err, `unknown status section 'consectetur', available sections are: ["amet","dolor","ipsum","lorem","sit"]`)
}
