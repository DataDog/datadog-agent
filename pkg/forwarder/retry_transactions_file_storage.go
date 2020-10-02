// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package forwarder

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/compression"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const retryTransactionsExtension = ".retry"

type retryTransactionsFileStorage struct {
	storagePath       string
	files             []string
	currentSize       int64
	maxSize           int64
	compressionBuffer []byte
}

func newRetryTransactionsFileStorage(runPath string, maxSize int64) *retryTransactionsFileStorage {
	storagePath := path.Join(runPath, "retry_transactions")

	// Returns an error when path already exists
	_ = os.MkdirAll(storagePath, 0755)

	return &retryTransactionsFileStorage{
		storagePath: storagePath,
		maxSize:     maxSize,
	}
}

// Serialize serializes transactions to the file system.
func (f *retryTransactionsFileStorage) Serialize(payloadsToFlush []Transaction) error {
	data, err := json.Marshal(payloadsToFlush)
	if err != nil {
		return err
	}

	// Note: The file content can be displayed with the command `pigz -d < FILE_PATH`
	if f.compressionBuffer, err = compression.Compress(f.compressionBuffer, data); err != nil {
		return err
	}

	bufferSize := int64(len(f.compressionBuffer))

	if err = f.makeRoomFor(bufferSize); err != nil {
		return err
	}

	filename := time.Now().UTC().Format("2006_01_02__15_04_05_")
	file, err := ioutil.TempFile(f.storagePath, filename+"*"+retryTransactionsExtension)
	if err != nil {
		return err
	}
	if _, err = file.Write(f.compressionBuffer); err != nil {
		_ = file.Close()
		_ = os.Remove(file.Name())
		return err
	}
	defer file.Close()
	stat, err := file.Stat()

	if err != nil {
		return err
	}
	f.currentSize += stat.Size()

	f.files = append(f.files, file.Name())
	return nil
}

// DeserializeLast deserializes the last transactions from the file system.
func (f *retryTransactionsFileStorage) DeserializeLast() ([]Transaction, error) {
	if len(f.files) == 0 {
		return nil, nil
	}

	path := f.files[len(f.files)-1]

	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	f.compressionBuffer, err = compression.Decompress(f.compressionBuffer, data)
	if err != nil {
		return nil, err
	}

	// The code supports only HTTPTransaction
	var httpTransactions []*HTTPTransaction
	if err := json.Unmarshal(f.compressionBuffer, &httpTransactions); err != nil {
		return nil, err
	}

	if err = os.Remove(path); err != nil {
		return nil, err
	}

	f.files = f.files[:len(f.files)-1]

	var transactions []Transaction
	for _, t := range httpTransactions {
		// FIXME: Restore the proper handlers
		t.attemptHandler = defaultAttemptHandler
		t.completionHandler = defaultCompletionHandler
		transactions = append(transactions, t)
	}
	return transactions, err
}

// Stop stops and removes files.
func (f *retryTransactionsFileStorage) Stop() error {
	f.files = nil
	return f.cleanFiles()
}

// GetFileCount returns the current file count.
func (f *retryTransactionsFileStorage) GetFileCount() int {
	return len(f.files)
}

func (f *retryTransactionsFileStorage) cleanFiles() error {
	files, err := ioutil.ReadDir(f.storagePath)
	if err != nil {
		return err
	}
	for _, file := range files {
		if file.Mode().IsRegular() && filepath.Ext(file.Name()) == retryTransactionsExtension {
			fullPath := path.Join(f.storagePath, file.Name())
			if err = os.Remove(fullPath); err != nil {
				return err
			}
		}
	}
	return nil
}

func (f *retryTransactionsFileStorage) makeRoomFor(bufferSize int64) error {
	for len(f.files) > 0 && f.currentSize+bufferSize > f.maxSize {
		filename := f.files[0]

		size, err := util.GetFileSize(filename)
		if err != nil {
			return err
		}

		log.Infof("Maximum disk space for retry transactions is reached. Removing %s", filename)
		if err := os.Remove(filename); err != nil {
			return err
		}

		f.currentSize -= size
		f.files = f.files[1:]
	}

	if f.currentSize+bufferSize > f.maxSize {
		return fmt.Errorf("Payload are too big. Current:%v Maximum:%v", bufferSize, f.maxSize)
	}

	return nil
}
