// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import (
	"context"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/process"
	"github.com/DataDog/datadog-agent/pkg/util/backoff"
	"github.com/DataDog/datadog-agent/pkg/util/clusteragent"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
	"github.com/benbjohnson/clock"
)

const (
	subscriber          = "language_detection_client"
	runningMetricPeriod = 1 * time.Minute

	minBackoffFactor = 2.0
	baseBackoffTime  = 1.0 * time.Second
	recoveryInterval = 2 * time.Second
	maxError         = 10
	maxBackoffTime   = 30 * time.Second
)

type containerDetails struct {
	containersLanguages map[string]*languagesSet
}

func (c *containerDetails) toProto() []*pbgo.ContainerLanguageDetails {
	res := make([]*pbgo.ContainerLanguageDetails, 0, len(c.containersLanguages))
	for containerName, languageSet := range c.containersLanguages {
		res = append(res, &pbgo.ContainerLanguageDetails{
			ContainerName: containerName,
			Languages:     languageSet.toProto(),
		})
	}
	return res
}

type languagesSet struct {
	languages map[string]struct{}
}

func (c *languagesSet) add(language string) {
	c.languages[language] = struct{}{}
}

func (c *languagesSet) toProto() []*pbgo.Language {
	res := make([]*pbgo.Language, 0, len(c.languages))
	for lang := range c.languages {
		res = append(res, &pbgo.Language{
			Name: lang,
		})
	}
	return res
}

type podDetails struct {
	namespace           string
	containersLanguages *containerDetails
	ownerRef            *workloadmeta.KubernetesPodOwner
}

func (p *podDetails) toProto(podName string) *pbgo.PodLanguageDetails {
	return &pbgo.PodLanguageDetails{
		Name:      podName,
		Namespace: p.namespace,
		Ownerref: &pbgo.KubeOwnerInfo{
			Id:   p.ownerRef.ID,
			Name: p.ownerRef.Name,
			Kind: p.ownerRef.Kind,
		},
		ContainerDetails: p.containersLanguages.toProto(),
	}
}

func (p *podDetails) getOrAddContainerDetails(containerName string) *languagesSet {
	if languagesSet, ok := p.containersLanguages.containersLanguages[containerName]; ok {
		return languagesSet
	}
	p.containersLanguages.containersLanguages[containerName] = &languagesSet{
		languages: make(map[string]struct{}),
	}
	return p.containersLanguages.containersLanguages[containerName]
}

func (b *batch) getOrAddPodDetails(podName, podNamespace string, ownerRef *workloadmeta.KubernetesPodOwner) *podDetails {
	if podDetails, ok := b.podDetails[podName]; ok {
		return podDetails
	}
	b.podDetails[podName] = &podDetails{
		namespace: podNamespace,
		containersLanguages: &containerDetails{
			containersLanguages: make(map[string]*languagesSet),
		},
		ownerRef: ownerRef,
	}
	return b.podDetails[podName]
}

type batch struct {
	podDetails map[string]*podDetails
}

func newBatch() *batch { return &batch{make(map[string]*podDetails, 0)} }

func (b *batch) toProto() *pbgo.ParentLanguageAnnotationRequest {
	res := &pbgo.ParentLanguageAnnotationRequest{}
	for podName, language := range b.podDetails {
		res.PodDetails = append(res.PodDetails, language.toProto(podName))
	}
	return res
}

func (b *batch) sendMetrics() {
	for podName, podDetails := range b.podDetails {
		for containerName, languages := range podDetails.containersLanguages.containersLanguages {
			for language := range languages.languages {
				ProcessedEvents.Inc(podName, containerName, language)
			}
		}
	}
}

// Client sends language information to the Cluster-Agent
type Client struct {
	ctx                        context.Context
	cfg                        config.Config
	store                      workloadmeta.Store
	dcaLanguageDetectionClient clusteragent.LanguageDetectionClient
	currentBatch               *batch
}

// NewClient creates a new Client
func NewClient(
	ctx context.Context,
	cfg config.Config,
	store workloadmeta.Store,
	dcaLanguageDetectionClient clusteragent.LanguageDetectionClient,
) *Client {
	return &Client{
		ctx:                        ctx,
		cfg:                        cfg,
		store:                      store,
		dcaLanguageDetectionClient: dcaLanguageDetectionClient,
		currentBatch:               newBatch(),
	}
}

