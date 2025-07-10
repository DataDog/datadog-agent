// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

/*
Package api implements the agent IPC api. Using HTTP
calls, it's possible to communicate with the agent,
sending commands and receiving infos.
*/
package api

import (
	"context"
	"crypto/subtle"
	"crypto/tls"
	"errors"
	"fmt"
	stdLog "log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	grpc_auth "github.com/grpc-ecosystem/go-grpc-middleware/auth"
	"google.golang.org/grpc"

	"github.com/DataDog/datadog-agent/cmd/cluster-agent/api/agent"
	v1 "github.com/DataDog/datadog-agent/cmd/cluster-agent/api/v1"
	"github.com/DataDog/datadog-agent/cmd/cluster-agent/api/v1/languagedetection"
	"github.com/DataDog/datadog-agent/cmd/cluster-agent/api/v2/series"
	"github.com/DataDog/datadog-agent/comp/api/grpcserver/helpers"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	"github.com/DataDog/datadog-agent/comp/core/config"
	diagnose "github.com/DataDog/datadog-agent/comp/core/diagnose/def"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	"github.com/DataDog/datadog-agent/comp/core/settings"
	"github.com/DataDog/datadog-agent/comp/core/status"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	taggerserver "github.com/DataDog/datadog-agent/comp/core/tagger/server"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	dcametadata "github.com/DataDog/datadog-agent/comp/metadata/clusteragent/def"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	grpcutil "github.com/DataDog/datadog-agent/pkg/util/grpc"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	pkglogsetup "github.com/DataDog/datadog-agent/pkg/util/log/setup"
)

var (
	listener  net.Listener
	router    *mux.Router
	apiRouter *mux.Router
)

// StartServer creates the router and starts the HTTP server
func StartServer(ctx context.Context, w workloadmeta.Component, taggerComp tagger.Component, ac autodiscovery.Component, statusComponent status.Component, settings settings.Component, cfg config.Component, ipc ipc.Component, diagnoseComponent diagnose.Component, dcametadataComp dcametadata.Component, telemetry telemetry.Component) error {
	// create the root HTTP router
	router = mux.NewRouter()
	apiRouter = router.PathPrefix("/api/v1").Subrouter()

	// IPC REST API server
	agent.SetupHandlers(router, w, ac, statusComponent, settings, taggerComp, diagnoseComponent, dcametadataComp, ipc)

	// API V1 Metadata APIs
	v1.InstallMetadataEndpoints(apiRouter, w)

	// API V1 Language Detection APIs
	languagedetection.InstallLanguageDetectionEndpoints(ctx, apiRouter, w, cfg)

	// API V2 Series APIs
	v2ApiRouter := router.PathPrefix("/api/v2").Subrouter()
	series.InstallNodeMetricsEndpoints(ctx, v2ApiRouter, cfg)

	// Validate token for every request
	router.Use(validateToken(ipc))

	// get the transport we're going to use under HTTP
	var err error
	listener, err = getListener()
	if err != nil {
		// we use the listener to handle commands for the agent, there's
		// no way we can recover from this error
		return fmt.Errorf("unable to create the api server: %v", err)
	}

	// DCA client token
	util.InitDCAAuthToken(pkgconfigsetup.Datadog()) //nolint:errcheck

	tlsConfig := ipc.GetTLSServerConfig()

	tlsConfig.MinVersion = tls.VersionTLS13

	if pkgconfigsetup.Datadog().GetBool("cluster_agent.allow_legacy_tls") {
		tlsConfig.MinVersion = tls.VersionTLS10
	}

	// Use a stack depth of 4 on top of the default one to get a relevant filename in the stdlib
	logWriter, _ := pkglogsetup.NewTLSHandshakeErrorWriter(4, log.WarnLvl)

	authInterceptor := grpcutil.AuthInterceptor(func(token string) (interface{}, error) {
		if subtle.ConstantTimeCompare([]byte(token), []byte(util.GetDCAAuthToken())) == 0 {
			return struct{}{}, errors.New("Invalid session token")
		}

		return struct{}{}, nil
	})

	maxMessageSize := cfg.GetInt("cluster_agent.cluster_tagger.grpc_max_message_size")
	opts := []grpc.ServerOption{
		grpc.StreamInterceptor(grpc_auth.StreamServerInterceptor(authInterceptor)),
		grpc.UnaryInterceptor(grpc_auth.UnaryServerInterceptor(authInterceptor)),
		grpc.MaxSendMsgSize(maxMessageSize),
		grpc.MaxRecvMsgSize(maxMessageSize),
	}

	grpcSrv := grpc.NewServer(opts...)
	// event size should be small enough to fit within the grpc max message size
	maxEventSize := maxMessageSize / 2
	pb.RegisterAgentSecureServer(grpcSrv, &serverSecure{
		taggerServer: taggerserver.NewServer(taggerComp, telemetry, maxEventSize, cfg.GetInt("remote_tagger.max_concurrent_sync")),
	})

	timeout := pkgconfigsetup.Datadog().GetDuration("cluster_agent.server.idle_timeout_seconds") * time.Second
	errorLog := stdLog.New(logWriter, "Error from the agent http API server: ", 0) // log errors to seelog
	srv := helpers.NewMuxedGRPCServer(
		listener.Addr().String(),
		tlsConfig,
		grpcSrv,
		// Use a recovery handler to log panics if they happen.
		// The client will receive a 500 error.
		handlers.RecoveryHandler(
			handlers.PrintRecoveryStack(true),
			handlers.RecoveryLogger(errorLog),
		)(router),
		timeout,
	)
	srv.ErrorLog = errorLog

	tlsListener := tls.NewListener(listener, srv.TLSConfig)

	go srv.Serve(tlsListener) //nolint:errcheck
	return nil
}

