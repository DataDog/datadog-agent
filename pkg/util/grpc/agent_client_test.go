package grpc

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/cihub/seelog"
	"github.com/stretchr/testify/assert"
)

func TestMain(m *testing.M) {
	log.SetupLogger(seelog.Default, "trace")
	os.Exit(m.Run())
}

func TestGetDDAgentClientTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err := GetDDAgentClient(ctx)
	assert.Equal(t, context.DeadlineExceeded, err)
}

func TestGetDDAgentClientWithCmdPort0(t *testing.T) {
	os.Setenv("DD_CMD_PORT", "-1")
	defer os.Unsetenv("DD_CMD_PORT")
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	_, err := GetDDAgentClient(ctx)
	assert.NotNil(t, err)
	assert.Equal(t, "grpc client disabled via cmd_port: -1", err.Error())
}
