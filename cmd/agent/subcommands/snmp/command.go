// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package snmp implements the 'agent snmp' subcommand.
package snmp

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"

	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/comp/aggregator"
	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer/demultiplexerimpl"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipcfx "github.com/DataDog/datadog-agent/comp/core/ipc/fx"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	nooptagger "github.com/DataDog/datadog-agent/comp/core/tagger/fx-noop"
	"github.com/DataDog/datadog-agent/comp/forwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/eventplatformimpl"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatformreceiver/eventplatformreceiverimpl"
	"github.com/DataDog/datadog-agent/comp/forwarder/orchestrator/orchestratorimpl"
	haagentfx "github.com/DataDog/datadog-agent/comp/haagent/fx"
	logscompression "github.com/DataDog/datadog-agent/comp/serializer/logscompression/fx"
	metricscompression "github.com/DataDog/datadog-agent/comp/serializer/metricscompression/fx"
	snmpscan "github.com/DataDog/datadog-agent/comp/snmpscan/def"
	snmpscanfx "github.com/DataDog/datadog-agent/comp/snmpscan/fx"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/metadata"
	"github.com/DataDog/datadog-agent/pkg/snmp/snmpparse"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

const (
	defaultTimeout                 = 10 // Timeout better suited to walking
	defaultRetries                 = 3
	defaultUseUnconnectedUDPSocket = false
)

// argsType is an alias so we can inject the args via fx.
type argsType []string

// configErr wraps any error caused by invalid configuration.
// If the main script returns a configErr it will print the usage string along
// with the error message.
type configErr struct {
	err error
}

func (u configErr) Error() string {
	if u.err != nil {
		return u.err.Error()
	}
	return "configuration error"
}

func (u configErr) Unwrap() error {
	return u.err
}

// confErrf is a shorthand for creating a simple confErr.
func confErrf(msg string, args ...any) configErr {
	return configErr{fmt.Errorf(msg, args...)}
}

