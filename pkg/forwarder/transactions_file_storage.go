// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package forwarder

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"sort"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const retryTransactionsExtension = ".retry"
const retryFileFormat = "2006_01_02__15_04_05_"

type transactionsFileStorage struct {
	serializer         *TransactionsSerializer
	storagePath        string
	maxSizeInBytes     int64
	filenames          []string
	currentSizeInBytes int64
}

func newTransactionsFileStorage(
	serializer *TransactionsSerializer,
	storagePath string,
	maxSizeInBytes int64) (*transactionsFileStorage, error) {

	if err := os.MkdirAll(storagePath, 0755); err != nil {
		return nil, err
	}

	storage := &transactionsFileStorage{
		serializer:     serializer,
		storagePath:    storagePath,
		maxSizeInBytes: maxSizeInBytes,
	}

	if err := storage.reloadExistingRetryFiles(); err != nil {
		return nil, err
	}
	return storage, nil
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

	filename := time.Now().UTC().Format(retryFileFormat)
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

	if err := s.removeFileAt(index); err != nil {
		return nil, err
	}

	transactions, err := s.serializer.Deserialize(bytes)
	if err != nil {
		return nil, err
	}

	return transactions, err
}

// GetFileCount returns the current files count.
func (s *transactionsFileStorage) getFilesCount() int {
	return len(s.filenames)
}

// getCurrentSizeInBytes returns the current disk space used.
func (s *transactionsFileStorage) getCurrentSizeInBytes() int64 {
	return s.currentSizeInBytes
}

func (s *transactionsFileStorage) makeRoomFor(bufferSize int64) error {
	if bufferSize > s.maxSizeInBytes {
		return fmt.Errorf("The payload is too big. Current:%v Maximum:%v", bufferSize, s.maxSizeInBytes)
	}

	for len(s.filenames) > 0 && s.currentSizeInBytes+bufferSize > s.maxSizeInBytes {
		index := 0
		filename := s.filenames[index]
		log.Infof("Maximum disk space for retry transactions is reached. Removing %s", filename)
		if err := s.removeFileAt(index); err != nil {
			return err
		}
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

func (s *transactionsFileStorage) reloadExistingRetryFiles() error {
	files, sizeInBytes, err := s.getExistingRetryFiles()
	if err != nil {
		return err
	}
	s.currentSizeInBytes = sizeInBytes

	sort.Slice(files, func(i, j int) bool {
		return files[i].ModTime().Before(files[j].ModTime())
	})

	for _, file := range files {
		fullPath := path.Join(s.storagePath, file.Name())
		s.filenames = append(s.filenames, fullPath)
	}
	return nil
}

func (s *transactionsFileStorage) getExistingRetryFiles() ([]os.FileInfo, int64, error) {
	entries, err := ioutil.ReadDir(s.storagePath)
	if err != nil {
		return nil, 0, err
	}
	var files []os.FileInfo
	currentSizeInBytes := int64(0)
	for _, entry := range entries {
		if entry.Mode().IsRegular() && filepath.Ext(entry.Name()) == retryTransactionsExtension {
			currentSizeInBytes += entry.Size()
			files = append(files, entry)
		}
	}
	return files, currentSizeInBytes, nil
}
