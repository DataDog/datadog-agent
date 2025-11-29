// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package daemon

import (
	"encoding/json"
	"errors"
	"fmt"

	"go.etcd.io/bbolt"
)

var (
	bucketTasks = []byte("tasks")
)

// taskDB is a database that stores information about tasks.
// It is opened by the installer daemon.
type taskDB struct {
	db *bbolt.DB
}

// newTaskDB creates a new TasksDB
func newTaskDB(dbPath string) (*taskDB, error) {
	db, err := bbolt.Open(dbPath, 0644, &bbolt.Options{
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
	return &taskDB{
		db: db,
	}, nil
}

// Close closes the database
func (p *taskDB) Close() error {
	return p.db.Close()
}

// SetLastTask sets the last task
func (p *taskDB) SetTaskState(task requestState) error {
	err := p.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(bucketTasks)
		if b == nil {
			return errors.New("bucket not found")
		}
		rawTask, err := json.Marshal(&task)
		if err != nil {
			return fmt.Errorf("could not marshal task: %w", err)
		}
		return b.Put([]byte(task.Package), rawTask)
	})
	if err != nil {
		return fmt.Errorf("could not set task: %w", err)
	}
	return nil
}

// GetTasksState returns the last task
func (p *taskDB) GetTasksState() (map[string]requestState, error) {
	var tasks = map[string]requestState{}
	err := p.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(bucketTasks)
		if b == nil {
			return errors.New("bucket not found")
		}
		err := b.ForEach(func(k, v []byte) error {
			var task requestState
			err := json.Unmarshal(v, &task)
			if err != nil {
				return fmt.Errorf("could not unmarshal task: %w", err)
			}
			tasks[string(k)] = task
			return nil
		})
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("could not get task: %w", err)
	}
	return tasks, nil
}