func getContainerNameFromPod(cid string, pod *workloadmeta.KubernetesPod) (string, bool) {
	for _, container := range pod.Containers {
		if container.ID == cid {
			return container.Name, true
		}
	}
	return "", false
}

func podHasOwner(pod *workloadmeta.KubernetesPod) bool {
	return pod.Owners != nil && len(pod.Owners) > 0
}

func (c *Client) flush() {
	if len(c.currentBatch.podDetails) == 0 {
		return
	}
	ch := make(chan *batch)
	go func() {
		data := <-ch
		clock := clock.New()
		errorCount := 0
		backoffPolicy := backoff.NewExpBackoffPolicy(minBackoffFactor, baseBackoffTime.Seconds(), maxBackoffTime.Seconds(), 0, false)
		data.sendMetrics()
		for {
			if errorCount > maxError {
				return
			}
			var err error
			refreshInterval := backoffPolicy.GetBackoffDuration(errorCount)
			select {
			case <-clock.After(refreshInterval):
				protoMessage := data.toProto()
				t := time.Now()
				err = c.dcaLanguageDetectionClient.PostLanguageMetadata(c.ctx, protoMessage)
				if err == nil {
					Latency.Observe(time.Since(t).Seconds())
					Requests.Inc(StatusSuccess)
					return
				}
				Requests.Add(1, StatusError)
				errorCount = backoffPolicy.IncError(1)
			case <-c.ctx.Done():
				return
			}
		}
	}()
	ch <- c.currentBatch
	c.currentBatch = newBatch()
}

func (c *Client) processEvent(evBundle workloadmeta.EventBundle) {
	close(evBundle.Ch)
	log.Tracef("Processing %d events", len(evBundle.Events))
	for _, event := range evBundle.Events {
		if event.Entity.GetID().Kind == workloadmeta.KindProcess && event.Type == workloadmeta.EventTypeSet {
			process := event.Entity.(*workloadmeta.Process)
			if process.Language == nil {
				continue
			}
			pod, err := c.store.GetKubernetesPodForContainer(process.ContainerId)
			if err != nil {
				log.Debug("skipping language detection for process %s", process.ID)
				continue
			}
			if !podHasOwner(pod) {
				continue
			}
			containerName, ok := getContainerNameFromPod(process.ContainerId, pod)
			if !ok {
				log.Debug("container name not found for %s", process.ContainerId)
				continue
			}
			podDetails := c.currentBatch.getOrAddPodDetails(pod.Name, pod.Namespace, &pod.Owners[0])
			containerDetails := podDetails.getOrAddContainerDetails(containerName)
			containerDetails.add(string(process.Language.Name))
		}
	}
}

// StreamLanguages starts streaming languages to the Cluster-Agent
func (c *Client) StreamLanguages() {
	log.Infof("Starting language detection client")
	defer log.Infof("Shutting down language detection client")

	processEventCh := c.store.Subscribe(
		subscriber,
		workloadmeta.NormalPriority,
		workloadmeta.NewFilter(
			[]workloadmeta.Kind{
				workloadmeta.KindProcess,
			},
			workloadmeta.SourceAll,
			workloadmeta.EventTypeAll,
		),
	)

	periodicFlushTimer := time.NewTicker(time.Duration(c.cfg.GetDuration("language_detection.client_period")))
	defer periodicFlushTimer.Stop()

	metricTicker := time.NewTicker(runningMetricPeriod)
	defer metricTicker.Stop()

	for {
		select {
		case eventBundle := <-processEventCh:
			c.processEvent(eventBundle)
		case <-periodicFlushTimer.C:
			if c.dcaLanguageDetectionClient == nil {
				dcaClient, err := clusteragent.GetClusterAgentClient()
				if err != nil {
					log.Errorf("failed to get dca client %s", err)
					continue
				}
				c.dcaLanguageDetectionClient = dcaClient
			}
			c.flush()
		case <-metricTicker.C:
			Running.Set(1.0)
		case <-c.ctx.Done():
			return
		}
	}
}
