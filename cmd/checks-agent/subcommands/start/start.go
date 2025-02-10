// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO Fix revive linter
package start

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime"
	"runtime/debug"
	"syscall"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/spf13/cobra"
	"go.uber.org/fx"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"

	"github.com/DataDog/datadog-agent/comp/api/authtoken"
	"github.com/DataDog/datadog-agent/comp/api/authtoken/fetchonlyimpl"
	"github.com/DataDog/datadog-agent/comp/collector/collector"
	"github.com/DataDog/datadog-agent/comp/collector/collector/onlycollectorimpl"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/proto"
	"github.com/DataDog/datadog-agent/comp/core/config"

	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logfx "github.com/DataDog/datadog-agent/comp/core/log/fx"
	"github.com/DataDog/datadog-agent/comp/core/pid"
	"github.com/DataDog/datadog-agent/comp/core/pid/pidimpl"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/comp/core/secrets/secretsimpl"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	remoteTagger "github.com/DataDog/datadog-agent/comp/core/tagger/fx-remote"
	taggerTypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/telemetryimpl"
	haagentfxnoop "github.com/DataDog/datadog-agent/comp/haagent/fx-noop"
	integrations "github.com/DataDog/datadog-agent/comp/logs/integrations/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/api/security"
	pkgcollector "github.com/DataDog/datadog-agent/pkg/collector"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/collector/check/stats"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/serializer/types"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

type CLIParams struct {
	confPath    string
	pidfilePath string
}

type memoryStats struct {
	heapSize uint64
}

type mockSender struct{}

func (m *mockSender) Commit()                                                                     {}
func (m *mockSender) Gauge(metric string, value float64, hostname string, tags []string)          {}
func (m *mockSender) GaugeNoIndex(metric string, value float64, hostname string, tags []string)   {}
func (m *mockSender) Rate(metric string, value float64, hostname string, tags []string)           {}
func (m *mockSender) Count(metric string, value float64, hostname string, tags []string)          {}
func (m *mockSender) MonotonicCount(metric string, value float64, hostname string, tags []string) {}
func (m *mockSender) MonotonicCountWithFlushFirstValue(metric string, value float64, hostname string, tags []string, flushFirstValue bool) {
}
func (m *mockSender) Counter(metric string, value float64, hostname string, tags []string)      {}
func (m *mockSender) Histogram(metric string, value float64, hostname string, tags []string)    {}
func (m *mockSender) Historate(metric string, value float64, hostname string, tags []string)    {}
func (m *mockSender) Distribution(metric string, value float64, hostname string, tags []string) {}
func (m *mockSender) ServiceCheck(checkName string, status servicecheck.ServiceCheckStatus, hostname string, tags []string, message string) {
}
func (m *mockSender) HistogramBucket(metric string, value int64, lowerBound, upperBound float64, monotonic bool, hostname string, tags []string, flushFirstValue bool) {
}
func (m *mockSender) GaugeWithTimestamp(metric string, value float64, hostname string, tags []string, timestamp float64) error {
	return nil
}
func (m *mockSender) CountWithTimestamp(metric string, value float64, hostname string, tags []string, timestamp float64) error {
	return nil
}
func (m *mockSender) Event(e event.Event)                                  {}
func (m *mockSender) EventPlatformEvent(rawEvent []byte, eventType string) {}
func (m *mockSender) GetSenderStats() stats.SenderStats {
	return stats.SenderStats{}
}
func (m *mockSender) DisableDefaultHostname(disable bool) {}
func (m *mockSender) SetCheckCustomTags(tags []string)    {}
func (m *mockSender) SetCheckService(service string)      {}
func (m *mockSender) SetNoIndex(noIndex bool)             {}
func (m *mockSender) FinalizeCheckServiceTag()            {}
func (m *mockSender) OrchestratorMetadata(msgs []types.ProcessMessageBody, clusterID string, nodeType int) {
}
func (m *mockSender) OrchestratorManifest(msgs []types.ProcessMessageBody, clusterID string) {}

type mockSenderManager struct{}

func (m *mockSenderManager) GetSender(id checkid.ID) (sender.Sender, error) {
	return &mockSender{}, nil
}
func (m *mockSenderManager) SetSender(sender.Sender, checkid.ID) error {
	return nil
}
func (m *mockSenderManager) DestroySender(id checkid.ID) {

}
func (m *mockSenderManager) GetDefaultSender() (sender.Sender, error) {
	return &mockSender{}, nil
}