// Commands returns a slice of subcommands for the 'agent' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	connParams := &snmpparse.SNMPConfig{}
	snmpCmd := &cobra.Command{
		Use:   "snmp",
		Short: "Snmp tools",
		Long:  ``,
	}

	snmpWalkCmd := &cobra.Command{
		Use:   "walk <IP Address>[:Port] [OID]",
		Short: "Perform an snmpwalk.",
		Long: `Walk the SNMP tree for a device, printing every OID found. If OID is specified, only show that OID and its children.
		Flags that aren't specified will be pulled from the agent SNMP config if possible.`,
		RunE: func(cmd *cobra.Command, args []string) error {

			err := fxutil.OneShot(snmpWalk,
				fx.Supply(connParams),
				fx.Provide(func() argsType { return args }),
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewAgentParams(globalParams.ConfFilePath, config.WithExtraConfFiles(globalParams.ExtraConfFilePath), config.WithFleetPoliciesDirPath(globalParams.FleetPoliciesDirPath)),
					SecretParams: secrets.NewEnabledParams(),
					LogParams:    log.ForOneShot(command.LoggerName, "off", true)}),
				core.Bundle(),
				snmpscanfx.Module(),
				demultiplexerimpl.Module(demultiplexerimpl.NewDefaultParams()),
				forwarder.Bundle(defaultforwarder.NewParams(defaultforwarder.WithFeatures(defaultforwarder.CoreFeatures))),
				orchestratorimpl.Module(orchestratorimpl.NewDefaultParams()),
				eventplatformimpl.Module(eventplatformimpl.NewDefaultParams()),
				nooptagger.Module(),
				eventplatformreceiverimpl.Module(),
				haagentfx.Module(),
				metricscompression.Module(),
				logscompression.Module(),
				ipcfx.ModuleReadOnly(),
			)
			if err != nil {
				var ue configErr
				if errors.As(err, &ue) {
					fmt.Println("Usage:", cmd.UseLine())
				}
				return err
			}
			return nil
		},
	}
	snmpWalkCmd.Flags().VarP(Flag(&snmpparse.VersionOpts, &connParams.Version), "snmp-version", "v",
		fmt.Sprintf("Specify SNMP version to use (%s)", snmpparse.VersionOpts.OptsStr()))

	// snmp v1 or v2c specific
	snmpWalkCmd.Flags().StringVarP(&connParams.CommunityString, "community-string", "C", "", "Set the community string")

	// snmp v3 specific
	snmpWalkCmd.Flags().VarP(Flag(&snmpparse.AuthOpts, &connParams.AuthProtocol), "auth-protocol", "a",
		fmt.Sprintf("Set authentication protocol (%s)", snmpparse.AuthOpts.OptsStr()))
	snmpWalkCmd.Flags().StringVarP(&connParams.AuthKey, "auth-key", "A", "", "Set authentication protocol pass phrase")
	snmpWalkCmd.Flags().VarP(Flag(&snmpparse.LevelOpts, &connParams.SecurityLevel), "security-level", "l",
		fmt.Sprintf("Set security level (%s)", snmpparse.LevelOpts.OptsStr()))
	snmpWalkCmd.Flags().StringVarP(&connParams.Context, "context", "N", "", "Set context name")
	snmpWalkCmd.Flags().StringVarP(&connParams.Username, "user-name", "u", "", "Set security name")
	snmpWalkCmd.Flags().VarP(Flag(&snmpparse.PrivOpts, &connParams.PrivProtocol), "priv-protocol", "x",
		fmt.Sprintf("Set privacy protocol (%s)", snmpparse.PrivOpts.OptsStr()))
	snmpWalkCmd.Flags().StringVarP(&connParams.PrivKey, "priv-key", "X", "", "Set privacy protocol pass phrase")

	// general communication options
	snmpWalkCmd.Flags().IntVarP(&connParams.Retries, "retries", "r", defaultRetries, "Set the number of retries")
	snmpWalkCmd.Flags().IntVarP(&connParams.Timeout, "timeout", "t", defaultTimeout, "Set the request timeout (in seconds)")
	snmpWalkCmd.Flags().BoolVar(&connParams.UseUnconnectedUDPSocket, "use-unconnected-udp-socket", defaultUseUnconnectedUDPSocket, "If specified, changes net connection to be unconnected UDP socket")

	snmpCmd.AddCommand(snmpWalkCmd)

	logLevelDefaultOff := command.LogLevelDefaultOff{}

	// This command does nothing until the backend supports it, so it isn't visible yet.
	snmpScanCmd := &cobra.Command{
		Hidden: true,
		Use:    "scan <ipaddress>[:port]",
		Short:  "Scan a device for the profile editor.",
		Long: `Walk the SNMP tree for a device, collecting available OIDs.
		Flags that aren't specified will be pulled from the agent SNMP config if possible.`,
		RunE: func(cmd *cobra.Command, args []string) error {

			err := fxutil.OneShot(scanDevice,
				fx.Supply(connParams, globalParams, cmd),
				fx.Provide(func() argsType { return args }),
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewAgentParams(globalParams.ConfFilePath, config.WithExtraConfFiles(globalParams.ExtraConfFilePath), config.WithFleetPoliciesDirPath(globalParams.FleetPoliciesDirPath)),
					SecretParams: secrets.NewEnabledParams(),
					LogParams:    log.ForOneShot(command.LoggerName, logLevelDefaultOff.Value(), true)}),
				core.Bundle(),
				aggregator.Bundle(demultiplexerimpl.NewDefaultParams()),
				orchestratorimpl.Module(orchestratorimpl.NewDefaultParams()),
				forwarder.Bundle(defaultforwarder.NewParams(defaultforwarder.WithFeatures(defaultforwarder.CoreFeatures))),
				eventplatformimpl.Module(eventplatformimpl.NewDefaultParams()),
				eventplatformreceiverimpl.Module(),
				nooptagger.Module(),
				snmpscanfx.Module(),
				haagentfx.Module(),
				metricscompression.Module(),
				logscompression.Module(),
				ipcfx.ModuleReadOnly(),
			)
			if err != nil {
				var ue configErr
				if errors.As(err, &ue) {
					fmt.Println("Usage:", cmd.UseLine())
				}
				return err
			}
			return nil
		},
	}

	logLevelDefaultOff.Register(snmpScanCmd)
	// TODO is there a way to merge these flags with snmpWalkCmd flags, without cobra changing the docs to mark them as "global flags"?
	snmpScanCmd.Flags().VarP(Flag(&snmpparse.VersionOpts, &connParams.Version), "snmp-version", "v",
		fmt.Sprintf("Specify SNMP version to use (%s)", snmpparse.VersionOpts.OptsStr()))

	// snmp v1 or v2c specific
	snmpScanCmd.Flags().StringVarP(&connParams.CommunityString, "community-string", "C", "", "Set the community string")

	// snmp v3 specific
	snmpScanCmd.Flags().VarP(Flag(&snmpparse.AuthOpts, &connParams.AuthProtocol), "auth-protocol", "a",
		fmt.Sprintf("Set authentication protocol (%s)", snmpparse.AuthOpts.OptsStr()))
	snmpScanCmd.Flags().StringVarP(&connParams.AuthKey, "auth-key", "A", "", "Set authentication protocol pass phrase")
	snmpScanCmd.Flags().VarP(Flag(&snmpparse.LevelOpts, &connParams.SecurityLevel), "security-level", "l",
		fmt.Sprintf("Set security level (%s)", snmpparse.LevelOpts.OptsStr()))
	snmpScanCmd.Flags().StringVarP(&connParams.Context, "context", "N", "", "Set context name")
	snmpScanCmd.Flags().StringVarP(&connParams.Username, "user-name", "u", "", "Set security name")
	snmpScanCmd.Flags().VarP(Flag(&snmpparse.PrivOpts, &connParams.PrivProtocol), "priv-protocol", "x",
		fmt.Sprintf("Set privacy protocol (%s)", snmpparse.PrivOpts.OptsStr()))
	snmpScanCmd.Flags().StringVarP(&connParams.PrivKey, "priv-key", "X", "", "Set privacy protocol pass phrase")

	// general communication options
	snmpScanCmd.Flags().IntVarP(&connParams.Retries, "retries", "r", defaultRetries, "Set the number of retries")
	snmpScanCmd.Flags().IntVarP(&connParams.Timeout, "timeout", "t", defaultTimeout, "Set the request timeout (in seconds)")
	snmpScanCmd.Flags().BoolVar(&connParams.UseUnconnectedUDPSocket, "use-unconnected-udp-socket", defaultUseUnconnectedUDPSocket, "If specified, changes net connection to be unconnected UDP socket")

	// This command does nothing until the backend supports it, so it isn't enabled yet.
	snmpCmd.AddCommand(snmpScanCmd)

	return []*cobra.Command{snmpCmd}
}

