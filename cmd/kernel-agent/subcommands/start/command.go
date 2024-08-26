// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO Fix revive linter
package start

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	grpc_auth "github.com/grpc-ecosystem/go-grpc-middleware/auth"
	"github.com/spf13/cobra"
	"go.uber.org/fx"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	grpc_runtime "github.com/grpc-ecosystem/grpc-gateway/runtime"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer"
	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer/demultiplexerimpl"
	authtokenimpl "github.com/DataDog/datadog-agent/comp/api/authtoken/fetchonlyimpl"
	"github.com/DataDog/datadog-agent/comp/collector/collector"
	"github.com/DataDog/datadog-agent/comp/collector/collector/collectorimpl"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/autodiscoveryimpl"
	acServer "github.com/DataDog/datadog-agent/comp/core/autodiscovery/server"
	"github.com/DataDog/datadog-agent/comp/core/config"
	healthprobe "github.com/DataDog/datadog-agent/comp/core/healthprobe/def"
	healthprobefx "github.com/DataDog/datadog-agent/comp/core/healthprobe/fx"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logfx "github.com/DataDog/datadog-agent/comp/core/log/fx"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/comp/core/secrets/secretsimpl"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	noopTelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/noopsimpl"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafx "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/eventplatformimpl"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatformreceiver"
	"github.com/DataDog/datadog-agent/comp/forwarder/orchestrator/orchestratorimpl"
	integrations "github.com/DataDog/datadog-agent/comp/logs/integrations/def"
	"github.com/DataDog/datadog-agent/comp/metadata/host"
	"github.com/DataDog/datadog-agent/comp/metadata/host/hostimpl"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryagent"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryagent/inventoryagentimpl"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryhost"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryhost/inventoryhostimpl"
	"github.com/DataDog/datadog-agent/comp/metadata/resources/resourcesimpl"
	"github.com/DataDog/datadog-agent/comp/metadata/runner"
	metadatarunnerimpl "github.com/DataDog/datadog-agent/comp/metadata/runner/runnerimpl"
	"github.com/DataDog/datadog-agent/comp/serializer/compression/compressionimpl"
	"github.com/DataDog/datadog-agent/pkg/api/security"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	pkgcollector "github.com/DataDog/datadog-agent/pkg/collector"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/generic"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/net/network"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/net/ntp"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/networkpath"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/system/cpu/cpu"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/system/cpu/load"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/system/disk/disk"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/system/disk/io"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/system/filehandles"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/system/memory"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/system/uptime"
	telemetryCheck "github.com/DataDog/datadog-agent/pkg/collector/corechecks/telemetry"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	grpcutil "github.com/DataDog/datadog-agent/pkg/util/grpc"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

type CLIParams struct {
	confPath string
}

// MakeCommand returns the start subcommand for the 'dogstatsd' command.
func MakeCommand() *cobra.Command {
	cliParams := &CLIParams{}
	startCmd := &cobra.Command{
		Use:   "start",
		Short: "Start Kernel Agent",
		Long:  `Runs Kernel Agent in the foreground`,
		RunE: func(*cobra.Command, []string) error {
			return RunKernelAgent(cliParams, "", start)
		},
	}

	// local flags
	startCmd.PersistentFlags().StringVarP(&cliParams.confPath, "cfgpath", "c", "", "path to directory containing datadog.yaml")

	return startCmd
}

