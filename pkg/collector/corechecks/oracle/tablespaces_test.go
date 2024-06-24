// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle_test

package oracle

import (
	"database/sql"
	"fmt"
	"testing"

	//"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"

	go_ora "github.com/sijms/go-ora/v2"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

/*
 * Prerequisites:
 * create tablespace tbs_test datafile '/opt/oracle/oradata/XE/tbs_test01.dbf' size 100M ;
 */

func TestTablespaces(t *testing.T) {
	c, s := newDefaultCheck(t, "", "")
	defer c.Teardown()
	err := c.Run()
	require.NoError(t, err)
	s.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	var expectedPdb string
	if c.connectedToPdb {
		expectedPdb = c.cdbName
	} else {
		expectedPdb = "cdb$root"
	}
	tags := []string{fmt.Sprintf("pdb:%s", expectedPdb), "tablespace:TBS_TEST"}
	s.AssertMetricOnce(t, "Gauge", "oracle.tablespace.size", 104857600, c.dbHostname, tags)
	s.AssertMetricOnce(t, "Gauge", "oracle.tablespace.offline", 0, c.dbHostname, tags)
}

func TestTablespacesOffline(t *testing.T) {
	c, s := newDefaultCheck(t, "", "")
	defer c.Teardown()

	connection := getConnectData(t, useSysUser)
	databaseUrl := go_ora.BuildUrl(connection.Server, connection.Port, connection.ServiceName, connection.Username, connection.Password, nil)
	conn, err2 := sql.Open("oracle", databaseUrl)
	require.NoError(t, err2)

	defer func() {
		_, err := conn.Exec("ALTER TABLESPACE TBS_TEST ONLINE")
		require.NoError(t, err)
		conn.Close()
	}()

	_, err3 := conn.Exec("ALTER TABLESPACE TBS_TEST OFFLINE")
	require.NoError(t, err3)

	err := c.Run()
	require.NoError(t, err)
	s.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	var expectedPdb string
	if c.connectedToPdb {
		expectedPdb = c.cdbName
	} else {
		expectedPdb = "cdb$root"
	}
	tags := []string{fmt.Sprintf("pdb:%s", expectedPdb), "tablespace:TBS_TEST"}
	s.AssertMetricOnce(t, "Gauge", "oracle.tablespace.offline", 1, c.dbHostname, tags)
}
