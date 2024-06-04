// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle_test

package oracle

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestActivity(t *testing.T) {
	// include_all_sessions is set to true because the session executing the test query
	// won't be active during sampling
	config := `dbm: true
query_samples:
  include_all_sessions: true`
	c, _ := newDefaultCheck(t, config, "")
	defer c.Teardown()

	var err error
	err = c.Run()
	assert.NoError(t, err, "check run activity")

	largeMultibyteString := strings.Repeat("안녕하세요", 200)
	filter := fmt.Sprintf("user='%s'", largeMultibyteString)
	andClause := strings.Repeat(fmt.Sprintf(" and %s", filter), 100)
	filter = filter + andClause
	statement := fmt.Sprintf("select 14 from dual where %s", filter)
	// we aren't scanning rows to force the session keep the cursor open, so
	// the test query sql_id will be stored in prev_sql_id
	rows, err := c.db.Query(statement)
	defer rows.Close()
	assert.NoError(t, err, "long query didn't run")

	err = c.SampleSession()
	assert.NoError(t, err, "activity run - multibyte characters")
}
