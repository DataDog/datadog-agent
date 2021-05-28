// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package retry

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"sort"
	"time"

	"github.com/DataDog/datadog-agent/pkg/forwarder/transaction"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const retryTransactionsExtension = ".retry"
const retryFileFormat = "2006_01_02__15_04_05_"

type onDiskRetryQueue struct {
	serializer         *HTTPTransactionsSerializer
	storagePath        string
	diskUsageLimit     *diskUsageLimit
	filenames          []string
	currentSizeInBytes int64
	telemetry          onDiskRetryQueueTelemetry
}

func newOnDiskRetryQueue(
	serializer *HTTPTransactionsSerializer,
	storagePath string,
	diskUsageLimit *diskUsageLimit,
	telemetry onDiskRetryQueueTelemetry) (*onDiskRetryQueue, error) {

	if err := os.MkdirAll(storagePath, 0700); err != nil {
		return nil, err
	}

	storage := &onDiskRetryQueue{
		serializer:     serializer,
		storagePath:    storagePath,
		diskUsageLimit: diskUsageLimit,
		telemetry:      telemetry,
	}

	if err := storage.reloadExistingRetryFiles(); err != nil {
		return nil, err
	}

	// Check if there is an error when computing the available space
	// in this function to warn the user sooner (and not when there is an outage)
	_, err := diskUsageLimit.computeAvailableSpace(0)

	return storage, err
}

// Serialize serializes transactions to the file system.
func (s *onDiskRetryQueue) Serialize(transactions []transaction.Transaction) error {
	s.telemetry.addSerializeCount()

	// Reset the serializer in case some transactions were serialized
	// but `GetBytesAndReset` was not called because of an error.
	_, _ = s.serializer.GetBytesAndReset()

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
	s.telemetry.setFileSize(bufferSize)
	s.telemetry.setCurrentSizeInBytes(s.getCurrentSizeInBytes())
	s.telemetry.setFilesCount(s.getFilesCount())
	return nil
}

// Deserialize deserializes a transactions from the file system.
func (s *onDiskRetryQueue) Deserialize() ([]transaction.Transaction, error) {
	if len(s.filenames) == 0 {
		return nil, nil
	}
	s.telemetry.addDeserializeCount()
	index := len(s.filenames) - 1
	path := s.filenames[index]
	bytes, err := ioutil.ReadFile(path)

	// Remove the file even in case of a read failure.
	if errRemoveFile := s.removeFileAt(index); errRemoveFile != nil {
		return nil, errRemoveFile
	}

	if err != nil {
		return nil, err
	}

	transactions, errorsCount, err := s.serializer.Deserialize(bytes)
	if err != nil {
		return nil, err
	}
	s.telemetry.addDeserializeErrorsCount(errorsCount)
	s.telemetry.addDeserializeTransactionsCount(len(transactions))
	s.telemetry.setCurrentSizeInBytes(s.getCurrentSizeInBytes())
	s.telemetry.setFilesCount(s.getFilesCount())
	return transactions, err
}

// GetFileCount returns the current files count.
func (s *onDiskRetryQueue) getFilesCount() int {
	return len(s.filenames)
}

// getCurrentSizeInBytes returns the current disk space used.
func (s *onDiskRetryQueue) getCurrentSizeInBytes() int64 {
	return s.currentSizeInBytes
}

func (s *onDiskRetryQueue) makeRoomFor(bufferSize int64) error {
	maxSizeInBytes := s.diskUsageLimit.getMaxSizeInBytes()
	if bufferSize > maxSizeInBytes {
		return fmt.Errorf("The payload is too big. Current:%v Maximum:%v", bufferSize, maxSizeInBytes)
	}

	maxStorageInBytes, err := s.diskUsageLimit.computeAvailableSpace(s.currentSizeInBytes)
	if err != nil {
		return err
	}
	for len(s.filenames) > 0 && s.currentSizeInBytes+bufferSize > maxStorageInBytes {
		index := 0
		filename := s.filenames[index]
		log.Infof("Maximum disk space for retry transactions is reached. Removing %s", filename)
		if err := s.removeFileAt(index); err != nil {
			return err
		}
		s.telemetry.addFilesRemovedCount()
	}

	return nil
}

func (s *onDiskRetryQueue) removeFileAt(index int) error {
	filename := s.filenames[index]

	// Remove the file from s.filenames also in case of error to not
	// fail on the next call.
	s.filenames = append(s.filenames[:index], s.filenames[index+1:]...)

	size, err := util.GetFileSize(filename)
	if err != nil {
		return err
	}

	if err := os.Remove(filename); err != nil {
		return err
	}

	s.currentSizeInBytes -= size
	return nil
}

func (s *onDiskRetryQueue) reloadExistingRetryFiles() error {
	files, sizeInBytes, err := s.getExistingRetryFiles()
	if err != nil {
		return err
	}
	s.currentSizeInBytes = sizeInBytes

	sort.Slice(files, func(i, j int) bool {
		return files[i].ModTime().Before(files[j].ModTime())
	})
	var filenames []string
	for _, file := range files {
		fullPath := path.Join(s.storagePath, file.Name())
		filenames = append(filenames, fullPath)
	}
	s.telemetry.addReloadedRetryFilesCount(len(filenames))
	s.filenames = append(s.filenames, filenames...)
	return nil
}

func (s *onDiskRetryQueue) getExistingRetryFiles() ([]os.FileInfo, int64, error) {
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
