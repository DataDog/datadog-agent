// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package remoteagentimpl implements the remoteagent component interface
package remoteagentimpl

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"net"
	"reflect"
	"runtime"
	"sync"
	"time"

	grpc_auth "github.com/grpc-ecosystem/go-grpc-middleware/auth"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"google.golang.org/protobuf/types/known/structpb"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/flare/types"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	remoteagent "github.com/DataDog/datadog-agent/comp/core/remoteagent/def"
	"github.com/DataDog/datadog-agent/comp/core/status"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	pbcore "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/common"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	grpcutil "github.com/DataDog/datadog-agent/pkg/util/grpc"
)

// DefaultRegistrationInterval is the default interval for refreshing the remote agent registration with the Core Agent.
const DefaultRegistrationInterval = 10 * time.Second

// Requires defines the dependencies for the remoteagent component
type Requires struct {
	// Remove this field if the component has no lifecycle hooks
	Lifecycle compdef.Lifecycle
	IPC       ipc.Component
	Log       log.Component
	Config    config.Component

	FlareProviders        []*types.FlareFiller    `group:"flare"`
	StatusHeaderProviders []status.HeaderProvider `group:"header_status"`
	StatusProviders       []status.Provider       `group:"status"`
}

// Provides defines the output of the remoteagent component
type Provides struct {
	Comp remoteagent.Component
}

type remoteAgentComponent struct {
	// dependencies
	log                    log.Component
	flareGenerationTimeout time.Duration
	ipc                    ipc.Component
	config                 config.Component

	// internal state
	pbcore.UnimplementedRemoteAgentServer
	listener net.Listener
	wg       sync.WaitGroup
	ctx      context.Context
	cancel   context.CancelFunc

	// providers
	flareProviders        []*types.FlareFiller
	statusProviders       []status.Provider
	statusHeaderProviders []status.HeaderProvider
}

// NewComponent creates a new remoteagent component
func NewComponent(reqs Requires) (Provides, error) {

	// Binding to a random port on localhost
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return Provides{}, err
	}

	server := &remoteAgentComponent{
		log:                    reqs.Log,
		flareGenerationTimeout: 30 * time.Second, // Default timeout for flare generation
		ipc:                    reqs.IPC,
		config:                 reqs.Config,
		listener:               listener,
		flareProviders:         fxutil.GetAndFilterGroup(reqs.FlareProviders),
		statusProviders:        fxutil.GetAndFilterGroup(reqs.StatusProviders),
		statusHeaderProviders:  fxutil.GetAndFilterGroup(reqs.StatusHeaderProviders),
	}

	serverOpts := []grpc.ServerOption{
		grpc.Creds(credentials.NewTLS(reqs.IPC.GetTLSServerConfig())),
		grpc.UnaryInterceptor(grpc_auth.UnaryServerInterceptor(grpcutil.StaticAuthInterceptor(reqs.IPC.GetAuthToken()))),
	}

	grpcServer := grpc.NewServer(serverOpts...)
	pbcore.RegisterRemoteAgentServer(grpcServer, server)

	reqs.Lifecycle.Append(compdef.Hook{
		OnStart: func(context.Context) error {
			return server.start(grpcServer)
		},
		OnStop: func(context.Context) error {
			server.cancel() // Cancel the context to stop the server
			// Wait for the server to finish serving before closing the listener
			grpcServer.GracefulStop()
			server.wg.Wait()
			return listener.Close()
		},
	})

	provides := Provides{
		Comp: server,
	}
	return provides, nil
}

func (r *remoteAgentComponent) start(grpcServer *grpc.Server) error {
	// Main context passed to components, consistent with the one used in the WorkloadMeta component.
	mainCtx, _ := common.GetMainCtxCancel()
	r.ctx, r.cancel = context.WithCancel(mainCtx)

	// Starting remoteAgent server
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		if err := grpcServer.Serve(r.listener); err != nil {
			r.log.Errorf("failed to serve remote agent: %v", err)
		}
	}()

	r.log.Infof("Remote Agent server started on %s", r.listener.Addr().String())

	registerReq := &pbcore.RegisterRemoteAgentRequest{
		Id:          flavor.GetFlavor(),
		DisplayName: flavor.GetHumanReadableFlavor(),
		ApiEndpoint: r.listener.Addr().String(),
		AuthToken:   r.ipc.GetAuthToken(), // This should be dropped in favor of mTLS in the future
	}

	ipcAddress, err := pkgconfigsetup.GetIPCAddress(r.config)
	if err != nil {
		return err
	}

	ipcPort := pkgconfigsetup.GetIPCPort()

	ipcAddress = net.JoinHostPort(ipcAddress, ipcPort)

	r.log.Infof("Registering with Core Agent at %s...", ipcAddress)

	conn, err := grpc.NewClient(ipcAddress,
		grpc.WithTransportCredentials(credentials.NewTLS(r.ipc.GetTLSClientConfig())),
		grpc.WithPerRPCCredentials(grpcutil.NewBearerTokenAuth(r.ipc.GetAuthToken())),
	)
	if err != nil {
		return err
	}
	coreAgentClient := pbcore.NewAgentSecureClient(conn)

	// Register remote agent with the Core Agent

	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		// This goroutine will periodically refresh the registration with the Core Agent.
		for {
			registrationInterval := DefaultRegistrationInterval
			resp, err := coreAgentClient.RegisterRemoteAgent(r.ctx, registerReq)
			if err != nil {
				r.log.Errorf("failed to refresh remote agent registration: %v", err)
			} else {
				r.log.Info("Registered with Core Agent.")
				registrationInterval = time.Duration(resp.RecommendedRefreshIntervalSecs) * time.Second
			}

			select {
			case <-r.ctx.Done():
				return // Exit the goroutine if the context is cancelled
			case <-time.After(registrationInterval):
				// Wait for the recommended refresh interval before retrying
			}
		}
	}()

	return nil
}

