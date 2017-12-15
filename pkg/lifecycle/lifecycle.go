// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package lifecycle

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	log "github.com/cihub/seelog"
	"github.com/gorilla/mux"
)

var globalLifecycle = &Lifecycle{}

const expectedRefreshSeconds = 60

// Lifecycle items to handle the readiness and liveness statuses
type Lifecycle struct {
	readiness         sync.Once
	lastHealthRefresh int64
	m                 sync.RWMutex
}

type lifecycleResponse struct {
	Health bool `json:"health"`
}

// GetLifecycle return the package globalLifecycle struct pointer
func GetLifecycle() *Lifecycle {
	return globalLifecycle
}

// RecordHealthPath add a /health route to the http server
func RecordHealthPath() {
	l := GetLifecycle()
	r := mux.NewRouter()
	r.HandleFunc("/health", l.healthHandler).Methods("GET")
	http.Handle("/", r)
}

// readinessOnce is executed only Once
// Contain all associated calls to notify the readiness state of the whole application
func (l *Lifecycle) readinessOnce() {
	l.readiness.Do(func() {
		log.Infof("Triggering the readiness stage")
		notifySystemd()
	})
}

// RefreshHealthStatus call readinessOnce and update the timestamp of lastHealthRefresh
func (l *Lifecycle) RefreshHealthStatus() {
	l.readinessOnce()
	l.m.Lock()
	l.lastHealthRefresh = time.Now().Unix()
	l.m.Unlock()
	log.Debugf("Refreshed the healthCheck value")
}

func (l *Lifecycle) healthHandler(w http.ResponseWriter, r *http.Request) {
	resp := &lifecycleResponse{}
	l.m.RLock()
	resp.Health = l.lastHealthRefresh > time.Now().Unix()-expectedRefreshSeconds
	l.m.RUnlock()
	w.Header().Set("Content-Type", "application/json")
	byteResp, err := json.Marshal(resp)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		log.Warnf("GET /health %d: fail to marshal response: %s", http.StatusInternalServerError, err)
		return
	}
	if resp.Health {
		w.WriteHeader(http.StatusOK)
		log.Debugf("GET /health %d: application is healthy", http.StatusOK)
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
		log.Debugf("GET /health %d: application is unhealthy", http.StatusServiceUnavailable)
	}
	w.Write(byteResp)
}
