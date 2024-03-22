// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle_test

package oracle

import (
	"testing"

	//"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

/*
 * Prerequisites:
 * create tablespace tbs_test datafile '/opt/oracle/oradata/XE/tbs_test01.dbf' size 100M ;
 * alter session set container=xepdb1 ;
 * create tablespace tbs_test datafile '/opt/oracle/oradata/XE/XEPDB1/tbs_test01.dbf' size 200M;
 */

func TestTablespaces(t *testing.T) {
	tags := []string{"pdb:cdb$root", "tablespace:TBS_TEST"}
	c, s := newRealCheck(t, "")
	err := c.Run()
	require.NoError(t, err)
	s.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	s.AssertMetricOnce(t, "Gauge", "oracle.tablespace.size", 104857600, c.dbHostname, tags)
}
