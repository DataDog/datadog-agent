// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config/remote"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo"
	"github.com/DataDog/datadog-agent/pkg/trace/api"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/metrics"
	"github.com/DataDog/datadog-agent/pkg/trace/metrics/timing"
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var bufferPool = sync.Pool{
	New: func() interface{} {
		return new(bytes.Buffer)
	},
}

func getBuffer() *bytes.Buffer {
	buffer := bufferPool.Get().(*bytes.Buffer)
	buffer.Reset()
	return buffer
}

func putBuffer(buffer *bytes.Buffer) {
	bufferPool.Put(buffer)
}

func remoteConfigHandler(r *api.HTTPReceiver, client remote.ConfigUpdater, cfg *config.AgentConfig) http.Handler {
	cidProvider := api.NewIDProvider(cfg.ContainerProcRoot)
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		defer timing.Since("datadog.trace_agent.receiver.config_process_ms", time.Now())
		tags := r.TagStats(api.V07, req.Header).AsTags()
		statusCode := http.StatusOK
		defer func() {
			tags = append(tags, fmt.Sprintf("status_code:%d", statusCode))
			metrics.Count("datadog.trace_agent.receiver.config_request", 1, tags, 1)
		}()

		buf := getBuffer()
		defer putBuffer(buf)
		_, err := io.Copy(buf, req.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)

		}
		var configsRequest pbgo.ClientGetConfigsRequest
		err = json.Unmarshal(buf.Bytes(), &configsRequest)
		if err != nil {
			statusCode = http.StatusBadRequest
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if configsRequest.GetClient().GetClientTracer() != nil {
			normalize(&configsRequest)
			if configsRequest.Client.ClientTracer.Tags == nil {
				configsRequest.Client.ClientTracer.Tags = make([]string, 0)
			}
			for _, tag := range getContainerTags(req, cfg, cidProvider) {
				configsRequest.Client.ClientTracer.Tags = append(configsRequest.Client.ClientTracer.Tags, tag)
			}
		}
		cfg, err := client.ClientGetConfigs(req.Context(), &configsRequest)
		if err != nil {
			statusCode = http.StatusInternalServerError
			http.Error(w, err.Error(), statusCode)
			return
		}
		if cfg == nil {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		content, err := json.Marshal(cfg)
		if err != nil {
			statusCode = http.StatusInternalServerError
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Write(content)

	})
}

func getContainerTags(req *http.Request, cfg *config.AgentConfig, provider api.IDProvider) []string {
	if cfg == nil || cfg.ContainerTags == nil {
		return nil
	}
	if cid := provider.GetContainerID(req.Context(), req.Header); cid != "" {
		containerTags, err := cfg.ContainerTags(cid)
		if err != nil {
			_ = log.Error("Failed getting container tags", err)
		}
		return containerTags
	}
	return nil
}

func normalize(configsRequest *pbgo.ClientGetConfigsRequest) {
	// err is explicitly ignored as it is not an actual error and the expected normalized service
	// is returned regardless.
	configsRequest.Client.ClientTracer.Service, _ = traceutil.NormalizeService(configsRequest.Client.ClientTracer.Service, configsRequest.Client.ClientTracer.Language)
	configsRequest.Client.ClientTracer.Env = traceutil.NormalizeTag(configsRequest.Client.ClientTracer.Env)
}
