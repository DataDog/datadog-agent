// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package gnmi implements the 'agent gnmi' troubleshooting subcommands.
package gnmi

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/fx"
	"go.yaml.in/yaml/v2"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/gnmi/client"
	gnmicfg "github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/gnmi/config"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type cliParams struct {
	*command.GlobalParams

	address            string
	port               int
	username           string
	password           string
	profile            string
	collectTopology    bool
	useTLS             bool
	insecureSkipVerify bool
	instanceIndex      int
	interval           time.Duration
	fastReconnect      bool
	listPaths          bool
}

type gnmiInstanceFile struct {
	Instances []gnmicfg.InstanceConfig `yaml:"instances"`
}

// Commands returns a slice of subcommands for the 'agent' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	params := &cliParams{
		GlobalParams: globalParams,
		port:            0,
		instanceIndex:   -1,
		interval:     2 * time.Second,
	}

	gnmiCmd := &cobra.Command{
		Use:   "gnmi",
		Short: "gNMI troubleshooting tools",
	}

	subscribeCmd := &cobra.Command{
		Use:   "subscribe",
		Short: "Subscribe to a gNMI target and print cached values on stdout",
		Long: `Open a gNMI subscribe stream and periodically print connection status and
cached path values. Useful for debugging transport, auth, and profile issues
without running the full agent.

Provide connection settings with flags, or load an instance from conf.d/gnmi.d/conf.yaml
with --instance.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			return fxutil.OneShot(
				runSubscribe,
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

	subscribeCmd.Flags().StringVar(&params.address, "address", "", "gNMI target address")
	subscribeCmd.Flags().IntVar(&params.port, "port", 0, "gNMI target port (default: from instance config or 57400)")
	subscribeCmd.Flags().StringVar(&params.username, "username", "", "gNMI username")
	subscribeCmd.Flags().StringVar(&params.password, "password", "", "gNMI password")
	subscribeCmd.Flags().StringVar(&params.profile, "profile", "", "Profile name or path under conf.d/gnmi.d/profiles/")
	subscribeCmd.Flags().BoolVar(&params.collectTopology, "collect-topology", false, "Subscribe to LLDP topology paths")
	subscribeCmd.Flags().BoolVar(&params.useTLS, "use-tls", false, "Use TLS for the gRPC transport")
	subscribeCmd.Flags().BoolVar(&params.insecureSkipVerify, "insecure-skip-verify", false, "Skip TLS certificate verification")
	subscribeCmd.Flags().IntVar(&params.instanceIndex, "instance", -1, "Load settings from conf.d/gnmi.d/conf.yaml instance index")
	subscribeCmd.Flags().DurationVar(&params.interval, "interval", 2*time.Second, "How often to print status and cached values")
	subscribeCmd.Flags().BoolVar(&params.fastReconnect, "fast-reconnect", true, "Use shorter reconnect backoff for interactive debugging")
	subscribeCmd.Flags().BoolVar(&params.listPaths, "list-paths", false, "Print subscription paths and exit without subscribing")

	gnmiCmd.AddCommand(subscribeCmd)
	return []*cobra.Command{gnmiCmd}
}

func runSubscribe(params *cliParams, config config.Component) error {
	instance, profile, err := resolveSubscribeTarget(config, params)
	if err != nil {
		return err
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
	}

	paths := client.SubscriptionPaths(clientCfg)
	if params.listPaths {
		for _, spec := range paths {
			fmt.Println(spec.String())
		}
		return nil
	}

	opts := []client.Option{}
	if params.fastReconnect {
		opts = append(opts, client.WithReconnectDelays(200*time.Millisecond, 2*time.Second))
	}

	gnmiClient, err := client.New(clientCfg, opts...)
	if err != nil {
		return err
	}

	fmt.Printf("target=%s:%d transport=%s profile=%q paths=%d\n",
		instance.Address,
		instance.Port,
		gnmiClient.TransportMode(),
		profile.Name,
		len(paths),
	)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if err := gnmiClient.Start(ctx); err != nil {
		return err
	}
	defer gnmiClient.Close() //nolint:errcheck

	ticker := time.NewTicker(params.interval)
	defer ticker.Stop()

	printSnapshot(gnmiClient)
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			printSnapshot(gnmiClient)
		}
	}
}

func resolveSubscribeTarget(config config.Component, params *cliParams) (*gnmicfg.InstanceConfig, *gnmicfg.ProfileDefinition, error) {
	instance := gnmicfg.InstanceConfig{
		Port:               params.port,
		CollectTopology:    params.collectTopology,
		UseTLS:             params.useTLS,
		InsecureSkipVerify: params.insecureSkipVerify,
	}

	if params.instanceIndex >= 0 {
		loaded, err := loadInstanceFromConfig(config, params.instanceIndex)
		if err != nil {
			return nil, nil, err
		}
		instance = *loaded
	}

	if params.address != "" {
		instance.Address = params.address
	}
	if params.username != "" {
		instance.Username = params.username
	}
	if params.password != "" {
		instance.Password = params.password
	}
	if params.profile != "" {
		instance.Profile = params.profile
	}
	if params.port > 0 {
		instance.Port = params.port
	} else if instance.Port == 0 {
		instance.Port = gnmicfg.DefaultPort
	}

	if instance.Address == "" {
		return nil, nil, fmt.Errorf("address is required (use --address or --instance)")
	}
	if instance.Username == "" {
		return nil, nil, fmt.Errorf("username is required")
	}
	if instance.Password == "" {
		return nil, nil, fmt.Errorf("password is required")
	}
	if instance.Profile == "" {
		return nil, nil, fmt.Errorf("profile is required")
	}

	profile, err := gnmicfg.LoadProfile(instance.Profile)
	if err != nil {
		return nil, nil, err
	}

	return &instance, profile, nil
}

func loadInstanceFromConfig(config config.Component, index int) (*gnmicfg.InstanceConfig, error) {
	confPath := filepath.Join(config.GetString("confd_path"), "gnmi.d", "conf.yaml")
	buf, err := os.ReadFile(confPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", confPath, err)
	}

	var instances gnmiInstanceFile
	if err := yaml.Unmarshal(buf, &instances); err != nil {
		return nil, fmt.Errorf("parse %s: %w", confPath, err)
	}
	if index < 0 || index >= len(instances.Instances) {
		return nil, fmt.Errorf("instance index %d out of range (found %d instance(s) in %s)", index, len(instances.Instances), confPath)
	}

	return &instances.Instances[index], nil
}

func printSnapshot(gnmiClient *client.Client) {
	connStatus := gnmiClient.ConnectionStatus()
	fmt.Printf("--- stream=%s reconnect=%d samples=%d",
		gnmiClient.StreamState(),
		gnmiClient.ReconnectAttempts(),
		gnmiClient.ReceivedSamples(),
	)
	if connStatus.LastError != "" {
		fmt.Printf(" last_error=%q", connStatus.LastError)
	}
	if !connStatus.NextReconnectAt.IsZero() {
		fmt.Printf(" next_reconnect=%s", connStatus.NextReconnectAt.Format(time.RFC3339))
	}
	fmt.Println()

	snapshot := gnmiClient.Snapshot()
	sort.Slice(snapshot, func(i, j int) bool {
		left := formatCacheKey(snapshot[i])
		right := formatCacheKey(snapshot[j])
		return left < right
	})

	if len(snapshot) == 0 {
		fmt.Println("(no cached values yet)")
		return
	}

	for _, item := range snapshot {
		fmt.Printf("%s value=%v updated=%s\n",
			formatCacheKey(item),
			item.Entry.Value,
			item.Entry.Timestamp.Format(time.RFC3339),
		)
	}
}

func formatCacheKey(item client.CachedValue) string {
	key := item.Key.Path
	if len(item.Key.Keys) == 0 {
		return key
	}

	parts := make([]string, 0, len(item.Key.Keys))
	for segment, value := range item.Key.Keys {
		parts = append(parts, fmt.Sprintf("%s=%s", segment, value))
	}
	sort.Strings(parts)
	return key + "{" + strings.Join(parts, ",") + "}"
}
