// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package remote

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/flare"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/mholt/archiver"
	"github.com/rs/xid"
)

type RemoteFlare struct {
	flareId string
	tempDir string
	zipPath string
	ts      int64 // collect until this timestamp
	files   map[string]*os.File
	sources map[string]*RegisteredSource
}

var (
	currentFlare *RemoteFlare = nil
	mutex        sync.RWMutex
)

func NewRemoteFlare(path, zip string, d time.Duration) *RemoteFlare {
	return &RemoteFlare{
		flareId: xid.New().String(),
		tempDir: path,
		zipPath: zip,
		ts:      time.Now().Add(d).Unix(),
		files:   map[string]*os.File{},
		sources: map[string]*RegisteredSource{},
	}
}

func (f RemoteFlare) GetFile(id string) (*os.File, error) {
	var fp *os.File

	mutex.RLock()
	if fp, ok := f.files[id]; ok {
		mutex.RUnlock()
		return fp, nil
	}
	mutex.RUnlock()

	mutex.Lock()
	defer mutex.Unlock()

	logPath := path.Join(f.tempDir, fmt.Sprintf("%s.log", id))
	fp, err := os.Create(logPath)
	if err != nil {
		return nil, err
	}

	f.files[id] = fp

	return fp, nil
}

func (f RemoteFlare) wrapUp(d time.Duration) error {
	defer os.RemoveAll(f.tempDir)

	mutex.Lock()
	currentFlare = nil
	mutex.Unlock()

	for _, fp := range f.files {
		fp.Close()
	}

	err := archiver.Zip.Make(f.zipPath, []string{f.tempDir})
	if err != nil {
		return "", err
	}

}

func GetCurrentFlareId() (string, error) {
	mutex.RLock()
	defer mutex.RUnlock()
	if currentFlare == nil {
		return "", errors.New("no ongoing flare")
	}

	return currentFlare.flareId, nil
}

func CreateRemoteFlareArchive(tracerId, svc, env string, d time.Duration) (*RemoteFlare, error) {
	mutex.Lock()
	defer mutex.Unlock()
	if currentFlare != nil {
		return currentFlare, OngoingFlareError{}
	}

	tempDir, err := flare.CreateTempDir()
	if err != nil {
		return nil, err
	}

	currentFlare = NewRemoteFlare(tempDir, d)
	if tracerId != "" {
		src, ok := GetSourceById(tracerId)
		if ok {
			currentFlare.sources[src.Id] = src
		}
	} else {
		currentFlare.sources = GetSourcesForServiceAndEnv(svc, env)
	}

	// do this somewhere else
	// defer os.RemoveAll(tempDir)

}

func LogEntry(flareId, tracerId string, data io.ReadCloser) error {
	mutex.RLock()
	if currentFlare == nil {
		mutex.RUnlock()
		return InvalidFlareId{}
	}

	if flareId != currentFlare.flareId {
		return errors.New("")
	}
	mutex.RUnlock()

	fp, err := currentFlare.GetFile(tracerId)
	if err != nil {
		return err
	}

	// write to file - thread safety enforced at OS-level (?)
	_, err = io.Copy(fp, data)
	return err

}
