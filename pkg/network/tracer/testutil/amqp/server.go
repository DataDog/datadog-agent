package amqp

import (
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	"github.com/stretchr/testify/require"
	"os"
	"os/exec"
	"testing"
)

func RunAmqp(t *testing.T, serverAddr string) {
	t.Helper()

	env := []string{
		"AMQP_ADDR=" + serverAddr,
	}
	dir, _ := testutil.CurDir()
	cmd := exec.Command("docker-compose", "-f", dir+"/testdata/docker-compose.yml", "up", "-d")
	cmd.Stderr = os.Stdout
	cmd.Stdout = os.Stdout
	cmd.Env = append(cmd.Env, env...)
	require.NoErrorf(t, cmd.Run(), "could not start amqp with docker-compose")

	t.Cleanup(func() {
		c := exec.Command("docker-compose", "-f", dir+"/testdata/docker-compose.yml", "down", "--remove-orphans")
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		c.Env = append(c.Env, env...)
		_ = c.Run()
	})
}
