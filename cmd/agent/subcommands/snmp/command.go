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
	"time"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/comp/aggregator"
	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer/demultiplexerimpl"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	nooptagger "github.com/DataDog/datadog-agent/comp/core/tagger/noopimpl"
	"github.com/DataDog/datadog-agent/comp/forwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/eventplatformimpl"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatformreceiver/eventplatformreceiverimpl"
	"github.com/DataDog/datadog-agent/comp/forwarder/orchestrator/orchestratorimpl"
	"github.com/DataDog/datadog-agent/comp/serializer/compression/compressionimpl"
	snmpscan "github.com/DataDog/datadog-agent/comp/snmpscan/def"
	snmpscanfx "github.com/DataDog/datadog-agent/comp/snmpscan/fx"
	"github.com/DataDog/datadog-agent/comp/snmptraps/snmplog"
	"github.com/DataDog/datadog-agent/pkg/snmp/snmpparse"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"

	"github.com/gosnmp/gosnmp"
	"github.com/spf13/cobra"
	"go.uber.org/fx"
)

const (
	defaultPort                    = 161
	defaultCommunityString         = "public"
	defaultTimeout                 = 10 // Timeout better suited to walking
	defaultRetries                 = 3
	defaultUseUnconnectedUDPSocket = false
)

var authOpts = NewOptions(OptPairs[gosnmp.SnmpV3AuthProtocol]{
	{"", gosnmp.NoAuth},
	{"MD5", gosnmp.MD5},
	{"SHA", gosnmp.SHA},
	{"SHA-224", gosnmp.SHA224},
	{"SHA-256", gosnmp.SHA256},
	{"SHA-384", gosnmp.SHA384},
	{"SHA-512", gosnmp.SHA512},
})

var privOpts = NewOptions(OptPairs[gosnmp.SnmpV3PrivProtocol]{
	{"", gosnmp.NoPriv},
	{"DES", gosnmp.DES},
	{"AES", gosnmp.AES},
	{"AES192", gosnmp.AES192},
	{"AES192C", gosnmp.AES192C},
	{"AES256", gosnmp.AES256},
	{"AES256C", gosnmp.AES256C},
})

var versionOpts = NewOptions(OptPairs[gosnmp.SnmpVersion]{
	{"1", gosnmp.Version1},
	{"2c", gosnmp.Version2c},
	{"3", gosnmp.Version3},
})

var levelOpts = NewOptions(OptPairs[gosnmp.SnmpV3MsgFlags]{
	{"noAuthNoPriv", gosnmp.NoAuthNoPriv},
	{"authNoPriv", gosnmp.AuthNoPriv},
	{"authPriv", gosnmp.AuthPriv},
})

// argsType is an alias so we can inject the args via fx.
type argsType []string

