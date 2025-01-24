// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO Fix revive linter
package start

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	// _ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer"
	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer/demultiplexerimpl"
	"github.com/DataDog/datadog-agent/comp/api/authtoken"
	"github.com/DataDog/datadog-agent/comp/api/authtoken/fetchonlyimpl"
	"github.com/DataDog/datadog-agent/comp/collector/collector"
	"github.com/DataDog/datadog-agent/comp/collector/collector/collectorimpl"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
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
	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/noopsimpl"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/eventplatformimpl"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatformreceiver/eventplatformreceiverimpl"
	orchestratorForwarderImpl "github.com/DataDog/datadog-agent/comp/forwarder/orchestrator/orchestratorimpl"
	haagentfx "github.com/DataDog/datadog-agent/comp/haagent/fx"
	integrations "github.com/DataDog/datadog-agent/comp/logs/integrations/def"
	logscompressionimpl "github.com/DataDog/datadog-agent/comp/serializer/logscompression/fx"
	metricscompressionimpl "github.com/DataDog/datadog-agent/comp/serializer/metricscompression/fx"
	"github.com/DataDog/datadog-agent/pkg/api/security"
	pkgcollector "github.com/DataDog/datadog-agent/pkg/collector"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

type CLIParams struct {
	confPath    string
	pidfilePath string
}

// MakeCommand returns the start subcommand for the 'dogstatsd' command.
func MakeCommand() *cobra.Command {
	cliParams := &CLIParams{}
	startCmd := &cobra.Command{
		Use:   "start",
		Short: "Start Checks Agent",
		Long:  `Runs Checks Agent in the foreground`,
		RunE: func(*cobra.Command, []string) error {
			return RunChecksAgent(cliParams, "", start)
		},
	}

	// local flags
	startCmd.PersistentFlags().StringVarP(&cliParams.confPath, "cfgpath", "c", "", "path to directory containing datadog.yaml")
	startCmd.Flags().StringVarP(&cliParams.pidfilePath, "pidfile", "p", "", "path to the pidfile")

	return startCmd
}

func RunChecksAgent(cliParams *CLIParams, defaultConfPath string, fct interface{}) error {
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
		fx.Supply(log.ForDaemon("CA", "log_file", "/var/log/datadog/checks-agent.log")),

		// Secrets management
		fx.Provide(func(comp secrets.Component) option.Option[secrets.Component] {
			return option.New[secrets.Component](comp)
		}),
		fx.Supply(secrets.NewEnabledParams()),
		secretsimpl.Module(),
		telemetryimpl.Module(),
		collectorimpl.Module(),
		// Sending metrics to the backend
		metricscompressionimpl.Module(),
		demultiplexerimpl.Module(demultiplexerimpl.NewDefaultParams()),
		orchestratorForwarderImpl.Module(orchestratorForwarderImpl.NewDisabledParams()),
		eventplatformimpl.Module(eventplatformimpl.NewDisabledParams()),
		eventplatformreceiverimpl.Module(),
		defaultforwarder.Module(defaultforwarder.NewParams()),
		logscompressionimpl.Module(),
		// injecting the shared Serializer to FX until we migrate it to a proper component. This allows other
		// already migrated components to request it.
		fx.Provide(func(demuxInstance demultiplexer.Component) serializer.MetricSerializer {
			return demuxInstance.Serializer()
		}),

		fx.Provide(func(ms serializer.MetricSerializer) option.Option[serializer.MetricSerializer] {
			return option.New[serializer.MetricSerializer](ms)
		}),
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
		haagentfx.Module(),

		pidimpl.Module(),
		fx.Supply(pidimpl.NewParams(cliParams.pidfilePath)),
	)
}

// Custom HTTP client with Authorization header middleware
type customClient struct {
	client *http.Client
	token  string
}

func (c *customClient) Do(req *http.Request) (*http.Response, error) {
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.token))
	return c.client.Do(req)
}

