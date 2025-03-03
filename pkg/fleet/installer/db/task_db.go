// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package db

import (
	"encoding/json"
	"fmt"

	installerErrors "github.com/DataDog/datadog-agent/pkg/fleet/installer/errors"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"go.etcd.io/bbolt"
)

var (
	bucketTasks = []byte("tasks")
	lastTaskKey = []byte("last_task")
)

// RequestState represents the state of a task.
type RequestState struct {
	Package   string                             `json:"package"`
	ID        string                             `json:"id"`
	State     pbgo.TaskState                     `json:"state"`
	Err       error                              `json:"error,omitempty"`
	ErrorCode installerErrors.InstallerErrorCode `json:"error_code,omitempty"`
}

// TasksDB is a database that stores information about tasks.
// It is opened by the installer daemon.
type TasksDB struct {
	db *bbolt.DB
}

// New creates a new TasksDB
func NewTasksDB(dbPath string, opts ...Option) (*TasksDB, error) {
	o := options{}
	for _, opt := range opts {
		opt(&o)
	}
	db, err := bbolt.Open(dbPath, 0644, &bbolt.Options{
		Timeout:      o.timeout,
		FreelistType: bbolt.FreelistArrayType,
	})
	if err != nil {
		return nil, fmt.Errorf("could not open database: %w", err)
	}
	err = db.Update(func(tx *bbolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(bucketTasks)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("could not create tasks bucket: %w", err)
	}
	return &TasksDB{
		db: db,
	}, nil
}

// Close closes the database
func (p *TasksDB) Close() error {
	return p.db.Close()
}

// SetLastTask sets the last task
func (p *TasksDB) SetLastTask(task *RequestState) error {
	err := p.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(bucketTasks)
		if b == nil {
			return fmt.Errorf("bucket not found")
		}
		rawTask, err := json.Marshal(&task)
		if err != nil {
			return fmt.Errorf("could not marshal task: %w", err)
		}
		return b.Put(lastTaskKey, rawTask)
	})
	if err != nil {
		return fmt.Errorf("could not set task: %w", err)
	}
	return nil
}

// GetLastTask retrieves the last task
func (p *TasksDB) GetLastTask() (*RequestState, error) {
	var task *RequestState
	err := p.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(bucketPackages)
		if b == nil {
			return fmt.Errorf("bucket not found")
		}
		v := b.Get(lastTaskKey)
		if len(v) == 0 {
			// No task found, no error
			return nil
		}
		err := json.Unmarshal(v, task)
		if err != nil {
			return fmt.Errorf("could not unmarshal task: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("could not get task: %w", err)
	}
	return task, nil
}
