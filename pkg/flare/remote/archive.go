// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package remote

import (
	"bufio"
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

type RemoteFlareStatus struct {
	Id     string
	Status string
	File   string
	Ttl    int64
	Err    string
}

var (
	currentFlare    *RemoteFlare = nil
	mutex           sync.RWMutex
	completedFlares map[string]*RemoteFlare = make(map[string]*RemoteFlare) // id to zipPath map
	listMutex       sync.RWMutex
)

const (
	sourcesLogfile = "sources.log"
	StatusUnknown  = "unknown"
	StatusOngoing  = "ongoing"
	StatusReady    = "ready"
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

func (f *RemoteFlare) GetId() string {
	mutex.RLock()
	defer mutex.RUnlock()

	return f.Id
}

func (f *RemoteFlare) GetFile(id string) (*os.File, error) {
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

func (f *RemoteFlare) hasSource(id string) (*RegisteredSource, bool) {
	src, ok := f.sources[id]
	return src, ok
}

func (f *RemoteFlare) wrapUp() error {
	defer os.RemoveAll(f.tempDir)

	grace := config.Datadog.GetInt("flare_grace_period")
	time.Sleep(time.Until(time.Unix(f.Ts, 0).Add(time.Second * time.Duration(grace))))

	mutex.Lock()
	currentFlare = nil
	mutex.Unlock()

	for _, fp := range f.files {
		fp.Close()
	}

	logPath := path.Join(f.tempDir, sourcesLogfile)
	fp, err := os.Create(logPath)
	if err != nil {
		return err
	}

	w := bufio.NewWriter(fp)
	for _, source := range f.sources {
		w.WriteString(source.String())
	}
	w.Flush()
	fp.Close()

	err = archiver.Zip.Make(f.zipPath, []string{f.tempDir})
	if err != nil {
		return err
	}

	listMutex.RLock()
	completedFlares[f.Id] = f
	listMutex.RUnlock()

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

func GetStatus(id string) *RemoteFlareStatus {
	var flareId string
	var err error

	status := &RemoteFlareStatus{
		Status: StatusUnknown,
	}

	// has the flare been completed
	listMutex.RLock()
	flare, ok := completedFlares[id]
	listMutex.RUnlock()

	// or it might be ongoing
	status.Id = id
	if !ok {
		// we can hold the RLock and still call `GetCurrentFlareId` safely
		mutex.RLock()
		flareId, err = GetCurrentFlareId()
		if err != nil || flareId != id {
			status.Err = errors.New("invalid flare id").Error()
			return status
		}

		status.Status = StatusOngoing
		status.File = currentFlare.zipPath

		status.Ttl = currentFlare.Ts - time.Now().Unix()
		if status.Ttl < 0 {
			status.Ttl = 0
		}
		mutex.RUnlock()
	} else {
		status.Status = StatusReady
		status.File = flare.zipPath
		status.Ttl = 0
	}

	return status
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

	if f == nil {
		return nil, nil
	}

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
