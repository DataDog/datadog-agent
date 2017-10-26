// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.
// +build !windows

package system

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	log "github.com/cihub/seelog"
)

var (
	samplecontent1 = []byte("896\t0\t101478\n") // 0.008829499990145647
	samplecontent2 = []byte("800\t0\t101478\n") // 0.007883482134058614
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
	return ioutil.TempFile("", "file-nr")
}

func TestFhCheckLinux(t *testing.T) {

	tmpFile, err := getFileNr()
	if err != nil {
		t.Fatalf("unable to create temporary file-nr file: %v", err)
	}
	defer os.Remove(tmpFile.Name()) // clean up

	fileNrHandle = writeSampleFile(tmpFile, samplecontent1)
	t.Logf("Testing from file %s", fileNrHandle) // To pass circle ci tests

	fileHandleCheck := new(fhCheck)
	fileHandleCheck.Configure(nil, nil)

	mock := mocksender.NewMockSender(fileHandleCheck.ID())

	mock.On("Gauge", "system.fs.file_handles.in_use", 0.008829499990145647, "", []string(nil)).Return().Times(1)
	mock.On("Commit").Return().Times(1)
	fileHandleCheck.Run()

	mock.AssertExpectations(t)
	mock.AssertNumberOfCalls(t, "Gauge", 1)
	mock.AssertNumberOfCalls(t, "Commit", 1)

	tmpFile, err = getFileNr()
	if err != nil {
		t.Fatalf("unable to create temporary file-nr file: %v", err)
	}
	defer os.Remove(tmpFile.Name()) // clean up

	fileNrHandle = writeSampleFile(tmpFile, samplecontent2)
	t.Logf("Testing from file %s", fileNrHandle) // To pass circle ci tests

	mock.On("Gauge", "system.fs.file_handles.in_use", 0.007883482134058614, "", []string(nil)).Return().Times(1)
	mock.On("Commit").Return().Times(1)
	fileHandleCheck.Run()

	mock.AssertExpectations(t)
	mock.AssertNumberOfCalls(t, "Gauge", 2)
	mock.AssertNumberOfCalls(t, "Commit", 2)
}