func start(
	cliParams *CLIParams,
	config config.Component,
	log log.Component,
	collector collector.Component,
	demultiplexer demultiplexer.Component,
	tagger tagger.Component,
	authToken authtoken.Component,
	telemetry telemetry.Component,
	_ pid.Component,
) error {

	// Main context passed to components
	ctx, cancel := context.WithCancel(context.Background())

	defer StopAgent(cancel, log)

	// go func() {
	// 	port := config.GetString("checks_agent_debug_port")
	// 	addr := net.JoinHostPort("localhost", port)
	// 	http.Handle("/telemetry", telemetry.Handler())
	// 	err := http.ListenAndServe(addr, nil)
	// 	if err != nil {
	// 		log.Warnf("pprof server: %s", err)
	// 	}
	// }()

	token := authToken.Get()

	if token == "" {
		return fmt.Errorf("unable to fetch authentication token")
	}

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	customClient := &customClient{
		client: client,
		token:  token,
	}

	// TODO: figure out how to initial.ize checks context
	// check.InitializeInventoryChecksContext(invChecks)

	scheduler := pkgcollector.InitCheckScheduler(option.New(collector), demultiplexer, option.None[integrations.Component](), tagger)

	// // Start the scheduler
	go startScheduler(ctx, customClient, scheduler, log, config)

	stopCh := make(chan struct{})
	go handleSignals(stopCh, log)

	err := Run(ctx, cliParams, config, log)
	if err != nil {
		return err
	}

	// if err := setupInternalProfiling(config); err != nil {
	// 	return log.Errorf("Error while setuping internal profiling, exiting: %v", err)
	// }

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

// type autodiscoveryStream struct {
// 	autodiscoveryStream       core.AgentSecure_AutodiscoveryStreamConfigClient
// 	autodiscoveryStreamCancel context.CancelFunc
// }

// func (a *autodiscoveryStream) initStream(ctx context.Context, client core.AgentSecureClient, log log.Component) error {
// 	expBackoff := backoff.NewExponentialBackOff()
// 	expBackoff.InitialInterval = 500 * time.Millisecond
// 	expBackoff.MaxInterval = 5 * time.Minute
// 	expBackoff.MaxElapsedTime = 0 * time.Minute

// 	return backoff.Retry(func() error {
// 		select {
// 		case <-ctx.Done():
// 			return &backoff.PermanentError{}
// 		default:
// 		}

// 		stream, err := client.AutodiscoveryStreamConfig(ctx, nil)
// 		if err != nil {
// 			log.Infof("unable to establish stream, will possibly retry: %s", err)
// 			// We need to handle the case that the kernel agent dies
// 			return err
// 		}

// 		a.autodiscoveryStream = stream

// 		log.Info("autodiscovery stream established successfully")
// 		return nil
// 	}, expBackoff)
// }

type auConfig struct {
	integration.Config
	EventType string `json:"event_type,omitempty"`
}

type results struct {
	Results map[string][]auConfig `json:"result,omitempty"`
}

func startScheduler(ctx context.Context, client *customClient, scheduler *pkgcollector.CheckScheduler, log log.Component, config config.Component) {
	url := fmt.Sprintf("https://localhost:%v/v1/grpc/autodiscovery/stream_configs", config.GetInt("cmd_port"))
	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		log.Warnf("Failed to create request: %v", err)
		return
	}
	// Send the HTTP request
	resp, err := client.Do(req)
	if err != nil {
		log.Warnf("Failed to send request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Warnf("Received non-200 response: %d %s", resp.StatusCode, resp.Status)
	}

	log.Info("Received 200 response: %d %s", resp.StatusCode, resp.Status)

	// Read the streaming response
	reader := bufio.NewReader(resp.Body)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				fmt.Println("Stream closed by server")
				break
			}
			log.Warnf("Error reading response: %v", err)
		}
		results := results{}
		err = json.Unmarshal([]byte(line), &results)
		if err != nil {
			log.Warnf("Failed to parse json: %v", err)
			break
		}

		scheduleConfigs := []integration.Config{}
		unscheduleConfigs := []integration.Config{}

		for _, configs := range results.Results {
			for _, config := range configs {
				log.Infof("received autodiscovery scheduler config event %s for check %s", config.EventType, config.Name)
				if config.EventType == "SCHEDULE" {
					scheduleConfigs = append(scheduleConfigs, config.Config)
				} else if config.EventType == "UNSCHEDULE" {
					unscheduleConfigs = append(scheduleConfigs, config.Config)
				}
			}
		}

		scheduler.Schedule(scheduleConfigs)
		scheduler.Unschedule(unscheduleConfigs)
	}
}

// func setupInternalProfiling(config config.Component) error {
// 	runtime.MemProfileRate = 1
// 	site := fmt.Sprintf(profiling.ProfilingURLTemplate, config.GetString("site"))

// 	// We need the trace agent runnning to send profiles
// 	profSettings := profiling.Settings{
// 		ProfilingURL:         site,
// 		Socket:               "/var/run/datadog/apm.socket",
// 		Env:                  "local",
// 		Service:              "checks-agent",
// 		Period:               config.GetDuration("internal_profiling.period"),
// 		CPUDuration:          config.GetDuration("internal_profiling.cpu_duration"),
// 		MutexProfileFraction: config.GetInt("internal_profiling.mutex_profile_fraction"),
// 		BlockProfileRate:     config.GetInt("internal_profiling.block_profile_rate"),
// 		WithGoroutineProfile: config.GetBool("internal_profiling.enable_goroutine_stacktraces"),
// 		WithBlockProfile:     config.GetBool("internal_profiling.enable_block_profiling"),
// 		WithMutexProfile:     config.GetBool("internal_profiling.enable_mutex_profiling"),
// 		WithDeltaProfiles:    config.GetBool("internal_profiling.delta_profiles"),
// 		CustomAttributes:     config.GetStringSlice("internal_profiling.custom_attributes"),
// 	}

// 	return profiling.Start(profSettings)
// }