// maybeSplitIP splits an address into a host and port if possible.
// The return value is (host, port, ok) where ok will be true if and only if
// the parsing succeeded. If it fails, we assume that this address is only an
// IP address, and return (address, 0, false).
func maybeSplitIP(address string) (string, uint16, bool) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return address, 0, false
	}
	pnum, err := strconv.ParseUint(port, 0, 16)
	if err != nil {
		return address, 0, false
	}
	return host, uint16(pnum), true
}

func setDefaultsFromAgent(connParams *snmpparse.SNMPConfig, conf config.Component, client ipc.HTTPClient) error {
	agentParams, agentError := snmpparse.GetParamsFromAgent(connParams.IPAddress, conf, client)
	if agentError != nil {
		return agentError
	}
	if connParams.Version == "" {
		connParams.Version = agentParams.Version
	}
	if connParams.Port == 0 {
		connParams.Port = agentParams.Port
	}
	if connParams.CommunityString == "" {
		connParams.CommunityString = agentParams.CommunityString
	}
	if connParams.Username == "" {
		connParams.Username = agentParams.Username
	}
	if connParams.AuthProtocol == "" {
		connParams.AuthProtocol = agentParams.AuthProtocol
	}
	if connParams.AuthKey == "" {
		connParams.AuthKey = agentParams.AuthKey
	}
	if connParams.PrivProtocol == "" {
		connParams.PrivProtocol = agentParams.PrivProtocol
	}
	if connParams.PrivKey == "" {
		connParams.PrivKey = agentParams.PrivKey
	}
	if connParams.Context == "" {
		connParams.Context = agentParams.Context
	}
	if connParams.Retries == 0 {
		connParams.Retries = agentParams.Retries
	}
	if connParams.Timeout == 0 {
		connParams.Timeout = agentParams.Timeout
	}
	return nil
}