func (r *remoteAgentComponent) GetFlareFiles(context.Context, *pbcore.GetFlareFilesRequest) (*pbcore.GetFlareFilesResponse, error) {
	r.log.Infof("Received request to generate flare files")
	fb := newRemoteFlareFiller()

	timer := time.NewTimer(r.flareGenerationTimeout)
	defer timer.Stop()

	for _, p := range r.flareProviders {
		timeout := max(r.flareGenerationTimeout, p.Timeout(fb))
		timer.Reset(timeout)
		providerName := runtime.FuncForPC(reflect.ValueOf(p.Callback).Pointer()).Name()
		r.log.Infof("Running flare provider %s with timeout %s", providerName, timeout)
		_ = fb.Logf("Running flare provider %s with timeout %s", providerName, timeout)

		done := make(chan struct{})
		go func() {
			startTime := time.Now()
			err := p.Callback(fb)
			duration := time.Since(startTime)

			if err == nil {
				r.log.Debugf("flare provider '%s' completed in %s", providerName, duration)
			} else {
				errMsg := r.log.Errorf("flare provider '%s' failed after %s: %s", providerName, duration, err)
				_ = fb.Logf("%s", errMsg.Error())
			}

			done <- struct{}{}
		}()

		select {
		case <-done:
			if !timer.Stop() {
				<-timer.C
			}
		case <-timer.C:
			err := r.log.Warnf("flare provider '%s' skipped after %s", providerName, timeout)
			_ = fb.Logf("%s", err.Error())
		}
	}

	r.log.Info("All flare providers have been run, creating archive...")

	return fb.GetFlareResponse(), nil
}

func (r *remoteAgentComponent) GetJsonStatusDetails(_ context.Context, _ *pbcore.GetStatusDetailsRequest) (*pbcore.GetJsonStatusDetailsResponse, error) {
	r.log.Infof("Received request for status details")

	response := &pbcore.GetJsonStatusDetailsResponse{
		NamedSections: make(map[string]*structpb.Struct),
	}

	headerStatus := make(map[string]interface{})

	// Process HeaderProviders for the MainSection (high-level details)
	for _, provider := range r.statusHeaderProviders {
		// Create a temporary map to collect fields from the header provider
		stats := make(map[string]interface{})

		// JSON is used to collect the status data
		err := provider.JSON(true, stats)
		if err != nil {
			r.log.Warnf("Error collecting status header from %s: %v", provider.Name(), err)
			continue
		}
		headerStatus[provider.Name()] = stats
	}
	// JSON is used to collect the status data
	m, err := structpb.NewStruct(headerStatus)
	if err != nil {
		return nil, fmt.Errorf("error converting header to JSON: %v", err)
	}
	response.MainSection = m

	// Process StatusProviders for NamedSections (component-specific details)
	for _, provider := range r.statusProviders {
		// Get the section name for this provider
		sectionName := provider.Section()

		if _, exist := response.NamedSections[sectionName]; exist {
			// If the section already exists, add suffix to the name
			// to avoid overwriting it. This can happen if multiple providers
			// return the same section name.
			suffix := 1
			for {
				if _, exist := response.NamedSections[fmt.Sprintf("%s_%d", sectionName, suffix)]; exist {
					suffix++
					continue
				}
				break
			}

			sectionName = fmt.Sprintf("%s_%d", sectionName, suffix)
			// Log that we are skipping the provider for this section
			r.log.Warnf("Status section '%s' already exists, adding suffix to the name: %s", sectionName, sectionName)
		}

		// JSON is used to collect the status data
		stats := make(map[string]interface{})
		err := provider.JSON(true, stats)
		if err != nil {
			r.log.Warnf("Error collecting status from %s (%s): %v", provider.Name(), sectionName, err)
			continue
		}
		// JSON is used to collect the status data
		statusValue, err := structpb.NewStruct(stats)
		if err != nil {
			return nil, fmt.Errorf("error converting header to JSON: %v", err)
		}
		response.NamedSections[sectionName] = statusValue
	}

	// Log the collected status details
	r.log.Infof("Collected status details: %s", response)

	return response, nil
}

