// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build !windows

package filehandles

import (
	"os"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	samplecontent1 = []byte("896\t201\t101478\n") // 0.008829499990145647
	samplecontent2 = []byte("800\t453\t101478\n") // 0.007883482134058614
)

func writeSampleFile(f *os.File, content []byte) string {
	if _, err := f.Write(content); err != nil {
		log.Debugf("error: %v", err)
	}
	if err := f.Close(); err != nil {
		log.Debugf("error: %v", err)
	}
	return f.Name()
}

func getFileNr() (f *os.File, err error) {
	return os.CreateTemp("", "file-nr")
}

func TestFhCheckLinux(t *testing.T) {
	tmpFile, err := getFileNr()
	if err != nil {
		t.Fatalf("unable to create temporary file-nr file: %v", err)
	}
	defer os.Remove(tmpFile.Name()) // clean up

	fileNrHandle = writeSampleFile(tmpFile, samplecontent1)
	t.Logf("Testing from file %s", fileNrHandle) // To pass circle ci tests

	// we have to init the mocked sender here before fileHandleCheck.Configure(mock.GetSenderManager(), integration.FakeConfigHash, ...)
	// (and append it to the aggregator, which is automatically done in NewMockSender)
	// because the FinalizeCheckServiceTag is called in Configure.
	// Hopefully, the check ID is an empty string while running unit tests;
	mock := mocksender.NewMockSender("")
	mock.On("FinalizeCheckServiceTag").Return()

	fileHandleCheck := new(fhCheck)
	fileHandleCheck.Configure(mock.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test")

	// reset the check ID for the sake of correctness
	mocksender.SetSender(mock, fileHandleCheck.ID())

	mock.On("Gauge", "system.fs.file_handles.allocated", 896.0, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.fs.file_handles.allocated_unused", 201.0, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.fs.file_handles.in_use", 0.006848775103963421, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.fs.file_handles.used", 695.0, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.fs.file_handles.max", 101478.0, "", []string(nil)).Return().Times(1)
	mock.On("Commit").Return().Times(1)
	fileHandleCheck.Run()

	mock.AssertExpectations(t)
	mock.AssertNumberOfCalls(t, "Gauge", 5)
	mock.AssertNumberOfCalls(t, "Commit", 1)

	tmpFile, err = getFileNr()
	if err != nil {
		t.Fatalf("unable to create temporary file-nr file: %v", err)
	}
	defer os.Remove(tmpFile.Name()) // clean up

	fileNrHandle = writeSampleFile(tmpFile, samplecontent2)
	t.Logf("Testing from file %s", fileNrHandle) // To pass circle ci tests

	mock.On("Gauge", "system.fs.file_handles.allocated", 800.0, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.fs.file_handles.allocated_unused", 453.0, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.fs.file_handles.in_use", 0.003419460375647924, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.fs.file_handles.used", 347.0, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.fs.file_handles.max", 101478.0, "", []string(nil)).Return().Times(1)
	mock.On("Commit").Return().Times(1)
	fileHandleCheck.Run()

	mock.AssertExpectations(t)
	mock.AssertNumberOfCalls(t, "Gauge", 10)
	mock.AssertNumberOfCalls(t, "Commit", 2)
}