// MakeCommand returns the start subcommand for the 'dogstatsd' command.
func MakeCommand() *cobra.Command {
	cliParams := &CLIParams{}
	startCmd := &cobra.Command{
		Use:   "start",
		Short: "Start Checks Agent",
		Long:  `Runs Checks Agent in the foreground`,
		RunE: func(*cobra.Command, []string) error {
			heapSize := getAlloc()

			fmt.Printf("value before Fx components = %d KB\n", heapSize)
			return RunChecksAgent(cliParams, "", start, memoryStats{
				heapSize: heapSize,
			})
		},
	}

	// local flags
	startCmd.PersistentFlags().StringVarP(&cliParams.confPath, "cfgpath", "c", "", "path to directory containing datadog.yaml")
	startCmd.Flags().StringVarP(&cliParams.pidfilePath, "pidfile", "p", "", "path to the pidfile")

	return startCmd
}

func RunChecksAgent(cliParams *CLIParams, defaultConfPath string, fct interface{}, memstats memoryStats) error {
	return fxutil.OneShot(fct,
		fx.Supply(cliParams),
		fx.Supply(memstats),

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
		fx.Supply(log.ForDaemon("CA", "log_file", "/var/log/datadog/checks-agent.log")),

		// Secrets management
		fx.Provide(func(comp secrets.Component) option.Option[secrets.Component] {
			return option.New[secrets.Component](comp)
		}),
		fx.Supply(secrets.NewEnabledParams()),
		secretsimpl.Module(),
		telemetryimpl.Module(),
		onlycollectorimpl.Module(),
		hostnameimpl.Module(),
		remoteTagger.Module(tagger.RemoteParams{
			RemoteTarget: func(c config.Component) (string, error) {
				return fmt.Sprintf(":%v", c.GetInt("cmd_port")), nil
			},
			RemoteTokenFetcher: func(c config.Component) func() (string, error) {
				return func() (string, error) {
					return security.FetchAuthToken(c)
				}
			},
			RemoteFilter: taggerTypes.NewMatchAllFilter(),
		}),

		fetchonlyimpl.Module(),
		haagentfxnoop.Module(),

		pidimpl.Module(),
		fx.Supply(pidimpl.NewParams(cliParams.pidfilePath)),
		fx.Provide(func() sender.SenderManager {
			return &mockSenderManager{}
		}),
	)
}

func start(
	cliParams *CLIParams,
	memoryStats memoryStats,
	config config.Component,
	log log.Component,
	collector collector.Component,
	senderManager sender.SenderManager,
	tagger tagger.Component,
	authToken authtoken.Component,
	telemetry telemetry.Component,
	_ pid.Component,
) error {
	currentheapSize := getAlloc()
	fmt.Printf("value after Fx components = %d KB\n", currentheapSize)
	fmt.Printf("heap size delta after components initialization = %d KB\n", heapDelta(memoryStats.heapSize, currentheapSize))

	// Free memory after initializing all components to return memory to the OS
	// as soon as possible. That way RSS will be lower
	debug.FreeOSMemory()

	currentheapSize = getAlloc()
	fmt.Printf("value after FreeOSMemory = %d KB\n", currentheapSize)
	fmt.Printf("heap size delta after FreeOSMemory = %d KB\n", heapDelta(memoryStats.heapSize, currentheapSize))
	// Main context passed to components
	ctx, cancel := context.WithCancel(context.Background())

	defer StopAgent(cancel, log)

	startPprof(config, telemetry)

	token := authToken.Get()

	if token == "" {
		return fmt.Errorf("unable to fetch authentication token")
	}

	md := metadata.MD{
		"authorization": []string{fmt.Sprintf("Bearer %s", token)},
	}
	ctx, StreamCancel := context.WithCancel(metadata.NewOutgoingContext(ctx, md))
	defer StreamCancel()

	// NOTE: we're using InsecureSkipVerify because the gRPC server only
	// persists its TLS certs in memory, and we currently have no
	// infrastructure to make them available to clients. This is NOT
	// equivalent to grpc.WithInsecure(), since that assumes a non-TLS
	// connection.
	creds := credentials.NewTLS(&tls.Config{
		InsecureSkipVerify: true,
	})

	conn, err := grpc.DialContext( //nolint:staticcheck // TODO (ASC) fix grpc.DialContext is deprecated
		ctx,
		fmt.Sprintf(":%v", config.GetInt("cmd_port")),
		grpc.WithTransportCredentials(creds),
	)
	if err != nil {
		return err
	}

	client := core.NewAgentSecureClient(conn)

	// TODO: figure out how to initial.ize checks contexts
	// check.InitializeInventoryChecksContext(invChecks)
	scheduler := pkgcollector.InitCheckScheduler(option.New(collector), &mockSenderManager{}, option.None[integrations.Component](), tagger)

	// Start the scheduler
	go startScheduler(ctx, StreamCancel, client, scheduler, log)

	stopCh := make(chan struct{})
	go handleSignals(stopCh, log)

	err = Run(ctx, cliParams, config, log)
	if err != nil {
		return err
	}

	// Block here until we receive a stop signal
	<-stopCh

	return nil
}