func RunKernelAgent(cliParams *CLIParams, defaultConfPath string, fct interface{}) error {
	return fxutil.OneShot(fct,
		fx.Supply(cliParams),

		// Configuration
		fx.Supply(config.NewParams(
			defaultConfPath,
			config.WithConfFilePath(cliParams.confPath),
			config.WithConfigMissingOK(true),
			config.WithConfigName("datadog")),
		),
		config.Module(),

		// Logging
		logfx.Module(),
		fx.Supply(log.ForDaemon("KA", "log_file", "/var/log/datadog/kernel-agent.log")),

		// Secrets management
		fx.Provide(func(comp secrets.Component) optional.Option[secrets.Component] {
			return optional.NewOption[secrets.Component](comp)
		}),
		fx.Supply(secrets.NewEnabledParams()),
		secretsimpl.Module(),
		noopTelemetry.Module(),

		// Metadata submission
		hostnameimpl.Module(),
		hostimpl.Module(),
		resourcesimpl.Module(),
		fx.Supply(resourcesimpl.Disabled()),
		inventoryagentimpl.Module(),
		inventoryhostimpl.Module(),
		// sysprobeconfig is optionally required by inventoryagent
		sysprobeconfig.NoneModule(),
		metadatarunnerimpl.Module(),
		authtokenimpl.Module(), // We need to think about this one

		// Sending metrics to the backend
		fx.Provide(defaultforwarder.NewParams),
		defaultforwarder.Module(),
		compressionimpl.Module(),
		// Since we do not use the build tag orchestrator, we use the comp/forwarder/orchestrator/orchestratorimpl/forwarder_no_orchestrator.go
		orchestratorimpl.Module(),
		fx.Supply(orchestratorimpl.NewDisabledParams()),
		eventplatformimpl.Module(),
		fx.Supply(eventplatformimpl.NewDisabledParams()),
		eventplatformreceiver.NoneModule(),
		// injecting the shared Serializer to FX until we migrate it to a proper component. This allows other
		// already migrated components to request it.
		fx.Provide(func(demuxInstance demultiplexer.Component) serializer.MetricSerializer {
			return demuxInstance.Serializer()
		}),
		demultiplexerimpl.Module(),
		fx.Provide(func(config config.Component) demultiplexerimpl.Params {
			params := demultiplexerimpl.NewDefaultParams()
			params.ContinueOnMissingHostname = true
			return params
		}),

		// Autodiscovery
		// Do we really need autodiscovery for the Logs Agent?
		autodiscoveryimpl.Module(),
		// TODO: (components) - some parts of the agent (such as the logs agent) implicitly depend on the global state
		// set up by LoadComponents. In order for components to use lifecycle hooks that also depend on this global state, we
		// have to ensure this code gets run first. Once the common package is made into a component, this can be removed.
		//
		// Workloadmeta component needs to be initialized before this hook is executed, and thus is included
		// in the function args to order the execution. This pattern might be worth revising because it is
		// error prone.
		fx.Invoke(func(lc fx.Lifecycle, wmeta workloadmeta.Component, ac autodiscovery.Component, config config.Component) {
			lc.Append(fx.Hook{
				OnStart: func(_ context.Context) error {
					//  setup the AutoConfig instance
					common.LoadComponents(nil, wmeta, ac, config.GetString("confd_path"))
					return nil
				},
			})
		}),
		fx.Provide(tagger.NewTaggerParams),
		// Can the tagger works without the workloadmeta?
		taggerimpl.Module(),
		// workloadmeta setup
		collectors.GetCatalog(),
		fx.Provide(workloadmeta.NewParams),
		workloadmetafx.Module(),

		// Core checks
		fx.Provide(func(ms serializer.MetricSerializer) optional.Option[serializer.MetricSerializer] {
			return optional.NewOption[serializer.MetricSerializer](ms)
		}),
		collectorimpl.Module(),
		fx.Supply(optional.NewNoneOption[integrations.Component]()),

		// Healthprobe
		fx.Provide(func(config config.Component) healthprobe.Options {
			return healthprobe.Options{
				Port:           config.GetInt("health_port"),
				LogsGoroutines: config.GetBool("log_all_goroutines_when_unhealthy"),
			}
		}),
		healthprobefx.Module(),
	)
}

func start(
	cliParams *CLIParams,
	config config.Component,
	log log.Component,
	_ host.Component,
	_ inventoryagent.Component,
	_ inventoryhost.Component,
	_ runner.Component,
	_ healthprobe.Component,
	_ tagger.Component,
	workloadmeta workloadmeta.Component,
	telemetry telemetry.Component,
	collector collector.Component,
	demultiplexer demultiplexer.Component,
	logReceiver optional.Option[integrations.Component],
	ac autodiscovery.Component,
) error {
	// Main context passed to components
	ctx, cancel := context.WithCancel(context.Background())

	defer StopAgent(cancel, log)

	stopCh := make(chan struct{})
	go handleSignals(stopCh, log)

	registerCoreChecks(workloadmeta, telemetry)
	ac.AddScheduler("check", pkgcollector.InitCheckScheduler(optional.NewOption(collector), demultiplexer, logReceiver), true)

	ac.LoadAndRun(context.Background())

	err := Run(ctx, cliParams, config, log, ac)
	if err != nil {
		return err
	}

	// Block here until we receive a stop signal
	<-stopCh

	return nil
}

