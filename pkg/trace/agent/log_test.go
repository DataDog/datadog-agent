package agent

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/stretchr/testify/assert"
)

func TestMakeLoggerConfig(t *testing.T) {
	t.Run("config", func(t *testing.T) {
		for _, tt := range []struct {
			cfg *config.AgentConfig
			out string
		}{
			{
				cfg: &config.AgentConfig{},
				out: `
<seelog minlevel="info">
  <outputs formatid="common">
    <filter levels="warn,error">
      <custom name="throttled" data-interval="0" data-console="false" data-max-per-interval="10" data-use-json="false" data-file-path="" />
    </filter>
    <filter levels="trace,debug,info,critical">
      <rollingfile type="size" filename="" maxsize="10000000" maxrolls="5" />
    </filter>
  </outputs>
  <formats>
    <format id="json" format="{&quot;agent&quot;:&quot;trace&quot;,&quot;time&quot;:&quot;%Date(2006-01-02 15:04:05 MST)&quot;,&quot;level&quot;:&quot;%LEVEL&quot;,&quot;file&quot;:&quot;%ShortFilePath&quot;,&quot;line&quot;:&quot;%Line&quot;,&quot;func&quot;:&quot;%FuncShort&quot;,&quot;msg&quot;:%QuoteMsg}%n"/>
    <format id="common" format="%Date(2006-01-02 15:04:05 MST) | TRACE | %LEVEL | (%ShortFilePath:%Line in %FuncShort) | %Msg%n"/>
  </formats>
</seelog>
`,
			},
			{
				cfg: &config.AgentConfig{LogThrottling: true},
				out: `
<seelog minlevel="info">
  <outputs formatid="common">
    <filter levels="warn,error">
      <custom name="throttled" data-interval="10000000000" data-console="false" data-max-per-interval="10" data-use-json="false" data-file-path="" />
    </filter>
    <filter levels="trace,debug,info,critical">
      <rollingfile type="size" filename="" maxsize="10000000" maxrolls="5" />
    </filter>
  </outputs>
  <formats>
    <format id="json" format="{&quot;agent&quot;:&quot;trace&quot;,&quot;time&quot;:&quot;%Date(2006-01-02 15:04:05 MST)&quot;,&quot;level&quot;:&quot;%LEVEL&quot;,&quot;file&quot;:&quot;%ShortFilePath&quot;,&quot;line&quot;:&quot;%Line&quot;,&quot;func&quot;:&quot;%FuncShort&quot;,&quot;msg&quot;:%QuoteMsg}%n"/>
    <format id="common" format="%Date(2006-01-02 15:04:05 MST) | TRACE | %LEVEL | (%ShortFilePath:%Line in %FuncShort) | %Msg%n"/>
  </formats>
</seelog>
`,
			},
			{
				cfg: &config.AgentConfig{LogThrottling: true, LogFormatJSON: true},
				out: `
<seelog minlevel="info">
  <outputs formatid="json">
    <filter levels="warn,error">
      <custom name="throttled" data-interval="10000000000" data-console="false" data-max-per-interval="10" data-use-json="true" data-file-path="" />
    </filter>
    <filter levels="trace,debug,info,critical">
      <rollingfile type="size" filename="" maxsize="10000000" maxrolls="5" />
    </filter>
  </outputs>
  <formats>
    <format id="json" format="{&quot;agent&quot;:&quot;trace&quot;,&quot;time&quot;:&quot;%Date(2006-01-02 15:04:05 MST)&quot;,&quot;level&quot;:&quot;%LEVEL&quot;,&quot;file&quot;:&quot;%ShortFilePath&quot;,&quot;line&quot;:&quot;%Line&quot;,&quot;func&quot;:&quot;%FuncShort&quot;,&quot;msg&quot;:%QuoteMsg}%n"/>
    <format id="common" format="%Date(2006-01-02 15:04:05 MST) | TRACE | %LEVEL | (%ShortFilePath:%Line in %FuncShort) | %Msg%n"/>
  </formats>
</seelog>
`,
			},
			{
				cfg: &config.AgentConfig{LogToConsole: true},
				out: `
<seelog minlevel="info">
  <outputs formatid="common">
    <filter levels="warn,error">
      <custom name="throttled" data-interval="0" data-console="true" data-max-per-interval="10" data-use-json="false" data-file-path="" />
    </filter>
    <filter levels="trace,debug,info,critical">
      <console />
      <rollingfile type="size" filename="" maxsize="10000000" maxrolls="5" />
    </filter>
  </outputs>
  <formats>
    <format id="json" format="{&quot;agent&quot;:&quot;trace&quot;,&quot;time&quot;:&quot;%Date(2006-01-02 15:04:05 MST)&quot;,&quot;level&quot;:&quot;%LEVEL&quot;,&quot;file&quot;:&quot;%ShortFilePath&quot;,&quot;line&quot;:&quot;%Line&quot;,&quot;func&quot;:&quot;%FuncShort&quot;,&quot;msg&quot;:%QuoteMsg}%n"/>
    <format id="common" format="%Date(2006-01-02 15:04:05 MST) | TRACE | %LEVEL | (%ShortFilePath:%Line in %FuncShort) | %Msg%n"/>
  </formats>
</seelog>
`,
			},
			{
				cfg: &config.AgentConfig{LogToConsole: true, LogFilePath: "/a/b/c"},
				out: `
<seelog minlevel="info">
  <outputs formatid="common">
    <filter levels="warn,error">
      <custom name="throttled" data-interval="0" data-console="true" data-max-per-interval="10" data-use-json="false" data-file-path="/a/b/c" />
    </filter>
    <filter levels="trace,debug,info,critical">
      <console />
      <rollingfile type="size" filename="/a/b/c" maxsize="10000000" maxrolls="5" />
    </filter>
  </outputs>
  <formats>
    <format id="json" format="{&quot;agent&quot;:&quot;trace&quot;,&quot;time&quot;:&quot;%Date(2006-01-02 15:04:05 MST)&quot;,&quot;level&quot;:&quot;%LEVEL&quot;,&quot;file&quot;:&quot;%ShortFilePath&quot;,&quot;line&quot;:&quot;%Line&quot;,&quot;func&quot;:&quot;%FuncShort&quot;,&quot;msg&quot;:%QuoteMsg}%n"/>
    <format id="common" format="%Date(2006-01-02 15:04:05 MST) | TRACE | %LEVEL | (%ShortFilePath:%Line in %FuncShort) | %Msg%n"/>
  </formats>
</seelog>
`,
			},
			{
				cfg: &config.AgentConfig{LogLevel: "warning", LogToConsole: true, LogFilePath: "/a/b/c"},
				out: `
<seelog minlevel="warn">
  <outputs formatid="common">
    <filter levels="warn,error">
      <custom name="throttled" data-interval="0" data-console="true" data-max-per-interval="10" data-use-json="false" data-file-path="/a/b/c" />
    </filter>
    <filter levels="trace,debug,info,critical">
      <console />
      <rollingfile type="size" filename="/a/b/c" maxsize="10000000" maxrolls="5" />
    </filter>
  </outputs>
  <formats>
    <format id="json" format="{&quot;agent&quot;:&quot;trace&quot;,&quot;time&quot;:&quot;%Date(2006-01-02 15:04:05 MST)&quot;,&quot;level&quot;:&quot;%LEVEL&quot;,&quot;file&quot;:&quot;%ShortFilePath&quot;,&quot;line&quot;:&quot;%Line&quot;,&quot;func&quot;:&quot;%FuncShort&quot;,&quot;msg&quot;:%QuoteMsg}%n"/>
    <format id="common" format="%Date(2006-01-02 15:04:05 MST) | TRACE | %LEVEL | (%ShortFilePath:%Line in %FuncShort) | %Msg%n"/>
  </formats>
</seelog>
`,
			},
			{
				cfg: &config.AgentConfig{LogLevel: "debug", LogToConsole: true, LogFilePath: "/a/b/c"},
				out: `
<seelog minlevel="debug">
  <outputs formatid="common">
    <filter levels="warn,error">
      <custom name="throttled" data-interval="0" data-console="true" data-max-per-interval="10" data-use-json="false" data-file-path="/a/b/c" />
    </filter>
    <filter levels="trace,debug,info,critical">
      <console />
      <rollingfile type="size" filename="/a/b/c" maxsize="10000000" maxrolls="5" />
    </filter>
  </outputs>
  <formats>
    <format id="json" format="{&quot;agent&quot;:&quot;trace&quot;,&quot;time&quot;:&quot;%Date(2006-01-02 15:04:05 MST)&quot;,&quot;level&quot;:&quot;%LEVEL&quot;,&quot;file&quot;:&quot;%ShortFilePath&quot;,&quot;line&quot;:&quot;%Line&quot;,&quot;func&quot;:&quot;%FuncShort&quot;,&quot;msg&quot;:%QuoteMsg}%n"/>
    <format id="common" format="%Date(2006-01-02 15:04:05 MST) | TRACE | %LEVEL | (%ShortFilePath:%Line in %FuncShort) | %Msg%n"/>
  </formats>
</seelog>
`,
			},
		} {
			t.Run("", func(t *testing.T) {
				assert.Equal(t, tt.out, makeLoggerConfig(tt.cfg))
			})
		}
	})
}
