// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package snmp implements the 'agent snmp' subcommand.
package snmp

import (
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/gosnmp/gosnmp"
	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	utilFunc "github.com/DataDog/datadog-agent/pkg/snmp/gosnmplib"
	parse "github.com/DataDog/datadog-agent/pkg/snmp/snmpparse"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

const (
	defaultPort                    = 161
	defaultCommunityString         = "public"
	defaultTimeout                 = 10 // Timeout better suited to walking
	defaultRetries                 = 3
	defaultUseUnconnectedUDPSocket = false
)

// // connectionParams are the data needed to connect to an SNMP instance.
type connectionParams struct {
	// embed a SNMPConfig because it's all the same fields anyway
	parse.SNMPConfig
	// fields that aren't part of parse.SNMPConfig
	SecurityLevel           string
	UseUnconnectedUDPSocket bool
}

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
	connParams := &connectionParams{}
	snmpCmd := &cobra.Command{
		Use:   "snmp",
		Short: "Snmp tools",
		Long:  ``,
	}

	snmpWalkCmd := &cobra.Command{
		Use:   "walk <IP Address>[:Port] [OID]",
		Short: "Perform an snmpwalk, if only a valid IP address is provided with the oid then the agent default snmp config will be used",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {

			err := fxutil.OneShot(snmpwalk,
				fx.Supply(connParams, globalParams, cmd),
				fx.Provide(func() argsType { return args }),
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewAgentParams(globalParams.ConfFilePath),
					SecretParams: secrets.NewEnabledParams(),
					LogParams:    logimpl.ForOneShot(command.LoggerName, "off", true)}),
				core.Bundle(),
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
	snmpWalkCmd.Flags().StringVarP(&connParams.Version, "snmp-version", "v", "", "Specify SNMP version to use")

	// snmp v1 or v2c specific
	snmpWalkCmd.Flags().StringVarP(&connParams.CommunityString, "community-string", "C", "", "Set the community string")

	// snmp v3 specific
	snmpWalkCmd.Flags().StringVarP(&connParams.AuthProtocol, "auth-protocol", "a", "", "Set authentication protocol (MD5|SHA|SHA-224|SHA-256|SHA-384|SHA-512)")
	snmpWalkCmd.Flags().StringVarP(&connParams.AuthKey, "auth-key", "A", "", "Set authentication protocol pass phrase")
	snmpWalkCmd.Flags().StringVarP(&connParams.SecurityLevel, "security-level", "l", "", "set security level (noAuthNoPriv|authNoPriv|authPriv)")
	snmpWalkCmd.Flags().StringVarP(&connParams.Context, "context", "N", "", "Set context name")
	snmpWalkCmd.Flags().StringVarP(&connParams.Username, "user-name", "u", "", "Set security name")
	snmpWalkCmd.Flags().StringVarP(&connParams.PrivProtocol, "priv-protocol", "x", "", "Set privacy protocol (DES|AES|AES192|AES192C|AES256|AES256C)")
	snmpWalkCmd.Flags().StringVarP(&connParams.PrivKey, "priv-key", "X", "", "Set privacy protocol pass phrase")

	// general communication options
	snmpWalkCmd.Flags().IntVarP(&connParams.Retries, "retries", "r", defaultRetries, "Set the number of retries")
	snmpWalkCmd.Flags().IntVarP(&connParams.Timeout, "timeout", "t", defaultTimeout, "Set the request timeout (in seconds)")
	snmpWalkCmd.Flags().BoolVar(&connParams.UseUnconnectedUDPSocket, "use-unconnected-udp-socket", defaultUseUnconnectedUDPSocket, "If specified, changes net connection to be unconnected UDP socket")

	snmpCmd.AddCommand(snmpWalkCmd)

	return []*cobra.Command{snmpCmd}
}

// maybeSplitIP splits an address into a host and port if possible.
// The return value is (host, port, ok) where ok will be true if and only if
// the parsing succeeded. If it fails, we assume that this address is only an
// IP address. Note that ANY failure means we assume it's just an address
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

func getParamsFromAgent(deviceIP string, conf config.Component) (*parse.SNMPConfig, error) {
	snmpConfigList, err := parse.GetConfigCheckSnmp(conf)
	if err != nil {
		return nil, fmt.Errorf("unable to load SNMP config from agent: %w", err)
	}
	instance := parse.GetIPConfig(deviceIP, snmpConfigList)
	if instance.IPAddress != "" {
		instance.IPAddress = deviceIP
		return &instance, nil
	}
	return nil, fmt.Errorf("agent has no SNMP config for IP %s", deviceIP)
}

func setDefaultsFromAgent(connParams *connectionParams, conf config.Component) error {
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
func newSNMP(connParams *connectionParams) (*gosnmp.GoSNMP, error) {
	// Communication options check
	if connParams.Timeout == 0 {
		return nil, fmt.Errorf("timeout cannot be 0")
	}

	version := gosnmp.Version2c
	// Set the snmp version
	if connParams.Version == "1" {
		version = gosnmp.Version1
	} else if connParams.Version == "3" || (connParams.Version == "" && connParams.Username != "") {
		// Use version 3 if no version was specified but a username was.
		version = gosnmp.Version3
	} else if connParams.Version == "2c" || connParams.Version == "2" || connParams.Version == "" {
		// Default to 2c if nothing was specified.
		version = gosnmp.Version2c
	} else {
		return nil, fmt.Errorf("SNMP version not supported: %s", connParams.Version)
	}

	if version != gosnmp.Version3 && connParams.CommunityString == "" {
		// Set default community string if version 1 or 2c and no given community string
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

		// Authentication Protocol
		switch strings.ToLower(connParams.AuthProtocol) {
		case "":
			securityParams.AuthenticationProtocol = gosnmp.NoAuth
		case "md5":
			securityParams.AuthenticationProtocol = gosnmp.MD5
		case "sha":
			securityParams.AuthenticationProtocol = gosnmp.SHA
		case "sha224", "sha-224":
			securityParams.AuthenticationProtocol = gosnmp.SHA224
		case "sha256", "sha-256":
			securityParams.AuthenticationProtocol = gosnmp.SHA256
		case "sha384", "sha-384":
			securityParams.AuthenticationProtocol = gosnmp.SHA384
		case "sha512", "sha-512":
			securityParams.AuthenticationProtocol = gosnmp.SHA512
		default:
			return nil, fmt.Errorf("unsupported authentication protocol: %s", connParams.AuthProtocol)
		}

		// Privacy Protocol
		switch strings.ToLower(connParams.PrivProtocol) {
		case "":
			securityParams.PrivacyProtocol = gosnmp.NoPriv
		case "des":
			securityParams.PrivacyProtocol = gosnmp.DES
		case "aes":
			securityParams.PrivacyProtocol = gosnmp.AES
		case "aes192":
			securityParams.PrivacyProtocol = gosnmp.AES192
		case "aes192c":
			securityParams.PrivacyProtocol = gosnmp.AES192C
		case "aes256":
			securityParams.PrivacyProtocol = gosnmp.AES256
		case "aes256c":
			securityParams.PrivacyProtocol = gosnmp.AES256C
		default:
			return nil, fmt.Errorf("unsupported privacy protocol: %s", connParams.PrivProtocol)
		}

		// MsgFlags
		switch strings.ToLower(connParams.SecurityLevel) {
		case "":
			msgFlags = gosnmp.NoAuthNoPriv
			if connParams.PrivKey != "" {
				msgFlags = gosnmp.AuthPriv
			} else if connParams.AuthKey != "" {
				msgFlags = gosnmp.AuthNoPriv
			}

		case "noauthnopriv":
			msgFlags = gosnmp.NoAuthNoPriv
		case "authpriv":
			msgFlags = gosnmp.AuthPriv
		case "authnopriv":
			msgFlags = gosnmp.AuthNoPriv
		default:
			return nil, fmt.Errorf("unsupported security level: %s", connParams.SecurityLevel)
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
	}, nil
}

// snmpwalk prints every SNMP value, in the style of the unix snmpwalk command.
func snmpwalk(connParams *connectionParams, args argsType, conf config.Component) error {
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
		fmt.Printf("Warning: %v\n", agentErr)
	}
	// Establish connection
	snmp, err := newSNMP(connParams)
	if err != nil {
		// newSNMP only returns config errors, so any problem is a usage error
		return configErr{err}
	}
	if err := snmp.Connect(); err != nil {
		return fmt.Errorf("unable to connect to SNMP agent on %s:%d: %w", snmp.LocalAddr, snmp.Port, err)
	}
	defer snmp.Conn.Close()

	// Perform a snmpwalk using Walk for all versions
	if err := snmp.Walk(oid, printValue); err != nil {
		return fmt.Errorf("unable to walk SNMP agent on %s:%d: %w", snmp.Target, snmp.Port, err)
	}

	return nil
}

func printValue(pdu gosnmp.SnmpPDU) error {

	fmt.Printf("%s = ", pdu.Name)

	switch pdu.Type {
	case gosnmp.OctetString:
		b := pdu.Value.([]byte)
		if !utilFunc.IsStringPrintable(b) {
			var strBytes []string
			for _, bt := range b {
				strBytes = append(strBytes, strings.ToUpper(hex.EncodeToString([]byte{bt})))
			}
			fmt.Print("Hex-STRING: " + strings.Join(strBytes, " ") + "\n")
		} else {
			fmt.Printf("STRING: %s\n", string(b))
		}
	case gosnmp.ObjectIdentifier:
		fmt.Printf("OID: %s\n", pdu.Value)
	case gosnmp.TimeTicks:
		fmt.Print(pdu.Value, "\n")
	case gosnmp.Counter32:
		fmt.Printf("Counter 32: %d\n", pdu.Value.(uint))
	case gosnmp.Counter64:
		fmt.Printf("Counter 64: %d\n", pdu.Value.(uint64))
	case gosnmp.Integer:
		fmt.Printf("INTEGER: %d\n", pdu.Value.(int))
	case gosnmp.Gauge32:
		fmt.Printf("Gauge 32: %d\n", pdu.Value.(uint))
	case gosnmp.IPAddress:
		fmt.Printf("IpAddress: %s\n", pdu.Value.(string))
	default:
		fmt.Printf("TYPE %d: %d\n", pdu.Type, gosnmp.ToBigInt(pdu.Value))
	}

	return nil
}
