// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package retry

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"time"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
	"github.com/DataDog/datadog-agent/pkg/util/filesystem"
)

const retryTransactionsExtension = ".retry"
const retryFileFormat = "2006_01_02__15_04_05_"

type onDiskRetryQueue struct {
	log                 log.Component
	serializer          *HTTPTransactionsSerializer
	storagePath         string
	diskUsageLimit      *DiskUsageLimit
	filenames           []string
	currentSizeInBytes  int64
	telemetry           onDiskRetryQueueTelemetry
	pointCountTelemetry *PointCountTelemetry
}

func newOnDiskRetryQueue(
	log log.Component,
	serializer *HTTPTransactionsSerializer,
	storagePath string,
	diskUsageLimit *DiskUsageLimit,
	telemetry onDiskRetryQueueTelemetry,
	pointCountTelemetry *PointCountTelemetry) (*onDiskRetryQueue, error) {

	if err := os.MkdirAll(storagePath, 0700); err != nil {
		return nil, err
	}

	storage := &onDiskRetryQueue{
		log:                 log,
		serializer:          serializer,
		storagePath:         storagePath,
		diskUsageLimit:      diskUsageLimit,
		telemetry:           telemetry,
		pointCountTelemetry: pointCountTelemetry,
	}

	if err := storage.reloadExistingRetryFiles(); err != nil {
		return nil, err
	}

	// Check if there is an error when computing the available space
	// in this function to warn the user sooner (and not when there is an outage)
	_, err := diskUsageLimit.computeAvailableSpace(0)

	return storage, err
}

// Store stores transactions to the file system.
func (s *onDiskRetryQueue) Store(transactions []transaction.Transaction) error {
	s.telemetry.addSerializeCount()

	// Reset the serializer in case some transactions were serialized
	// but `GetBytesAndReset` was not called because of an error.
	_, _ = s.serializer.GetBytesAndReset()

	for _, t := range transactions {
		if err := t.SerializeTo(s.log, s.serializer); err != nil {
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
	file, err := os.CreateTemp(s.storagePath, filename+"*"+retryTransactionsExtension)
	if err != nil {
		return err
	}
	if _, err = file.Write(bytes); err != nil {
		_ = file.Close()
		_ = os.Remove(file.Name())
		return err
	}
	err = file.Close()
	if err != nil {
		_ = os.Remove(file.Name())
		return err
	}
	s.currentSizeInBytes += bufferSize
	s.filenames = append(s.filenames, file.Name())
	s.telemetry.setFileSize(bufferSize)
	s.telemetry.setCurrentSizeInBytes(s.GetDiskSpaceUsed())
	s.telemetry.setFilesCount(s.getFilesCount())
	return nil
}

// ExtractLast extracts the last transactions stored.
func (s *onDiskRetryQueue) ExtractLast() ([]transaction.Transaction, error) {
	if len(s.filenames) == 0 {
		return nil, nil
	}
	s.telemetry.addDeserializeCount()
	index := len(s.filenames) - 1
	path := s.filenames[index]
	bytes, err := os.ReadFile(path)

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
	s.telemetry.setCurrentSizeInBytes(s.GetDiskSpaceUsed())
	s.telemetry.setFilesCount(s.getFilesCount())
	return transactions, err
}

// GetFileCount returns the current files count.
func (s *onDiskRetryQueue) getFilesCount() int {
	return len(s.filenames)
}

// GetDiskSpaceUsed() returns the current disk space used.
func (s *onDiskRetryQueue) GetDiskSpaceUsed() int64 {
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
		s.log.Errorf("Maximum disk space for retry transactions is reached. Removing %s", filename)

		bytes, err := os.ReadFile(filename)
		if err != nil {
			s.log.Errorf("Cannot read the file %v: %v", filename, err)
		} else if transactions, _, errDeserialize := s.serializer.Deserialize(bytes); errDeserialize == nil {
			pointDroppedCount := 0
			for _, tr := range transactions {
				pointDroppedCount += tr.GetPointCount()
			}
			s.onPointDropped(pointDroppedCount)
		} else {
			s.log.Errorf("Cannot deserialize the content of file %v: %v", filename, errDeserialize)
		}

		if err := s.removeFileAt(index); err != nil {
			return err
		}
		s.telemetry.addFilesRemovedCount()
	}

	return nil
}

func (s *onDiskRetryQueue) onPointDropped(count int) {
	s.telemetry.addPointDroppedCount(count)
	s.pointCountTelemetry.OnPointDropped(count)
}

func (s *onDiskRetryQueue) removeFileAt(index int) error {
	filename := s.filenames[index]

	// Remove the file from s.filenames also in case of error to not
	// fail on the next call.
	s.filenames = slices.Delete(s.filenames, index, index+1)

	size, err := filesystem.GetFileSize(filename)
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
	s.telemetry.setReloadedRetryFilesCount(len(filenames))
	s.filenames = append(s.filenames, filenames...)
	return nil
}

func (s *onDiskRetryQueue) getExistingRetryFiles() ([]os.FileInfo, int64, error) {
	entries, err := os.ReadDir(s.storagePath)
	if err != nil {
		return nil, 0, err
	}
	var files []os.FileInfo
	currentSizeInBytes := int64(0)
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			s.log.Warn("Can't get file info", err)
			continue
		}

		if info.Mode().IsRegular() && filepath.Ext(entry.Name()) == retryTransactionsExtension {
			currentSizeInBytes += info.Size()
			files = append(files, info)
		}
	}
	return files, currentSizeInBytes, nil
}