// Run starts the Logs agent server
func Run(ctx context.Context, cliParams *CLIParams, config config.Component, log log.Component) (err error) {
	if len(cliParams.confPath) == 0 {
		log.Infof("Config will be read from env variables")
	}

	if !config.IsSet("api_key") {
		err = log.Critical("no API key configured, exiting")
		return
	}

	return nil
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
		log.Warnf("Logs Agent health unknown: %s", err)
	} else if len(health.Unhealthy) > 0 {
		log.Warnf("Some components were unhealthy: %v", health.Unhealthy)
	}

	// gracefully shut down any component
	cancel()

	log.Info("See ya!")
	log.Flush()
}

// type auConfig struct {
// 	integration.Config
// 	EventType string `json:"event_type,omitempty"`
// }

// type results struct {
// 	Results map[string][]auConfig `json:"result,omitempty"`
// }

type autodiscoveryStream struct {
	autodiscoveryStream       core.AgentSecure_AutodiscoveryStreamConfigClient
	autodiscoveryStreamCancel context.CancelFunc
}

func (a *autodiscoveryStream) initStream(ctx context.Context, client core.AgentSecureClient, log log.Component) error {
	expBackoff := backoff.NewExponentialBackOff()
	expBackoff.InitialInterval = 500 * time.Millisecond
	expBackoff.MaxInterval = 5 * time.Minute
	expBackoff.MaxElapsedTime = 0 * time.Minute

	return backoff.Retry(func() error {
		select {
		case <-ctx.Done():
			return &backoff.PermanentError{}
		default:
		}

		stream, err := client.AutodiscoveryStreamConfig(ctx, nil)
		if err != nil {
			log.Infof("unable to establish stream, will possibly retry: %s", err)
			// We need to handle the case that the kernel agent dies
			return err
		}

		a.autodiscoveryStream = stream

		log.Info("autodiscovery stream established successfully")
		return nil
	}, expBackoff)
}

func startScheduler(ctx context.Context, f context.CancelFunc, client core.AgentSecureClient, scheduler *pkgcollector.CheckScheduler, log log.Component) {
	// Start a stream using the grpc Client to consume autodiscovery updates for the different configurations
	autodiscoveryStream := &autodiscoveryStream{
		autodiscoveryStreamCancel: f,
	}

	for {
		if autodiscoveryStream.autodiscoveryStream == nil {
			err := autodiscoveryStream.initStream(ctx, client, log)
			if err != nil {
				log.Warnf("error received trying to start stream: %s", err)
				continue
			}
		}
		log.Infof("autodiscoveryStream: %+v\n", autodiscoveryStream.autodiscoveryStream)

		streamConfigs, err := autodiscoveryStream.autodiscoveryStream.Recv()

		if err != nil {
			autodiscoveryStream.autodiscoveryStreamCancel()

			autodiscoveryStream.autodiscoveryStream = nil

			if err != io.EOF {
				log.Warnf("error received from autodiscovery stream: %s", err)
			}

			continue
		}

		scheduleConfigs := []integration.Config{}
		unscheduleConfigs := []integration.Config{}

		for _, config := range streamConfigs.Configs {
			log.Infof("received autodiscovery scheduler config event %s for check %s", config.EventType, config.CheckName)
			if config.EventType == core.ConfigEventType_SCHEDULE.String() {
				scheduleConfigs = append(scheduleConfigs, proto.AutodiscoveryConfigFromProtobufConfig(config))
			} else if config.EventType == core.ConfigEventType_UNSCHEDULE.String() {
				unscheduleConfigs = append(unscheduleConfigs, proto.AutodiscoveryConfigFromProtobufConfig(config))
			}
		}

		scheduler.Schedule(scheduleConfigs)
		scheduler.Unschedule(unscheduleConfigs)
	}
}

// heapDelta returns the delta in KB between
// the current heap size and the previous heap size.
func heapDelta(prev, cur uint64) uint64 {
	if cur < prev {
		return 0
	}
	return cur - prev
}

// getAlloc returns the current heap size in KB.
func getAlloc() uint64 {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.Alloc / 1024
}

func setupInternalProfiling(_ config.Component) error {
	return nil

}

func startPprof(_ config.Component, _ telemetry.Component) {}
