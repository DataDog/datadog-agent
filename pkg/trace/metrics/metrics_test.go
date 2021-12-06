package metrics

import (
	"testing"

	mainconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/trace/config"

	"github.com/stretchr/testify/assert"
)

func TestFindAddr(t *testing.T) {
	t.Run("pipe", func(t *testing.T) {
		defer func(old string) {
			mainconfig.Datadog.Set("dogstatsd_pipe_name", old)
		}(mainconfig.Datadog.GetString("dogstatsd_pipe_name"))
		mainconfig.Datadog.Set("dogstatsd_pipe_name", "sock.pipe")

		addr, err := findAddr(&config.AgentConfig{})
		assert.NoError(t, err)
		assert.Equal(t, addr, `\\.\pipe\sock.pipe`)
	})

	t.Run("tcp", func(t *testing.T) {
		addr, err := findAddr(&config.AgentConfig{
			StatsdHost: "localhost",
			StatsdPort: 123,
		})
		assert.NoError(t, err)
		assert.Equal(t, addr, `localhost:123`)
	})

	t.Run("socket", func(t *testing.T) {
		defer func(old string) {
			mainconfig.Datadog.Set("dogstatsd_socket", old)
		}(mainconfig.Datadog.GetString("dogstatsd_socket"))
		mainconfig.Datadog.Set("dogstatsd_socket", "pipe.sock")
		addr, err := findAddr(&config.AgentConfig{})
		assert.NoError(t, err)
		assert.Equal(t, addr, `unix://pipe.sock`)
	})

	t.Run("error", func(t *testing.T) {
		_, err := findAddr(&config.AgentConfig{})
		assert.Error(t, err)
	})
}
