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

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/flare"
	// "github.com/DataDog/datadog-agent/pkg/util"
	"github.com/mholt/archiver"
	"github.com/rs/xid"
)

type RemoteFlare struct {
	Id      string
	Ts      int64 // collect until this timestamp
	tempDir string
	zipPath string
	files   map[string]*os.File
	sources map[string]*RegisteredSource
}

var (
	currentFlare *RemoteFlare = nil
	mutex        sync.RWMutex
)

func NewRemoteFlare(path, zip string, d time.Duration) *RemoteFlare {
	return &RemoteFlare{
		Id:      xid.New().String(),
		tempDir: path,
		zipPath: zip,
		Ts:      time.Now().Add(d).Unix(),
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

func (f RemoteFlare) hasSource(id string) (*RegisteredSource, bool) {
	src, ok := f.sources[id]
	return src, ok
}

func (f RemoteFlare) wrapUp() error {
	defer os.RemoveAll(f.tempDir)

	grace := config.Datadog.GetInt("flare_grace_period")
	time.Sleep(time.Until(time.Unix(f.Ts, 0).Add(time.Second * time.Duration(grace))))

	mutex.Lock()
	currentFlare = nil
	mutex.Unlock()

	for _, fp := range f.files {
		fp.Close()
	}

	err := archiver.Zip.Make(f.zipPath, []string{f.tempDir})
	if err != nil {
		return err
	}

	return nil
}

func GetCurrentFlareId() (string, error) {
	mutex.RLock()
	defer mutex.RUnlock()
	if currentFlare == nil {
		return "", errors.New("no ongoing flare")
	}

	return currentFlare.Id, nil
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

	currentFlare = NewRemoteFlare(tempDir, flare.GetArchivePath(), d)
	if tracerId != "" {
		src, ok := GetSourceById(tracerId)
		if ok {
			currentFlare.sources[src.Id] = src
		}
	} else {
		currentFlare.sources = GetSourcesByServiceAndEnv(svc, env)
	}

	go currentFlare.wrapUp()

	return currentFlare, nil

}

func GetFlareForId(id string) (*RemoteFlare, error) {
	mutex.RLock()
	f := currentFlare
	mutex.RUnlock()

	if _, ok := f.hasSource(id); !ok {
		return nil, nil
	}
	return f, nil

}

func LogEntry(flareId, tracerId string, data io.ReadCloser) error {
	mutex.RLock()
	if currentFlare == nil {
		mutex.RUnlock()
		return InvalidFlareId{}
	}

	if flareId != currentFlare.Id {
		return errors.New("")
	}
	mutex.RUnlock()

	fp, err := currentFlare.GetFile(tracerId)
	if err != nil {
		return err
	}

	// write to file - thread safety enforced at OS-level (?)
	// + writes from tracers should be serialized by nature.
	_, err = io.Copy(fp, data)
	return err

}
