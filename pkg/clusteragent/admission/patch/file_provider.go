// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package patch

import (
	"encoding/json"
	"os"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// filePatchProvider this is a stub and will be used for e2e testing only
type filePatchProvider struct {
	file                  string
	isLeaderNotif         <-chan struct{}
	pollInterval          time.Duration
	subscribers           map[TargetObjKind]chan PatchRequest
	lastSuccessfulRefresh time.Time
	clusterName           string
}

var _ patchProvider = &filePatchProvider{}

func newfileProvider(file string, isLeaderNotif <-chan struct{}, clusterName string) *filePatchProvider {
	return &filePatchProvider{
		file:          file,
		isLeaderNotif: isLeaderNotif,
		pollInterval:  15 * time.Second,
		subscribers:   make(map[TargetObjKind]chan PatchRequest),
		clusterName:   clusterName,
	}
}

func (fpp *filePatchProvider) subscribe(kind TargetObjKind) chan PatchRequest {
	ch := make(chan PatchRequest, 10)
	fpp.subscribers[kind] = ch
	return ch
}

func (fpp *filePatchProvider) start(stopCh <-chan struct{}) {
	log.Infof("Starting file patch provider: watching %s", fpp.file)
	ticker := time.NewTicker(fpp.pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-fpp.isLeaderNotif:
			log.Info("Got a leader notification, polling from file")
			fpp.process(true)
		case <-ticker.C:
			fpp.process(false)
		case <-stopCh:
			log.Info("Shutting down file patch provider")
			return
		}
	}
}

func (fpp *filePatchProvider) process(forcePoll bool) {
	requests, err := fpp.poll(forcePoll)
	if err != nil {
		log.Errorf("Error refreshing patch requests: %v", err)
		return
	}
	if len(requests) == 0 {
		return
	}
	log.Infof("Got %d updates from local file", len(requests))
	for _, req := range requests {
		if err := req.Validate(fpp.clusterName); err != nil {
			log.Errorf("Skipping invalid patch request: %s", err)
			continue
		}
		if ch, found := fpp.subscribers[req.K8sTarget.Kind]; found {
			log.Infof("Publishing patch request for target %s", req.K8sTarget)
			ch <- req
		}
	}
	fpp.lastSuccessfulRefresh = time.Now()
}

func (fpp *filePatchProvider) poll(forcePoll bool) ([]PatchRequest, error) {
	info, err := os.Stat(fpp.file)
	if err != nil {
		return nil, err
	}
	if !forcePoll && fpp.lastSuccessfulRefresh.After(info.ModTime()) {
		log.Debugf("File %q hasn't changed since the last Successful refresh at %v", fpp.file, fpp.lastSuccessfulRefresh)
		return []PatchRequest{}, nil
	}
	content, err := os.ReadFile(fpp.file)
	if err != nil {
		return nil, err
	}
	var requests []PatchRequest
	err = json.Unmarshal(content, &requests)
	return requests, err
}
