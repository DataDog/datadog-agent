package resources

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestGetPayload(t *testing.T) {

	hostname := "foo"
	processesPayload := GetPayload(hostname)

	assert.NotNil(t, processesPayload.Processes["snaps"])
	assert.Equal(t, hostname, processesPayload.Meta["host"])
}
