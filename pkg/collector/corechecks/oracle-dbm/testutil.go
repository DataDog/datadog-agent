// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle_test

package oracle

import (
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/stretchr/testify/require"
)

func initCheck(t *testing.T, senderManager sender.SenderManager, server string, port int, user string, password string, serviceName string) (Check, error) {
	c := Check{}
	rawInstanceConfig := []byte(fmt.Sprintf(`server: %s
port: %d
username: %s
password: %s
service_name: %s
`, server, port, user, password, serviceName))
	err := c.Configure(senderManager, integration.FakeConfigHash, rawInstanceConfig, []byte(``), "oracle_test")
	require.NoError(t, err)

	return c, err
}

var HOST = "localhost"
var PORT = 1521
var USER = "c##datadog"
var PASSWORD = "datadog"
var SERVICE_NAME = "XE"
var TNS_ALIAS = "XE"
var TNS_ADMIN = "/Users/nenad.noveljic/go/src/github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle-dbm/testutil/etc/netadmin"