// Run starts the kernel agent server
func Run(ctx context.Context, cliParams *CLIParams, config config.Component, log log.Component, ac autodiscovery.Component) (err error) {
	if len(cliParams.confPath) == 0 {
		log.Infof("Config will be read from env variables")
	}

	if !config.IsSet("api_key") {
		err = log.Critical("no API key configured, exiting")
		return
	}

	err = util.CreateAndSetAuthToken(config)
	if err != nil {
		return err
	}

	apiAddr, err := getIPCAddressPort(config)
	if err != nil {
		return fmt.Errorf("unable to get IPC address and port: %v", err)
	}

	tlsKeyPair, tlsCertPool, err := initializeTLS(log, []string{apiAddr}...)
	if err != nil {
		return fmt.Errorf("unable to initialize TLS: %v", err)
	}

	// tls.Config is written to when serving, so it has to be cloned for each server
	tlsConfig := func() *tls.Config {
		return &tls.Config{
			Certificates: []tls.Certificate{*tlsKeyPair},
			NextProtos:   []string{"h2"},
			MinVersion:   tls.VersionTLS12,
		}
	}

	// start the server
	if err := startServer(
		apiAddr,
		tlsConfig(),
		tlsCertPool,
		log,
		config,
		ac,
	); err != nil {
		return fmt.Errorf("unable to start API server: %v", err)
	}

	return nil
}

func startServer(
	cmdAddr string,
	tlsConfig *tls.Config,
	tlsCertPool *x509.CertPool,
	log log.Component,
	config config.Component,
	ac autodiscovery.Component,
) (err error) {
	// get the transport we're going to use under HTTP
	cmdListener, err := net.Listen("tcp", cmdAddr)
	if err != nil {
		return fmt.Errorf("unable to listen to the given address: %v", err)
	}

	// gRPC server
	authInterceptor := grpcutil.AuthInterceptor(parseToken)
	opts := []grpc.ServerOption{
		grpc.Creds(credentials.NewClientTLSFromCert(tlsCertPool, cmdAddr)),
		grpc.StreamInterceptor(grpc_auth.StreamServerInterceptor(authInterceptor)),
		grpc.UnaryInterceptor(grpc_auth.UnaryServerInterceptor(authInterceptor)),
	}

	s := grpc.NewServer(opts...)
	pb.RegisterAgentSecureServer(s, &serverSecure{
		autoDiscoveryServer: acServer.NewServer(ac),
	})

	dcreds := credentials.NewTLS(&tls.Config{
		ServerName: cmdAddr,
		RootCAs:    tlsCertPool,
	})
	dopts := []grpc.DialOption{grpc.WithTransportCredentials(dcreds)}

	// starting grpc gateway
	ctx := context.Background()
	gwmux := grpc_runtime.NewServeMux()

	err = pb.RegisterAgentSecureHandlerFromEndpoint(
		ctx, gwmux, cmdAddr, dopts)
	if err != nil {
		return fmt.Errorf("error registering agent secure handler from endpoint %s: %v", cmdAddr, err)
	}

	cmdMux := http.NewServeMux()
	cmdMux.Handle("/", gwmux)

	srv := grpcutil.NewMuxedGRPCServer(
		cmdAddr,
		tlsConfig,
		s,
		grpcutil.TimeoutHandlerFunc(cmdMux, time.Duration(config.GetInt64("server_timeout"))*time.Second),
	)

	tlsListener := tls.NewListener(cmdListener, srv.TLSConfig)

	go srv.Serve(tlsListener) //nolint:errcheck

	log.Infof("Started HTTP server on %s", cmdListener.Addr().String())

	return nil
}

func parseToken(token string) (interface{}, error) {
	if token != util.GetAuthToken() {
		return struct{}{}, errors.New("Invalid session token")
	}

	// Currently this empty struct doesn't add any information
	// to the context, but we could potentially add some custom
	// type.
	return struct{}{}, nil
}

