// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package apiimpl

import "net/http"

// cliMetricsMiddleware returns a gorilla/mux middleware that reads CLI invocation
// headers injected by the agent CLI and increments the agent_cli_invocations counter.
func (server *apiServer) cliMetricsMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cmd := r.Header.Get("X-DD-CLI-Command")
			label := r.Header.Get("X-DD-CLI-Heuristic-Label")
			if cmd != "" && label != "" {
				server.cliInvocations.Inc(cmd, label)
			}
			next.ServeHTTP(w, r)
		})
	}
}
