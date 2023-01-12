// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver
// +build kubeapiserver

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
	pollInterval          time.Duration
	isLeader              func() bool
	subscribers           map[TargetObjKind]chan PatchRequest
	lastSuccessfulRefresh time.Time
	clusterName           string
}

var _ patchProvider = &filePatchProvider{}

func newfileProvider(isLeaderFunc func() bool, clusterName string) *filePatchProvider {
	return &filePatchProvider{
		file:         "/etc/datadog-agent/auto-instru.json",
		pollInterval: 15 * time.Second,
		isLeader:     isLeaderFunc,
		subscribers:  make(map[TargetObjKind]chan PatchRequest),
		clusterName:  clusterName,
	}
}

func (fpp *filePatchProvider) subscribe(kind TargetObjKind) chan PatchRequest {
	ch := make(chan PatchRequest, 10)
	fpp.subscribers[kind] = ch
	return ch
}

func (fpp *filePatchProvider) start(stopCh <-chan struct{}) {
	ticker := time.NewTicker(fpp.pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if err := fpp.refresh(); err != nil {
				log.Errorf(err.Error())
			}
		case <-stopCh:
			log.Info("Shutting down patch provider")
			return
		}
	}
}

func (fpp *filePatchProvider) refresh() error {
	if !fpp.isLeader() {
		log.Debug("Not leader, skipping")
		return nil
	}
	requests, err := fpp.poll()
	if err != nil {
		return err
	}
	log.Debugf("Got %d new patch requests", len(requests))
	for _, req := range requests {
		if err := req.Validate(fpp.clusterName); err != nil {
			log.Errorf("Skipping invalid patch request: %s", err)
			continue
		}
		if ch, found := fpp.subscribers[req.K8sTarget.Kind]; found {
			log.Infof("Publishing patch requests for target %s", req.K8sTarget)
			ch <- req
		}
	}
	fpp.lastSuccessfulRefresh = time.Now()
	return nil
}

func (fpp *filePatchProvider) poll() ([]PatchRequest, error) {
	info, err := os.Stat(fpp.file)
	if err != nil {
		return nil, err
	}
	modTime := info.ModTime()
	if fpp.lastSuccessfulRefresh.After(modTime) {
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