func (r *remoteAgentComponent) GetTextStatusDetails(_ context.Context, _ *pbcore.GetStatusDetailsRequest) (*pbcore.GetTextStatusDetailsResponse, error) {
	r.log.Infof("Received request for status details")

	response := &pbcore.GetTextStatusDetailsResponse{
		NamedSections: make(map[string]string),
	}

	var b bytes.Buffer
	writer := bufio.NewWriter(&b)

	// Process HeaderProviders for the MainSection (high-level details)
	for _, provider := range r.statusHeaderProviders {
		// Text is used to collect the status data
		err := provider.Text(true, writer)
		if err != nil {
			r.log.Warnf("Error collecting status header from %s: %v", provider.Name(), err)
			continue
		}
	}

	writer.Flush() // Ensure all data is written to the buffer
	response.MainSection = b.String()

	// Process StatusProviders for NamedSections (component-specific details)
	for _, provider := range r.statusProviders {
		// Get the section name for this provider
		sectionName := provider.Section()

		if _, exist := response.NamedSections[sectionName]; exist {
			// If the section already exists, add suffix to the name
			// to avoid overwriting it. This can happen if multiple providers
			// return the same section name.
			suffix := 1
			for {
				if _, exist := response.NamedSections[fmt.Sprintf("%s_%d", sectionName, suffix)]; exist {
					suffix++
					continue
				}
				break
			}

			sectionName = fmt.Sprintf("%s_%d", sectionName, suffix)
			// Log that we are skipping the provider for this section
			r.log.Warnf("Status section '%s' already exists, adding suffix to the name: %s", sectionName, sectionName)
		}

		// Create a io Writer to collect fields from the provider
		var b bytes.Buffer
		writer := bufio.NewWriter(&b)

		// Text is used to collect the status data
		err := provider.Text(true, writer)
		if err != nil {
			r.log.Warnf("Error collecting status from %s (%s): %v", provider.Name(), sectionName, err)
			continue
		}

		writer.Flush() // Ensure all data is written to the buffer
		response.NamedSections[sectionName] = b.String()
	}

	// Log the collected status details
	r.log.Infof("Collected status details: %s", response)

	return response, nil
}

func (r *remoteAgentComponent) GetHtmlStatusDetails(_ context.Context, _ *pbcore.GetStatusDetailsRequest) (*pbcore.GetHtmlStatusDetailsResponse, error) {
	r.log.Infof("Received request for status details")

	response := &pbcore.GetHtmlStatusDetailsResponse{
		NamedSections: make(map[string]string),
	}

	var b bytes.Buffer
	writer := bufio.NewWriter(&b)

	// Process HeaderProviders for the MainSection (high-level details)
	for _, provider := range r.statusHeaderProviders {
		// Text is used to collect the status data
		err := provider.HTML(true, writer)
		if err != nil {
			r.log.Warnf("Error collecting status header from %s: %v", provider.Name(), err)
			continue
		}
	}

	writer.Flush() // Ensure all data is written to the buffer
	response.MainSection = b.String()

	// Process StatusProviders for NamedSections (component-specific details)
	for _, provider := range r.statusProviders {
		// Get the section name for this provider
		sectionName := provider.Section()

		if _, exist := response.NamedSections[sectionName]; exist {
			// If the section already exists, add suffix to the name
			// to avoid overwriting it. This can happen if multiple providers
			// return the same section name.
			suffix := 1
			for {
				if _, exist := response.NamedSections[fmt.Sprintf("%s_%d", sectionName, suffix)]; exist {
					suffix++
					continue
				}
				break
			}

			sectionName = fmt.Sprintf("%s_%d", sectionName, suffix)
			// Log that we are skipping the provider for this section
			r.log.Warnf("Status section '%s' already exists, adding suffix to the name: %s", sectionName, sectionName)
		}
		// Create a io Writer to collect fields from the provider
		var b bytes.Buffer
		writer := bufio.NewWriter(&b)

		// Text is used to collect the status data
		err := provider.HTML(true, writer)
		if err != nil {
			r.log.Warnf("Error collecting status from %s (%s): %v", provider.Name(), sectionName, err)
			continue
		}

		writer.Flush() // Ensure all data is written to the buffer
		response.NamedSections[sectionName] = b.String()
	}

	// Log the collected status details
	r.log.Infof("Collected status details: %s", response)

	return response, nil
}

func (r *remoteAgentComponent) GetTelemetry(context.Context, *pbcore.GetTelemetryRequest) (*pbcore.GetTelemetryResponse, error) {
	// Not implemented yet
	return &pbcore.GetTelemetryResponse{}, nil
}