func scanDevice(connParams *snmpparse.SNMPConfig, args argsType, snmpScanner snmpscan.Component, conf config.Component, client ipc.HTTPClient) error {
	// Parse args
	if len(args) == 0 {
		return confErrf("missing argument: IP address")
	}
	deviceAddr := args[0]
	if len(args) > 1 {
		return confErrf("unexpected extra arguments; only one argument expected.")
	}
	// Parse port from IP address
	connParams.IPAddress, connParams.Port, _ = maybeSplitIP(deviceAddr)
	agentErr := setDefaultsFromAgent(connParams, conf, client)
	if agentErr != nil {
		// Warn that we couldn't contact the agent, but keep going in case the
		// user provided enough arguments to do this anyway.
		_, _ = fmt.Fprintf(os.Stderr, "Warning: %v\n", agentErr)
	}
	namespace := conf.GetString("network_devices.namespace")
	deviceID := namespace + ":" + connParams.IPAddress
	// Start the scan
	fmt.Printf("Launching scan for device: %s\n", deviceID)
	err := snmpScanner.ScanDeviceAndSendData(connParams, namespace, metadata.ManualScan)
	if err != nil {
		fmt.Printf("Unable to perform device scan for device %s : %e", deviceID, err)
	}
	fmt.Printf("Completed scan successfully for device: %s\n", deviceID)
	return err
}

// snmpWalk prints every SNMP value, in the style of the unix snmpwalk command.
func snmpWalk(connParams *snmpparse.SNMPConfig, args argsType, snmpScanner snmpscan.Component, conf config.Component, logger log.Component, client ipc.HTTPClient) error {
	// Parse args
	if len(args) == 0 {
		return confErrf("missing argument: IP address")
	}
	deviceAddr := args[0]
	oid := ""
	if len(args) > 1 {
		oid = args[1]
	}
	if len(args) > 2 {
		return confErrf("the number of arguments must be between 1 and 2. %d arguments were given.", len(args))
	}
	// Parse port from IP address
	connParams.IPAddress, connParams.Port, _ = maybeSplitIP(deviceAddr)
	agentErr := setDefaultsFromAgent(connParams, conf, client)
	if agentErr != nil {
		// Warn that we couldn't contact the agent, but keep going in case the
		// user provided enough arguments to do this anyway.
		_, _ = fmt.Fprintf(os.Stderr, "Warning: %v\n", agentErr)
	}
	// Establish connection
	snmp, err := snmpparse.NewSNMP(connParams, logger)
	if err != nil {
		// newSNMP only returns config errors, so any problem is a usage error
		return configErr{err}
	}
	if err := snmp.Connect(); err != nil {
		return fmt.Errorf("unable to connect to SNMP agent on %s:%d: %w", snmp.LocalAddr, snmp.Port, err)
	}
	defer func() { _ = snmp.Conn.Close() }()

	err = snmpScanner.RunSnmpWalk(snmp, oid)

	if err != nil {
		return fmt.Errorf("unable to walk SNMP agent on %s:%d: %w", connParams.IPAddress, connParams.Port, err)
	}

	return nil
}
