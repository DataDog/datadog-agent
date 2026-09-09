// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package gnmi

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/gnmi/client"
	gnmicfg "github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/gnmi/config"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/gnmi/report"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type previewMetricsParams struct {
	*cliParams

	includeHealth          bool
	includeInterfaceStatus bool
	includeMetadata        bool
	waitSync               bool
	once                   bool
	timeout                time.Duration
}

func previewMetricsCommand(globalParams *command.GlobalParams, baseParams *cliParams) *cobra.Command {
	params := &previewMetricsParams{
		cliParams:              baseParams,
		includeHealth:          true,
		includeInterfaceStatus: true,
		includeMetadata:        true,
		waitSync:               true,
	}

	cmd := &cobra.Command{
		Use:   "preview-metrics",
		Short: "Subscribe to a gNMI target and print metrics that would be emitted",
		Long: `Open a gNMI subscribe stream and periodically run the same reporting path as the
gNMI check, printing metric names, values, and tags to stdout instead of sending
them to the Datadog intake.

Provide connection settings with flags, or load an instance from conf.d/gnmi.d/conf.yaml
with --instance.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			return fxutil.OneShot(
				runPreviewMetrics,
				fx.Supply(params),
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewAgentParams(
						globalParams.ConfFilePath,
						config.WithExtraConfFiles(globalParams.ExtraConfFilePath),
						config.WithFleetPoliciesDirPath(globalParams.FleetPoliciesDirPath),
					),
					LogParams: log.ForOneShot(command.LoggerName, "off", true),
				}),
				core.Bundle(),
			)
		},
	}

	registerTargetFlags(cmd, baseParams)
	cmd.Flags().BoolVar(&params.includeHealth, "include-health", true, "Include datadog.gnmi.* operational metrics")
	cmd.Flags().BoolVar(&params.includeInterfaceStatus, "include-interface-status", true, "Include snmp.interface.status metrics when inventory is ready")
	cmd.Flags().BoolVar(&params.includeMetadata, "include-metadata", true, "Include network-devices-metadata event payloads when inventory is ready")
	cmd.Flags().BoolVar(&params.waitSync, "wait-sync", true, "Wait for stream synchronization before printing metrics")
	cmd.Flags().BoolVar(&params.once, "once", false, "Collect metrics once and exit after synchronization")
	cmd.Flags().DurationVar(&params.timeout, "timeout", 30*time.Second, "Max time to wait for stream synchronization with --once")

	return cmd
}

func runPreviewMetrics(params *previewMetricsParams, config config.Component) error {
	instance, profile, err := resolveSubscribeTarget(config, params.cliParams)
	if err != nil {
		return err
	}

	encoding, err := instance.ResolvedEncoding()
	if err != nil {
		return err
	}
	if params.encoding != "" {
		encoding, err = gnmicfg.ParseEncoding(params.encoding)
		if err != nil {
			return err
		}
	}

	clientCfg := client.Config{
		Address:            instance.Address,
		Port:               instance.Port,
		Username:           instance.Username,
		Password:           instance.Password,
		Profile:            *profile,
		CollectTopology:    instance.CollectTopology,
		UseTLS:             instance.UseTLS,
		InsecureSkipVerify: instance.InsecureSkipVerify,
		Encoding:           encoding,
	}

	opts := []client.Option{}
	if params.fastReconnect {
		opts = append(opts, client.WithReconnectDelays(200*time.Millisecond, 2*time.Second))
	}

	gnmiClient, err := client.New(clientCfg, opts...)
	if err != nil {
		return err
	}

	checkCfg := &gnmicfg.CheckConfig{
		Instance: *instance,
		Profile:  *profile,
	}

	fmt.Printf("target=%s:%d transport=%s encoding=%s profile=%q\n",
		instance.Address,
		instance.Port,
		gnmiClient.TransportMode(),
		gnmicfg.EncodingName(clientCfg.Encoding),
		profile.Name,
	)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if err := gnmiClient.Start(ctx); err != nil {
		return err
	}
	defer gnmiClient.Close() //nolint:errcheck

	capture := report.NewCaptureSender()
	bandwidthState := report.NewBandwidthState()
	previewOpts := report.PreviewOptions{
		IncludeHealth:          params.includeHealth,
		IncludeInterfaceStatus: params.includeInterfaceStatus,
		IncludeMetadata:        params.includeMetadata,
	}
	var lastMetadataReport time.Time

	ticker := time.NewTicker(params.interval)
	defer ticker.Stop()

	deadline := time.Now().Add(params.timeout)
	for {
		now := time.Now()
		if params.waitSync && !gnmiClient.Synchronized() {
			printPreviewStatus(gnmiClient, 0, 0)
			fmt.Println("(waiting for stream synchronization)")
			if params.once && now.After(deadline) {
				return syncWaitTimeoutError(gnmiClient, params.timeout)
			}
		} else {
			lastMetadataReport, err = report.PreviewCollection(
				capture,
				checkCfg,
				gnmiClient,
				bandwidthState,
				lastMetadataReport,
				now,
				previewOpts,
			)
			if err != nil {
				return err
			}
			printPreviewStatus(gnmiClient, len(capture.Metrics()), len(capture.MetadataEvents()))
			printCapturedMetrics(capture.Metrics())
			printCapturedMetadata(capture.MetadataEvents())

			if params.once {
				return nil
			}
		}

		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func syncWaitTimeoutError(gnmiClient *client.Client, timeout time.Duration) error {
	connStatus := gnmiClient.ConnectionStatus()
	if connStatus.LastError != "" {
		return fmt.Errorf("timed out after %s waiting for stream synchronization: %s", timeout, connStatus.LastError)
	}
	return fmt.Errorf("timed out after %s waiting for stream synchronization", timeout)
}

func printPreviewStatus(gnmiClient *client.Client, metricCount int, metadataCount int) {
	connStatus := gnmiClient.ConnectionStatus()
	fmt.Printf("--- stream=%s synchronized=%t reconnect=%d samples=%d metrics=%d metadata=%d",
		gnmiClient.StreamState(),
		gnmiClient.Synchronized(),
		gnmiClient.ReconnectAttempts(),
		gnmiClient.ReceivedSamples(),
		metricCount,
		metadataCount,
	)
	if connStatus.LastError != "" {
		fmt.Printf(" last_error=%q", connStatus.LastError)
	}
	if !connStatus.NextReconnectAt.IsZero() {
		fmt.Printf(" next_reconnect=%s", connStatus.NextReconnectAt.Format(time.RFC3339))
	}
	fmt.Println()
}

func printCapturedMetrics(metrics []report.CapturedMetric) {
	if len(metrics) == 0 {
		fmt.Println("(no metrics emitted)")
		return
	}

	sorted := append([]report.CapturedMetric(nil), metrics...)
	sort.Slice(sorted, func(i, j int) bool {
		left := sorted[i].Name + "|" + sorted[i].Type
		right := sorted[j].Name + "|" + sorted[j].Type
		if left != right {
			return left < right
		}
		leftTags := append([]string(nil), sorted[i].Tags...)
		rightTags := append([]string(nil), sorted[j].Tags...)
		sort.Strings(leftTags)
		sort.Strings(rightTags)
		return strings.Join(leftTags, ",") < strings.Join(rightTags, ",")
	})

	for _, metric := range sorted {
		fmt.Println(report.FormatCapturedMetric(metric))
	}
}

func printCapturedMetadata(events []report.CapturedMetadataEvent) {
	if len(events) == 0 {
		return
	}

	for _, event := range events {
		fmt.Println(report.FormatCapturedMetadataEvent(event))
	}
}

func registerTargetFlags(cmd *cobra.Command, params *cliParams) {
	cmd.Flags().StringVar(&params.address, "address", "", "gNMI target address")
	cmd.Flags().IntVar(&params.port, "port", 0, "gNMI target port (default: from instance config or 57400)")
	cmd.Flags().StringVar(&params.username, "username", "", "gNMI username")
	cmd.Flags().StringVar(&params.password, "password", "", "gNMI password")
	cmd.Flags().StringVar(&params.profile, "profile", "", "Profile name or path under conf.d/gnmi.d/profiles/")
	cmd.Flags().BoolVar(&params.collectTopology, "collect-topology", false, "Subscribe to LLDP topology paths")
	cmd.Flags().BoolVar(&params.useTLS, "use-tls", false, "Use TLS for the gRPC transport")
	cmd.Flags().BoolVar(&params.insecureSkipVerify, "insecure-skip-verify", false, "Skip TLS certificate verification")
	cmd.Flags().StringVar(&params.encoding, "encoding", "", "gNMI encoding: proto, json, or json_ietf (default: json_ietf)")
	cmd.Flags().IntVar(&params.instanceIndex, "instance", -1, "Load settings from conf.d/gnmi.d/conf.yaml instance index")
	cmd.Flags().StringVar(&params.confFile, "conf-file", "", "Path to conf.d/gnmi.d/conf.yaml (overrides confd_path for instance loading)")
	cmd.Flags().DurationVar(&params.interval, "interval", 2*time.Second, "How often to collect and print metrics")
	cmd.Flags().BoolVar(&params.fastReconnect, "fast-reconnect", true, "Use shorter reconnect backoff for interactive debugging")
}