// getIPCAddressPort returns a listening connection
func getIPCAddressPort(config config.Component) (string, error) {
	return fmt.Sprintf("%v:%v", config.GetString("cmd_host"), config.GetInt("cmd_port")), nil
}

func buildSelfSignedKeyPair(additionalHostIdentities ...string) ([]byte, []byte) {
	hosts := []string{"127.0.0.1", "localhost", "::1"}
	hosts = append(hosts, additionalHostIdentities...)
	_, rootCertPEM, rootKey, err := security.GenerateRootCert(hosts, 2048)
	if err != nil {
		return nil, nil
	}

	// PEM encode the private key
	rootKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(rootKey),
	})

	// Create and return TLS private cert and key
	return rootCertPEM, rootKeyPEM
}

func initializeTLS(log log.Component, additionalHostIdentities ...string) (*tls.Certificate, *x509.CertPool, error) {
	// print the caller to identify what is calling this function
	if _, file, line, ok := runtime.Caller(1); ok {
		log.Infof("[%s:%d] Initializing TLS certificates for hosts %v", file, line, strings.Join(additionalHostIdentities, ", "))
	}

	cert, key := buildSelfSignedKeyPair(additionalHostIdentities...)
	if cert == nil {
		return nil, nil, errors.New("unable to generate certificate")
	}
	pair, err := tls.X509KeyPair(cert, key)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to generate TLS key pair: %v", err)
	}

	tlsCertPool := x509.NewCertPool()
	ok := tlsCertPool.AppendCertsFromPEM(cert)
	if !ok {
		return nil, nil, fmt.Errorf("unable to add new certificate to pool")
	}

	return &pair, tlsCertPool, nil
}

// handleSignals handles OS signals, and sends a message on stopCh when an interrupt
// signal is received.
func handleSignals(stopCh chan struct{}, log log.Component) {
	// Setup a channel to catch OS signals
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM, syscall.SIGPIPE)

	// Block here until we receive the interrupt signal
	for signo := range signalCh {
		switch signo {
		case syscall.SIGPIPE:
			// By default systemd redirects the stdout to journald. When journald is stopped or crashes we receive a SIGPIPE signal.
			// Go ignores SIGPIPE signals unless it is when stdout or stdout is closed, in this case the agent is stopped.
			// We never want dogstatsd to stop upon receiving SIGPIPE, so we intercept the SIGPIPE signals and just discard them.
		default:
			log.Infof("Received signal '%s', shutting down...", signo)
			stopCh <- struct{}{}
			return
		}
	}
}

func StopAgent(cancel context.CancelFunc, log log.Component) {
	// retrieve the agent health before stopping the components
	// GetReadyNonBlocking has a 100ms timeout to avoid blocking
	health, err := health.GetReadyNonBlocking()
	if err != nil {
		log.Warnf("Kernel Agent health unknown: %s", err)
	} else if len(health.Unhealthy) > 0 {
		log.Warnf("Some components were unhealthy: %v", health.Unhealthy)
	}

	// gracefully shut down any component
	cancel()

	log.Info("See ya!")
	log.Flush()
}

// registerCoreChecks registers all core checks
func registerCoreChecks(workloadmeta workloadmeta.Component, telemetry telemetry.Component) {
	// Required checks
	corechecks.RegisterCheck(cpu.CheckName, cpu.Factory())
	corechecks.RegisterCheck(load.CheckName, load.Factory())
	corechecks.RegisterCheck(memory.CheckName, memory.Factory())
	corechecks.RegisterCheck(uptime.CheckName, uptime.Factory())
	corechecks.RegisterCheck(ntp.CheckName, ntp.Factory())
	corechecks.RegisterCheck(network.CheckName, network.Factory())
	corechecks.RegisterCheck(snmp.CheckName, snmp.Factory())
	corechecks.RegisterCheck(io.CheckName, io.Factory())
	corechecks.RegisterCheck(filehandles.CheckName, filehandles.Factory())
	corechecks.RegisterCheck(telemetryCheck.CheckName, telemetryCheck.Factory())
	corechecks.RegisterCheck(networkpath.CheckName, networkpath.Factory(telemetry))
	corechecks.RegisterCheck(disk.CheckName, io.Factory())
	corechecks.RegisterCheck(generic.CheckName, generic.Factory(workloadmeta))
}
