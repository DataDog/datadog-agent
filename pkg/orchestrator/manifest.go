// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2021 Datadog, Inc.

package orchestrator

import (
	"math/rand"
	"sync"
	"time"

	model "github.com/DataDog/agent-payload/process"
	"github.com/DataDog/datadog-agent/pkg/forwarder"
	"github.com/DataDog/datadog-agent/pkg/orchestrator/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"k8s.io/apimachinery/pkg/util/wait"
)

// Collector is the public manifest collector instance
var Collector *ManifestCollector

// ManifestCollector buffers and sends manifest
type ManifestCollector struct {
	buffer     []*model.Manifest
	bufferLock sync.Mutex
	sender     *Sender
	groupID    int32
	cfg        *config.OrchestratorConfig
}

// InitManifestCollector inits the public collector
func InitManifestCollector(cfg *config.OrchestratorConfig, hostName string) {
	Collector = &ManifestCollector{
		sender:  NewSender(cfg, hostName),
		groupID: rand.Int31(),
		cfg:     cfg,
	}
}

// BufferManifest buffers manifest before sending them
func (m *ManifestCollector) BufferManifest(manifests []*model.Manifest) {
	m.bufferLock.Lock()
	m.buffer = append(m.buffer, manifests...)
	m.bufferLock.Unlock()
}

// Start starts the manifest collector
func (m *ManifestCollector) Start(stopCh <-chan struct{}) {
	if err := m.sender.Start(); err != nil {
		log.Errorf("Error starting manifest forwarder: %s", err)
		return
	}
	defer m.sender.Stop()
	go wait.Until(m.sendManifests, 10*time.Second, stopCh)
	<-stopCh
}

func (m *ManifestCollector) sendManifests() {
	m.bufferLock.Lock()
	defer m.bufferLock.Unlock()

	if len(m.buffer) == 0 {
		return
	}

	clusterID, err := clustername.GetClusterID()
	if err != nil {
		log.Errorf("Error sending manifests, could not get cluster ID: %s", err)
		return
	}

	groupSize := len(m.buffer) / m.cfg.MaxPerMessage
	if len(m.buffer)%m.cfg.MaxPerMessage != 0 {
		groupSize++
	}
	chunkedManifests := chunkManifests(m.buffer, groupSize, m.cfg.MaxPerMessage)
	manifestMessages := make([]model.MessageBody, 0, groupSize)
	for i := 0; i < groupSize; i++ {
		manifestMessages = append(manifestMessages, &model.CollectorManifest{
			ClusterName: m.cfg.KubeClusterName,
			Manifests:   chunkedManifests[i],
			GroupId:     m.groupID,
			GroupSize:   int32(groupSize),
			ClusterId:   clusterID,
		})
	}
	m.sender.SendMessages(manifestMessages, forwarder.PayloadTypeManifest)
	// clear the buffer
	m.buffer = nil
}

// chunkManifests formats and chunks the pods into a slice of chunks using a specific number of chunks.
func chunkManifests(manifests []*model.Manifest, chunkCount, chunkSize int) [][]*model.Manifest {
	chunks := make([][]*model.Manifest, 0, chunkCount)

	for c := 1; c <= chunkCount; c++ {
		var (
			chunkStart = chunkSize * (c - 1)
			chunkEnd   = chunkSize * (c)
		)
		// last chunk may be smaller than the chunk size
		if c == chunkCount {
			chunkEnd = len(manifests)
		}
		chunks = append(chunks, manifests[chunkStart:chunkEnd])
	}

	return chunks
}