type snmpConnectionParams struct {
	// embed a SNMPConfig because it's all the same fields anyway
	snmpparse.SNMPConfig
	// fields that aren't part of snmpparse.SNMPConfig
	SecurityLevel           string
	UseUnconnectedUDPSocket bool
}

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
	connParams := &snmpConnectionParams{}
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
					LogParams:    log.ForOneShot(command.LoggerName, "info", true)}),
				core.Bundle(),
				snmpscanfx.Module(),
				demultiplexerimpl.Module(demultiplexerimpl.NewDefaultParams()),
				forwarder.Bundle(defaultforwarder.NewParams(defaultforwarder.WithFeatures(defaultforwarder.CoreFeatures))),
				orchestratorimpl.Module(orchestratorimpl.NewDefaultParams()),
				eventplatformimpl.Module(eventplatformimpl.NewDefaultParams()),
				nooptagger.Module(),
				compressionimpl.Module(),
				eventplatformreceiverimpl.Module(),
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
	snmpWalkCmd.Flags().VarP(versionOpts.Flag(&connParams.Version), "snmp-version", "v", fmt.Sprintf("Specify SNMP version to use (%s)", versionOpts.OptsStr()))

	// snmp v1 or v2c specific
	snmpWalkCmd.Flags().StringVarP(&connParams.CommunityString, "community-string", "C", "", "Set the community string")

	// snmp v3 specific
	snmpWalkCmd.Flags().VarP(authOpts.Flag(&connParams.AuthProtocol), "auth-protocol", "a", fmt.Sprintf("Set authentication protocol (%s)", authOpts.OptsStr()))
	snmpWalkCmd.Flags().StringVarP(&connParams.AuthKey, "auth-key", "A", "", "Set authentication protocol pass phrase")
	snmpWalkCmd.Flags().VarP(levelOpts.Flag(&connParams.SecurityLevel), "security-level", "l", fmt.Sprintf("Set security level (%s)", levelOpts.OptsStr()))
	snmpWalkCmd.Flags().StringVarP(&connParams.Context, "context", "N", "", "Set context name")
	snmpWalkCmd.Flags().StringVarP(&connParams.Username, "user-name", "u", "", "Set security name")
	snmpWalkCmd.Flags().VarP(privOpts.Flag(&connParams.PrivProtocol), "priv-protocol", "x", fmt.Sprintf("Set privacy protocol (%s)", privOpts.OptsStr()))
	snmpWalkCmd.Flags().StringVarP(&connParams.PrivKey, "priv-key", "X", "", "Set privacy protocol pass phrase")

	// general communication options
	snmpWalkCmd.Flags().IntVarP(&connParams.Retries, "retries", "r", defaultRetries, "Set the number of retries")
	snmpWalkCmd.Flags().IntVarP(&connParams.Timeout, "timeout", "t", defaultTimeout, "Set the request timeout (in seconds)")
	snmpWalkCmd.Flags().BoolVar(&connParams.UseUnconnectedUDPSocket, "use-unconnected-udp-socket", defaultUseUnconnectedUDPSocket, "If specified, changes net connection to be unconnected UDP socket")

	snmpCmd.AddCommand(snmpWalkCmd)

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
					LogParams:    log.ForOneShot(command.LoggerName, "off", true)}),
				core.Bundle(),
				aggregator.Bundle(demultiplexerimpl.NewDefaultParams()),
				orchestratorimpl.Module(orchestratorimpl.NewDefaultParams()),
				forwarder.Bundle(defaultforwarder.NewParams(defaultforwarder.WithFeatures(defaultforwarder.CoreFeatures))),
				eventplatformimpl.Module(eventplatformimpl.NewDefaultParams()),
				eventplatformreceiverimpl.Module(),
				nooptagger.Module(),
				compressionimpl.Module(),
				snmpscanfx.Module(),
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
	// TODO is there a way to merge these flags with snmpWalkCmd flags, without cobra changing the docs to mark them as "global flags"?
	snmpScanCmd.Flags().VarP(versionOpts.Flag(&connParams.Version), "snmp-version", "v", fmt.Sprintf("Specify SNMP version to use (%s)", versionOpts.OptsStr()))

	// snmp v1 or v2c specific
	snmpScanCmd.Flags().StringVarP(&connParams.CommunityString, "community-string", "C", "", "Set the community string")

	// snmp v3 specific
	snmpScanCmd.Flags().VarP(authOpts.Flag(&connParams.AuthProtocol), "auth-protocol", "a", fmt.Sprintf("Set authentication protocol (%s)", authOpts.OptsStr()))
	snmpScanCmd.Flags().StringVarP(&connParams.AuthKey, "auth-key", "A", "", "Set authentication protocol pass phrase")
	snmpScanCmd.Flags().VarP(levelOpts.Flag(&connParams.SecurityLevel), "security-level", "l", fmt.Sprintf("Set security level (%s)", levelOpts.OptsStr()))
	snmpScanCmd.Flags().StringVarP(&connParams.Context, "context", "N", "", "Set context name")
	snmpScanCmd.Flags().StringVarP(&connParams.Username, "user-name", "u", "", "Set security name")
	snmpScanCmd.Flags().VarP(privOpts.Flag(&connParams.PrivProtocol), "priv-protocol", "x", fmt.Sprintf("Set privacy protocol (%s)", privOpts.OptsStr()))
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

func getParamsFromAgent(deviceIP string, conf config.Component) (*snmpparse.SNMPConfig, error) {
	snmpConfigList, err := snmpparse.GetConfigCheckSnmp(conf)
	if err != nil {
		return nil, fmt.Errorf("unable to load SNMP config from agent: %w", err)
	}
	instance := snmpparse.GetIPConfig(deviceIP, snmpConfigList)
	if instance.IPAddress != "" {
		instance.IPAddress = deviceIP
		return &instance, nil
	}
	return nil, fmt.Errorf("agent has no SNMP config for IP %s", deviceIP)
}

