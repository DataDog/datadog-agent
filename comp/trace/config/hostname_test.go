package config

import (
	"context"
	"testing"
	"time"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/grpc"
	"github.com/stretchr/testify/assert"
)

func TestGetDDAgentClientTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	ipcAddress, err := pkgconfigsetup.GetIPCAddress(pkgconfigsetup.Datadog())
	assert.NoError(t, err)

	_, err = grpc.GetDDAgentClient(ctx, ipcAddress, "5001")
	assert.Equal(t, context.DeadlineExceeded, err)
}
