// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build kubeapiserver

package languagedetection

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/DataDog/datadog-agent/comp/core/config"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	langUtil "github.com/DataDog/datadog-agent/pkg/languagedetection/util"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/process"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/leaderelection"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	// statusSuccess is the value for the "status" tag that represents a successful operation
	statusSuccess = "success"
	// statusError is the value for the "status" tag that represents an error
	statusError = "error"
)

type handlerConfig struct {
	enabled       bool
	languageTTL   time.Duration
	cleanupPeriod time.Duration
}

type languageDetectionHandler struct {
	ownersLanguages       *OwnersLanguages
	cfg                   handlerConfig
	wlm                   workloadmeta.Component
	leaderElectionEnabled bool

	// Protected by stateMutex
	wasLeader          bool
	followerSyncCancel context.CancelFunc
	initialized        bool // tracks if we've done initial setup
	stateMutex         sync.Mutex
}

func newLanguageDetectionHandler(wlm workloadmeta.Component, cfg config.Component) *languageDetectionHandler {
	return &languageDetectionHandler{
		cfg: handlerConfig{
			enabled:       cfg.GetBool("language_detection.enabled"),
			languageTTL:   cfg.GetDuration("cluster_agent.language_detection.cleanup.language_ttl"),
			cleanupPeriod: cfg.GetDuration("cluster_agent.language_detection.cleanup.period"),
		},
		wlm:                   wlm,
		ownersLanguages:       newOwnersLanguages(),
		leaderElectionEnabled: cfg.GetBool("leader_election"),
		wasLeader:             false,
	}
}

