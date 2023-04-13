// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle
// +build oracle

package oracle

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle/common"
	"github.com/DataDog/datadog-agent/pkg/obfuscate"
	"github.com/stretchr/testify/assert"
)

func TestObfuscator(t *testing.T) {
	obfuscatorOptions := obfuscate.SQLConfig{}
	obfuscatorOptions.DBMS = common.IntegrationName
	obfuscatorOptions.TableNames = true
	obfuscatorOptions.CollectCommands = true
	obfuscatorOptions.CollectComments = true

	o := obfuscate.NewObfuscator(obfuscate.Config{SQL: obfuscatorOptions})
	for _, statement := range []string{
		// needs https://datadoghq.atlassian.net/browse/DBM-2295
		`UPDATE /* comment */ SET t n=1`,

		`SELECT /* comment */ from dual`} {
		obfuscatedStatement, err := o.ObfuscateSQLString(statement)
		assert.NoError(t, err, "obfuscator error")
		assert.NotContains(t, obfuscatedStatement.Query, "comment", "comment wasn't removed by the obfuscator")
	}
}
