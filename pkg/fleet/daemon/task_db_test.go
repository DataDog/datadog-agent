// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package daemon

import (
	"path/filepath"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/stretchr/testify/assert"
)

func TestSetTaskState(t *testing.T) {
	db, err := newTaskDB(filepath.Join(t.TempDir(), "test.db"))
	assert.NoError(t, err)
	defer db.Close()

	task1 := requestState{
		Package:   "test-package1",
		ID:        "test-id1",
		State:     core.TaskState_ERROR,
		Err:       "test-error",
		ErrorCode: 1,
	}
	err = db.SetTaskState(task1)
	assert.NoError(t, err)

	task2 := requestState{
		Package: "test-package2",
		ID:      "test-id2",
		State:   core.TaskState_DONE,
	}
	err = db.SetTaskState(task2)
	assert.NoError(t, err)

	tasks, err := db.GetTasksState()
	assert.NoError(t, err)
	assert.Len(t, tasks, 2)
	assert.Equal(t, task1, tasks["test-package1"])
	assert.Equal(t, task2, tasks["test-package2"])
}
