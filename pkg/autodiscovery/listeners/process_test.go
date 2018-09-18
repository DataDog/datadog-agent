// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package listeners

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func getMockProcessService() *ProcessService {
	return &ProcessService{
		hostname:      "127.0.0.1",
		adIdentifiers: []string{"redisdb", "redis", "redis-server"},
		ports:         []ContainerPort{{Port: 8080}},
		pid:           12345,
		name:          "redisdb",
	}
}

func TestProcGetEntity(t *testing.T) {
	svc := getMockProcessService()

	assert.Equal(t, "12345:redisdb", svc.GetEntity())
	svc.pid = 12346
	assert.Equal(t, "12346:redisdb", svc.GetEntity())
}

func TestProcGetADIdentifiers(t *testing.T) {
	svc := getMockProcessService()

	ids, err := svc.GetADIdentifiers()
	assert.NoError(t, err)

	assert.Equal(t, []string{"redisdb", "redis", "redis-server"}, ids)
}

func TestProcGetHosts(t *testing.T) {
	svc := getMockProcessService()

	hosts, err := svc.GetHosts()
	assert.NoError(t, err)

	host, ok := hosts["host"]
	assert.True(t, ok)

	assert.Equal(t, "127.0.0.1", host)
}

func TestProcGetPorts(t *testing.T) {
	svc := getMockProcessService()

	ports, err := svc.GetPorts()
	assert.NoError(t, err)

	assert.Equal(t, 1, len(ports))
	assert.Equal(t, []ContainerPort{{Port: 8080}}, ports)
}

func TestProcGetPid(t *testing.T) {
	svc := getMockProcessService()

	pid, err := svc.GetPid()
	assert.NoError(t, err)

	assert.Equal(t, 12345, pid)
}

func TestProcGetHostname(t *testing.T) {
	svc := getMockProcessService()

	hostname, err := svc.GetHostname()
	assert.NoError(t, err)

	assert.Equal(t, "127.0.0.1", hostname)
}

func TestProcGetTags(t *testing.T) {
	svc := getMockProcessService()

	tags, err := svc.GetTags()
	assert.NoError(t, err)

	assert.Equal(t, []string{}, tags)
}