func (handler *languageDetectionHandler) startCleanupInBackground(ctx context.Context) {
	// Launch periodic cleanup mechanism
	go func() {
		cleanupTicker := time.NewTicker(handler.cfg.cleanupPeriod)
		defer cleanupTicker.Stop()
		for {
			select {
			case <-cleanupTicker.C:
				// Only clean expired languages if we're the leader
				if handler.isLeader() {
					handler.ownersLanguages.cleanExpiredLanguages(handler.wlm)
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// Periodic flush for followers to push synced languages to workloadmeta
	// without blocking the subscriber handler on eventCh
	go func() {
		flushTicker := time.NewTicker(1 * time.Minute)
		defer flushTicker.Stop()
		for {
			select {
			case <-flushTicker.C:
				if !handler.isLeader() {
					_ = handler.ownersLanguages.flush(handler.wlm)
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// Remove any owner when its corresponding resource is deleted
	go handler.ownersLanguages.cleanRemovedOwners(handler.wlm)

	// Monitor leadership changes and sync state for followers
	if handler.leaderElectionEnabled {
		go handler.monitorLeadershipChanges(ctx)
	}
}

// preHandler is called by both leader and followers and returns true if the request should be forwarded or handled by the leader
func (handler *languageDetectionHandler) preHandler(w http.ResponseWriter, r *http.Request) bool {
	if !handler.cfg.enabled {
		ProcessedRequests.Inc(statusError)
		http.Error(w, "Language detection feature is disabled on the cluster agent", http.StatusServiceUnavailable)
		return false
	}

	// Reject if no body
	if r.Body == nil {
		ProcessedRequests.Inc(statusError)
		http.Error(w, "Request body is empty", http.StatusBadRequest)
		return false
	}

	return true
}

// leaderHandler is called only by the leader and used to patch the annotations
func (handler *languageDetectionHandler) leaderHandler(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		ProcessedRequests.Inc(statusError)
		return
	}

	// Create a new instance of the protobuf message type
	requestData := &pbgo.ParentLanguageAnnotationRequest{}

	// Unmarshal the request body into the protobuf message
	err = proto.Unmarshal(body, requestData)
	if err != nil {
		http.Error(w, "Failed to unmarshal request body", http.StatusBadRequest)
		ProcessedRequests.Inc(statusError)
		return
	}

	ownersLanguagesFromRequest := getOwnersLanguages(requestData, time.Now().Add(handler.cfg.languageTTL))

	if log.ShouldLog(log.TraceLvl) { // Avoid call to String() if not needed
		log.Tracef("Owner Languages state pre merge-and-flush: %s", handler.ownersLanguages.String())
		log.Tracef("Owner languages received from pld client: %s", ownersLanguagesFromRequest.String())
	}

	err = handler.ownersLanguages.mergeAndFlush(ownersLanguagesFromRequest, handler.wlm)
	if log.ShouldLog(log.TraceLvl) { // Avoid call to String() if not needed
		log.Tracef("Owner Languages state post merge-and-flush: %s", handler.ownersLanguages.String())
	}
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to store some (or all) languages in workloadmeta store: %s", err), http.StatusInternalServerError)
		ProcessedRequests.Inc(statusError)
		return
	}

	ProcessedRequests.Inc(statusSuccess)
	w.WriteHeader(http.StatusOK)
}

// isLeader checks if the current instance is the leader
func (handler *languageDetectionHandler) isLeader() bool {
	if !handler.leaderElectionEnabled {
		return true
	}

	leaderEngine, err := leaderelection.GetLeaderEngine()
	if err != nil {
		log.Errorf("Failed to get leader engine: %v", err)
		return false
	}

	return leaderEngine.IsLeader()
}

// monitorLeadershipChanges monitors leader election changes and manages state accordingly
// Uses a hybrid approach: subscribes to events but also polls periodically as a fallback
// This is necessary because the event-based mechanism can miss notifications (see PR #37122)
func (handler *languageDetectionHandler) monitorLeadershipChanges(ctx context.Context) {
	// Try to get the leader election engine and subscribe
	var leadershipChangeChan <-chan struct{}
	if handler.leaderElectionEnabled {
		leaderEngine, err := leaderelection.GetLeaderEngine()
		if err != nil {
			log.Warnf("Failed to get leader engine for event subscription, will use polling only: %v", err)
		} else {
			// Subscribe to events, but don't rely on them exclusively
			leadershipChangeChan, _ = leaderEngine.Subscribe()
		}
	}

	// Use a ticker as a fallback mechanism (or primary mechanism if leader election is disabled)
	// This ensures we don't miss leadership changes even if events are dropped
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	// Cleanup on exit
	defer func() {
		handler.stateMutex.Lock()
		if handler.followerSyncCancel != nil {
			handler.followerSyncCancel()
		}
		handler.stateMutex.Unlock()
	}()

	// Process initial state
	handler.handleLeadershipState(ctx)

	// Monitor for leadership changes using both events and polling
	for {
		select {
		case <-leadershipChangeChan:
			// Event received, check leadership immediately
			handler.handleLeadershipState(ctx)

		case <-ticker.C:
			// Periodic check as fallback
			handler.handleLeadershipState(ctx)

		case <-ctx.Done():
			return
		}
	}
}

// handleLeadershipState checks the current leadership state and handles any necessary transitions
// It manages starting/stopping follower sync when leadership changes
func (handler *languageDetectionHandler) handleLeadershipState(ctx context.Context) {
	isLeader := handler.isLeader()

	handler.stateMutex.Lock()
	defer handler.stateMutex.Unlock()

	// Handle initial state
	if !handler.initialized {
		handler.initialized = true
		handler.wasLeader = isLeader

		if isLeader {
			log.Info("Starting as leader")
			// Leader starts with empty state and will populate it from incoming requests
		} else {
			log.Info("Starting as follower, will sync DetectedLangs with InjectableLangs")
			var syncCtx context.Context
			syncCtx, handler.followerSyncCancel = context.WithCancel(ctx)
			go handler.syncFollowerWithInjectableLanguages(syncCtx)
		}
		return
	}

	// No change in leadership state
	if handler.wasLeader == isLeader {
		return
	}

	// Update state
	handler.wasLeader = isLeader

	if isLeader {
		// Became leader
		log.Info("Gained leadership")
		// Since we were a follower, our DetectedLangs are already in sync with InjectableLangs
		// No need to initialize - we already have the correct state

		// Stop follower sync if running
		if handler.followerSyncCancel != nil {
			handler.followerSyncCancel()
			handler.followerSyncCancel = nil
		}
	} else {
		// Lost leadership
		log.Info("Lost leadership, starting to sync DetectedLangs with InjectableLangs")
		// As a follower, we need to keep DetectedLangs in sync with InjectableLangs

		// Start follower sync
		if handler.followerSyncCancel != nil {
			handler.followerSyncCancel() // Cancel any existing sync
		}
		var syncCtx context.Context
		syncCtx, handler.followerSyncCancel = context.WithCancel(ctx)
		go handler.syncFollowerWithInjectableLanguages(syncCtx)
	}
}

// syncFollowerWithInjectableLanguages keeps follower's DetectedLangs in sync with InjectableLangs
// This runs continuously until the context is cancelled or leadership changes
func (handler *languageDetectionHandler) syncFollowerWithInjectableLanguages(ctx context.Context) {
	// Subscribe to deployment changes from kubeapiserver
	// We're interested in InjectableLanguages changes (which come from annotations)
	filter := workloadmeta.NewFilterBuilder().
		SetSource(workloadmeta.SourceKubeAPIServer).
		AddKind(workloadmeta.KindKubernetesDeployment).
		Build()

	evChan := handler.wlm.Subscribe(
		"language-detection-follower",
		workloadmeta.NormalPriority,
		filter,
	)
	defer handler.wlm.Unsubscribe(evChan)

	for {
		select {
		case evBundle := <-evChan:
			evBundle.Acknowledge()

			// Process deployment events to sync DetectedLangs with InjectableLangs
			for _, event := range evBundle.Events {
				if event.Type != workloadmeta.EventTypeSet {
					continue
				}
				deployment := event.Entity.(*workloadmeta.KubernetesDeployment)

				// Extract deployment name and namespace from entity id
				deploymentIDs := strings.Split(deployment.ID, "/")
				if len(deploymentIDs) != 2 {
					log.Errorf("Invalid deployment ID format: %s, expected 'namespace/name'", deployment.ID)
					continue
				}
				namespace := deploymentIDs[0]
				deploymentName := deploymentIDs[1]

				owner := langUtil.NewNamespacedOwnerReference(
					"apps/v1",
					langUtil.KindDeployment,
					deploymentName,
					namespace,
				)

				// Always sync DetectedLangs to match InjectableLangs (even if empty)
				handler.ownersLanguages.syncFromInjectableLanguages(owner, deployment.InjectableLanguages, handler.cfg.languageTTL)

				if len(deployment.InjectableLanguages) > 0 {
					log.Debugf("Follower synced languages for deployment %s/%s from InjectableLanguages",
						namespace, deploymentName)
				}
			}

		case <-ctx.Done():
			return
		}
	}
}