func setDefaultsFromAgent(connParams *snmpConnectionParams, conf config.Component) error {
	agentParams, agentError := getParamsFromAgent(connParams.IPAddress, conf)
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

// newSNMP validates connection parameters and builds a GoSNMP from them.
func newSNMP(connParams *snmpConnectionParams, logger log.Component) (*gosnmp.GoSNMP, error) {
	// Communication options check
	if connParams.Timeout == 0 {
		return nil, fmt.Errorf("timeout cannot be 0")
	}
	var version gosnmp.SnmpVersion
	var ok bool
	if connParams.Version == "" {
		// Assume v3 if a username was set, otherwise assume v2c.
		if connParams.Username != "" {
			version = gosnmp.Version3
		} else {
			version = gosnmp.Version2c
		}
	} else if version, ok = versionOpts.getVal(connParams.Version); !ok {
		return nil, fmt.Errorf("SNMP version %q not supported; must be %s", connParams.Version, versionOpts.OptsStr())
	}

	// Set default community string if version 1 or 2c and no given community string
	if version != gosnmp.Version3 && connParams.CommunityString == "" {
		connParams.CommunityString = defaultCommunityString
	}

	// Authentication check
	if version == gosnmp.Version3 && connParams.Username == "" {
		return nil, fmt.Errorf("username is required for snmp v3")
	}

	port := connParams.Port
	if port == 0 {
		port = defaultPort
	}

	securityParams := &gosnmp.UsmSecurityParameters{}
	var msgFlags gosnmp.SnmpV3MsgFlags
	// Set v3 security parameters
	if version == gosnmp.Version3 {
		securityParams.UserName = connParams.Username
		securityParams.AuthenticationPassphrase = connParams.AuthKey
		securityParams.PrivacyPassphrase = connParams.PrivKey

		if securityParams.AuthenticationProtocol, ok = authOpts.getVal(connParams.AuthProtocol); !ok {
			return nil, fmt.Errorf("authentication protocol %q not supported; must be %s", connParams.AuthProtocol, authOpts.OptsStr())
		}

		if securityParams.PrivacyProtocol, ok = privOpts.getVal(connParams.PrivProtocol); !ok {
			return nil, fmt.Errorf("privacy protocol %q not supported; must be %s", connParams.PrivProtocol, privOpts.OptsStr())
		}

		if connParams.SecurityLevel == "" {
			msgFlags = gosnmp.NoAuthNoPriv
			if connParams.PrivKey != "" {
				msgFlags = gosnmp.AuthPriv
			} else if connParams.AuthKey != "" {
				msgFlags = gosnmp.AuthNoPriv
			}
		} else {
			var ok bool // can't use := below because it'll make a new msgFlags instead of setting the one in the parent scope.
			if msgFlags, ok = levelOpts.getVal(connParams.SecurityLevel); !ok {
				return nil, fmt.Errorf("security level %q not supported; must be %s", connParams.SecurityLevel, levelOpts.OptsStr())
			}
		}
	}
	// Set SNMP parameters
	return &gosnmp.GoSNMP{
		Target:                  connParams.IPAddress,
		Port:                    port,
		Community:               connParams.CommunityString,
		Transport:               "udp",
		Version:                 version,
		Timeout:                 time.Duration(connParams.Timeout * int(time.Second)),
		Retries:                 connParams.Retries,
		SecurityModel:           gosnmp.UserSecurityModel,
		ContextName:             connParams.Context,
		MsgFlags:                msgFlags,
		SecurityParameters:      securityParams,
		UseUnconnectedUDPSocket: connParams.UseUnconnectedUDPSocket,
		Logger:                  gosnmp.NewLogger(snmplog.New(logger)),
	}, nil
}

func scanDevice(connParams *snmpConnectionParams, args argsType, snmpScanner snmpscan.Component, conf config.Component, logger log.Component) error {
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
	agentErr := setDefaultsFromAgent(connParams, conf)
	if agentErr != nil {
		// Warn that we couldn't contact the agent, but keep going in case the
		// user provided enough arguments to do this anyway.
		fmt.Fprintf(os.Stderr, "Warning: %v\n", agentErr)
	}
	// Establish connection
	snmp, err := newSNMP(connParams, logger)
	if err != nil {
		// newSNMP only returns config errors, so any problem is a usage error
		return configErr{err}
	}
	if err := snmp.Connect(); err != nil {
		return fmt.Errorf("unable to connect to SNMP agent on %s:%d: %w", snmp.LocalAddr, snmp.Port, err)
	}

	namespace := conf.GetString("network_devices.namespace")

	err = snmpScanner.RunDeviceScan(snmp, namespace, connParams.IPAddress)
	if err != nil {
		return fmt.Errorf("unable to perform device scan: %v", err)
	}
	return nil
}

// snmpWalk prints every SNMP value, in the style of the unix snmpwalk command.
func snmpWalk(connParams *snmpConnectionParams, args argsType, snmpScanner snmpscan.Component, conf config.Component, logger log.Component) error {
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
	agentErr := setDefaultsFromAgent(connParams, conf)
	if agentErr != nil {
		// Warn that we couldn't contact the agent, but keep going in case the
		// user provided enough arguments to do this anyway.
		fmt.Fprintf(os.Stderr, "Warning: %v\n", agentErr)
	}
	// Establish connection
	snmp, err := newSNMP(connParams, logger)
	if err != nil {
		// newSNMP only returns config errors, so any problem is a usage error
		return configErr{err}
	}
	if err := snmp.Connect(); err != nil {
		return fmt.Errorf("unable to connect to SNMP agent on %s:%d: %w", snmp.LocalAddr, snmp.Port, err)
	}
	defer snmp.Conn.Close()

	err = snmpScanner.RunSnmpWalk(snmp, oid)

	if err != nil {
		return fmt.Errorf("unable to walk SNMP agent on %s:%d: %w", connParams.IPAddress, connParams.Port, err)
	}

	return nil
}