// ModifyAPIRouter allows to pass in a function to modify router used in server
func ModifyAPIRouter(f func(*mux.Router)) {
	f(apiRouter)
}

// StopServer closes the connection and the server
// stops listening to new commands.
func StopServer() {
	if listener != nil {
		listener.Close()
	}
}

// We only want to maintain 1 API and expose an external route to serve the cluster level metadata.
// As we have 2 different tokens for the validation, we need to validate accordingly.
func validateToken(ipc ipc.Component) mux.MiddlewareFunc {
	dcaTokenValidator := util.TokenValidator(util.GetDCAAuthToken)
	localTokenGetter := util.TokenValidator(ipc.GetAuthToken)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path := r.URL.String()
			var isValid bool
			// If communication is intra-pod
			if !isExternalPath(path) {
				if err := localTokenGetter(w, r); err == nil {
					isValid = true
				}
			}
			if !isValid {
				if err := dcaTokenValidator(w, r); err != nil {
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// isExternal returns whether the path is an endpoint used by Node Agents.
func isExternalPath(path string) bool {
	return strings.HasPrefix(path, "/api/v1/metadata/") && len(strings.Split(path, "/")) == 7 || // support for agents < 6.5.0
		path == "/version" ||
		path == "/api/v1/languagedetection" ||
		path == "/api/v2/series" ||
		strings.HasPrefix(path, "/api/v1/annotations/node/") && len(strings.Split(path, "/")) == 6 ||
		strings.HasPrefix(path, "/api/v1/cf/apps") && len(strings.Split(path, "/")) == 5 ||
		strings.HasPrefix(path, "/api/v1/cf/apps/") && len(strings.Split(path, "/")) == 6 ||
		strings.HasPrefix(path, "/api/v1/cf/org_quotas") && len(strings.Split(path, "/")) == 5 ||
		strings.HasPrefix(path, "/api/v1/cf/orgs") && len(strings.Split(path, "/")) == 5 ||
		strings.HasPrefix(path, "/api/v1/cluster/id") && len(strings.Split(path, "/")) == 5 ||
		strings.HasPrefix(path, "/api/v1/clusterchecks/") && len(strings.Split(path, "/")) == 6 ||
		strings.HasPrefix(path, "/api/v1/endpointschecks/") && len(strings.Split(path, "/")) == 6 ||
		strings.HasPrefix(path, "/api/v1/metadata/namespace/") && len(strings.Split(path, "/")) == 6 ||
		strings.HasPrefix(path, "/api/v1/tags/cf/apps/") && len(strings.Split(path, "/")) == 7 ||
		strings.HasPrefix(path, "/api/v1/tags/namespace/") && len(strings.Split(path, "/")) == 6 ||
		strings.HasPrefix(path, "/api/v1/tags/node/") && len(strings.Split(path, "/")) == 6 ||
		strings.HasPrefix(path, "/api/v1/tags/pod/") && (len(strings.Split(path, "/")) == 6 || len(strings.Split(path, "/")) == 8)
}
