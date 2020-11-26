// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package forwarder

import (
	"fmt"
	"io/ioutil"
	"os"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const retryTransactionsExtension = ".retry"

type transactionsFileStorage struct {
	serializer         *TransactionsSerializer
	storagePath        string
	maxSizeInBytes     int64
	filenames          []string
	currentSizeInBytes int64
}

func newTransactionsFileStorage(serializer *TransactionsSerializer, storagePath string, maxSizeInBytes int64) *transactionsFileStorage {
	// Do not return an error when the path already exists
	_ = os.MkdirAll(storagePath, 0755)

	return &transactionsFileStorage{
		serializer:     serializer,
		storagePath:    storagePath,
		maxSizeInBytes: maxSizeInBytes,
	}
}

// Serialize serializes transactions to the file system.
func (s *transactionsFileStorage) Serialize(transactions []Transaction) error {
	for _, t := range transactions {
		if err := t.SerializeTo(s.serializer); err != nil {
			return err
		}
	}

	bytes, err := s.serializer.GetBytesAndReset()
	if err != nil {
		return err
	}
	bufferSize := int64(len(bytes))

	if err := s.makeRoomFor(bufferSize); err != nil {
		return err
	}

	filename := time.Now().UTC().Format("2006_01_02__15_04_05_")
	file, err := ioutil.TempFile(s.storagePath, filename+"*"+retryTransactionsExtension)
	if err != nil {
		return err
	}
	if _, err = file.Write(bytes); err != nil {
		_ = file.Close()
		_ = os.Remove(file.Name())
		return err
	}
	defer file.Close()

	s.currentSizeInBytes += bufferSize
	s.filenames = append(s.filenames, file.Name())
	return nil
}

// Deserialize deserializes a transactions from the file system.
func (s *transactionsFileStorage) Deserialize() ([]Transaction, error) {
	if len(s.filenames) == 0 {
		return nil, nil
	}
	index := len(s.filenames) - 1
	path := s.filenames[index]
	bytes, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	transactions, err := s.serializer.Deserialize(bytes)
	if err != nil {
		return nil, err
	}

	if err := s.removeFileAt(index); err != nil {
		return nil, err
	}

	return transactions, err
}

// GetFileCount returns the current files count.
func (s *transactionsFileStorage) GetFilesCount() int {
	return len(s.filenames)
}

// GetCurrentSizeInBytes returns the current disk space used.
func (s *transactionsFileStorage) GetCurrentSizeInBytes() int64 {
	return s.currentSizeInBytes
}

func (s *transactionsFileStorage) makeRoomFor(bufferSize int64) error {
	for len(s.filenames) > 0 && s.currentSizeInBytes+bufferSize > s.maxSizeInBytes {
		index := 0
		filename := s.filenames[index]
		log.Infof("Maximum disk space for retry transactions is reached. Removing %s", filename)
		if err := s.removeFileAt(index); err != nil {
			return err
		}
	}

	if s.currentSizeInBytes+bufferSize > s.maxSizeInBytes {
		return fmt.Errorf("The payload is too big. Current:%v Maximum:%v", bufferSize, s.maxSizeInBytes)
	}

	return nil
}

func (s *transactionsFileStorage) removeFileAt(index int) error {
	filename := s.filenames[index]

	size, err := util.GetFileSize(filename)
	if err != nil {
		return err
	}

	if err := os.Remove(filename); err != nil {
		return err
	}

	s.currentSizeInBytes -= size
	s.filenames = append(s.filenames[:index], s.filenames[index+1:]...)
	return nil
}
