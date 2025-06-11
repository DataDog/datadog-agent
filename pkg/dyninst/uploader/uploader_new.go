// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package uploader

import (
	"bytes"
	"fmt"
	"net/http"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type BatchUploader struct {
	client         http.Client
	batchSize      int
	uploadInterval time.Duration
	buffer         chan []byte
}

func NewBatchUploader() *BatchUploader {
	u := &BatchUploader{
		client:         http.Client{},
		batchSize:      100,
		uploadInterval: 1 * time.Second, // should be configured by new env variable
		buffer:         make(chan []byte, 100),
	}

	go u.processBuffer()
	return u
}

func (u *BatchUploader) processBuffer() {
	flushTimer := time.NewTicker(u.uploadInterval)
	defer flushTimer.Stop()

	batch := make([][]byte, 0, u.batchSize)

	for {
		select {
		case item := <-u.buffer:
			batch = append(batch, item)
			if len(batch) >= u.batchSize {
				u.uploadBatch(batch)
				flushTimer.Reset(u.uploadInterval)
			}
		case <-flushTimer.C:
			if len(batch) > 0 {
				u.uploadBatch(batch)
			}
		}
	}
}

func (u *BatchUploader) uploadBatch(batch [][]byte) {
	url := fmt.Sprintf("http://%s:8126/debugger/v1/input", getAgentHost())
	body := bytes.Join(batch, []byte("\n"))
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		log.Info("Failed to build request", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := u.client.Do(req)
	if err != nil {
		log.Info("Error uploading log batch", err)
		return
	}
	defer resp.Body.Close()
	log.Info("HTTP", resp.StatusCode, url)
}
