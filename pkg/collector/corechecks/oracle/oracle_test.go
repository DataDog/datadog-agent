// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package oracle

import (
	"database/sql"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	go_ora "github.com/sijms/go-ora/v2"

	_ "github.com/godror/godror"
	"github.com/jmoiron/sqlx"
)

var HOST = "localhost"
var PORT = 1521
var USER = "c##datadog"
var PASSWORD = "datadog"
var DB = "XE"

func TestBasic(t *testing.T) {
	chk := Check{}

	// language=yaml
	rawInstanceConfig := []byte(`
server: localhost,1521
username: datadog
password: datadog
service_name: XE
`)

	err := chk.Configure(integration.FakeConfigHash, rawInstanceConfig, []byte(``), "oracle_test")
	assert.NoError(t, err)
}

func TestConnectionGoOra(t *testing.T) {
	//databaseUrl := go_ora.BuildUrl("localhost", 1521, "XE", "c##datadog", "datadog", nil)
	databaseUrl := go_ora.BuildUrl(HOST, PORT, DB, USER, PASSWORD, nil)
	conn, err := sql.Open("oracle", databaseUrl)
	assert.NoError(t, err)

	err = conn.Ping()
	assert.NoError(t, err)

}

func TestConnectionGodror(t *testing.T) {
	databaseUrl := fmt.Sprintf("%s/%s@%s:%d/%s", USER, PASSWORD, HOST, PORT, DB)
	_, err := sqlx.Connect("godror", databaseUrl)
	assert.NoError(t, err)
}
