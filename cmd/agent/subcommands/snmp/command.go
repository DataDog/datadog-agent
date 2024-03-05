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
	defaultVersion = ""
	defaultOID     = ""
	defaultPort    = 161

	// snmp v1 & v2c
	defaultCommunityString = "public"

	// snmp v3
	defaultUserName      = ""
	defaultAuthProtocol  = ""
	defaultAuthKey       = ""
	defaultPrivProtocol  = ""
	defaultPrivKey       = ""
	defaultContext       = ""
	defaultSecurityLevel = ""

	// general communication options
	defaultTimeout                 = 10 // Timeout better suited to walking
	defaultRetries                 = 3
	defaultUseUnconnectedUDPSocket = false
)

// connectionParams are the data needed to connect to an SNMP instance.
type connectionParams struct {
	// general
	snmpVersion string

	// v1 & v2c
	communityString string

	// v3
	user          string
	authProt      string
	authKey       string
	privProt      string
	privKey       string
	snmpContext   string
	securityLevel string

	// communication
	retries              int
	timeout              int
	unconnectedUDPSocket bool
}

// argsType is an alias so we can inject the args via fx.
type argsType []string

// usageError wraps any error that indicates the user has provided invalid inputs.
// If the main script returns a usageError it will print the usage string along
// with the error message.
type usageError struct {
	e error
}

func (u usageError) Error() string {
	if u.e != nil {
		return u.e.Error()
	}
	return "Usage error"
}

func (u usageError) Unwrap() error {
	return u.e
}

// usagef is a shorthand for creating a simple usageError.
func usagef(msg string, args ...any) usageError {
	return usageError{fmt.Errorf(msg, args...)}
}

// getParams tries to parse deviceAddr as host:port; if it can, it just returns
// (host, port, params, nil). If the address is not a host:port pair, then it
// assumes it is a plain IP address, and queries the running agent for the
// configuration for that address.
func getParams(deviceAddr string, connParams *connectionParams) (string, uint16, *connectionParams, error) {
	deviceIP, port, hasPort := parseIPWithMaybePort(deviceAddr)

	if !hasPort {
		fmt.Println("No port provided. Loading details from agent config.")
		// If the customer provides only the ip_address then we fetch parameters from the agent runtime.
		// Allow the possibility to pass the config file as an argument to the command
		snmpConfigList, err := parse.GetConfigCheckSnmp()
		instance := parse.GetIPConfig(deviceIP, snmpConfigList)
		if err != nil {
			return "", 0, nil, fmt.Errorf("unable to load SNMP config from agent: %w", err)
		}
		if instance.IPAddress != "" {
			connParams.snmpVersion = instance.Version
			port = instance.Port

			// v1 & v2c
			connParams.communityString = instance.CommunityString

			// v3
			connParams.user = instance.Username
			connParams.authProt = instance.AuthProtocol
			connParams.authKey = instance.AuthKey
			connParams.privProt = instance.PrivProtocol
			connParams.privKey = instance.PrivKey
			connParams.snmpContext = instance.Context

			// communication
			connParams.retries = instance.Retries
			if instance.Timeout != 0 {
				connParams.timeout = instance.Timeout
			}
		}
	}

	if port == 0 {
		port = defaultPort
	}
	return deviceIP, port, connParams, nil
}

// newSNMP validates connection parameters and builds a GoSNMP from them.
func newSNMP(deviceIP string, port uint16, connParams *connectionParams) (*gosnmp.GoSNMP, error) {
	// Communication options check
	if connParams.timeout == 0 {
		return nil, fmt.Errorf("timeout cannot be 0")
	}

	// Authentication check
	if connParams.communityString == "" && connParams.user == "" {
		// Set default community string if version 1 or 2c and no given community string
		if connParams.snmpVersion == "1" || connParams.snmpVersion == "2c" {
			connParams.communityString = defaultCommunityString
		} else {
			return nil, fmt.Errorf("no authentication mechanism specified")
		}
	}

	version := gosnmp.Version2c
	// Set the snmp version
	if connParams.snmpVersion == "1" {
		version = gosnmp.Version1
	} else if connParams.snmpVersion == "2c" || (connParams.snmpVersion == "" && connParams.communityString != "") {
		version = gosnmp.Version2c
	} else if connParams.snmpVersion == "3" || (connParams.snmpVersion == "" && connParams.user != "") {
		version = gosnmp.Version3
	} else {
		fmt.Printf("SNMP version not supported: %s, using default version 2c. \n", connParams.snmpVersion) // match default version of the core check
	}
	securityParams := &gosnmp.UsmSecurityParameters{}
	var msgFlags gosnmp.SnmpV3MsgFlags
	// Set v3 security parameters
	if version == gosnmp.Version3 {
		securityParams.UserName = connParams.user
		securityParams.AuthenticationPassphrase = connParams.authKey
		securityParams.PrivacyPassphrase = connParams.privKey

		// Authentication Protocol
		switch strings.ToLower(connParams.authProt) {
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
			return nil, fmt.Errorf("unsupported authentication protocol: %s", connParams.authProt)
		}

		// Privacy Protocol
		switch strings.ToLower(connParams.privProt) {
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
			return nil, fmt.Errorf("unsupported privacy protocol: %s", connParams.privProt)
		}

		// MsgFlags
		switch strings.ToLower(connParams.securityLevel) {
		case "":
			msgFlags = gosnmp.NoAuthNoPriv
			if connParams.privKey != "" {
				msgFlags = gosnmp.AuthPriv
			} else if connParams.authKey != "" {
				msgFlags = gosnmp.AuthNoPriv
			}

		case "noauthnopriv":
			msgFlags = gosnmp.NoAuthNoPriv
		case "authpriv":
			msgFlags = gosnmp.AuthPriv
		case "authnopriv":
			msgFlags = gosnmp.AuthNoPriv
		default:
			return nil, fmt.Errorf("unsupported security level: %s", connParams.securityLevel)
		}
	}
	// Set SNMP parameters
	return &gosnmp.GoSNMP{
		Target:                  deviceIP,
		Port:                    port,
		Community:               connParams.communityString,
		Transport:               "udp",
		Version:                 version,
		Timeout:                 time.Duration(connParams.timeout * int(time.Second)),
		Retries:                 connParams.retries,
		SecurityModel:           gosnmp.UserSecurityModel,
		ContextName:             connParams.snmpContext,
		MsgFlags:                msgFlags,
		SecurityParameters:      securityParams,
		UseUnconnectedUDPSocket: connParams.unconnectedUDPSocket,
	}, nil
}

// Commands returns a slice of subcommands for the 'agent' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	connParams := &connectionParams{}
	snmpCmd := &cobra.Command{
		Use:   "snmp",
		Short: "Snmp tools",
		Long:  ``,
	}

	snmpCmd.PersistentFlags().StringVarP(&connParams.snmpVersion, "snmp-version", "v", defaultVersion, "Specify SNMP version to use")

	// snmp v1 or v2c specific
	snmpCmd.PersistentFlags().StringVarP(&connParams.communityString, "community-string", "C", "", "Set the community string")

	// snmp v3 specific
	snmpCmd.PersistentFlags().StringVarP(&connParams.authProt, "auth-protocol", "a", defaultAuthProtocol, "Set authentication protocol (MD5|SHA|SHA-224|SHA-256|SHA-384|SHA-512)")
	snmpCmd.PersistentFlags().StringVarP(&connParams.authKey, "auth-key", "A", defaultAuthKey, "Set authentication protocol pass phrase")
	snmpCmd.PersistentFlags().StringVarP(&connParams.securityLevel, "security-level", "l", defaultSecurityLevel, "set security level (noAuthNoPriv|authNoPriv|authPriv)")
	snmpCmd.PersistentFlags().StringVarP(&connParams.snmpContext, "context", "N", defaultContext, "Set context name")
	snmpCmd.PersistentFlags().StringVarP(&connParams.user, "user-name", "u", defaultUserName, "Set security name")
	snmpCmd.PersistentFlags().StringVarP(&connParams.privProt, "priv-protocol", "x", defaultPrivProtocol, "Set privacy protocol (DES|AES|AES192|AES192C|AES256|AES256C)")
	snmpCmd.PersistentFlags().StringVarP(&connParams.privKey, "priv-key", "X", defaultPrivKey, "Set privacy protocol pass phrase")

	// general communication options
	snmpCmd.PersistentFlags().IntVarP(&connParams.retries, "retries", "r", defaultRetries, "Set the number of retries")
	snmpCmd.PersistentFlags().IntVarP(&connParams.timeout, "timeout", "t", defaultTimeout, "Set the request timeout (in seconds)")
	snmpCmd.PersistentFlags().BoolVar(&connParams.unconnectedUDPSocket, "use-unconnected-udp-socket", defaultUseUnconnectedUDPSocket, "If specified, changes net connection to be unconnected UDP socket")

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
				var ue usageError
				if errors.As(err, &ue) {
					fmt.Println("Usage:", cmd.UseLine())
				}
				return err
			}
			return nil
		},
	}
	snmpCmd.AddCommand(snmpWalkCmd)

	return []*cobra.Command{snmpCmd}
}

// parseIPWithMaybePort splits an address into a host and port if possible.
// The return value is (host, port, ok) where ok will be true if
// and only if the parsing succeeded. If it fails, we assume that
// this address is only an IP address.
func parseIPWithMaybePort(address string) (string, uint16, bool) {
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

// snmpwalk walks every SNMP value and prints it, in the style of the unix snmpwalk command.
func snmpwalk(connParams *connectionParams, args argsType) error {
	// Parse args
	if len(args) == 0 {
		return usagef("missing argument: IP address")
	}
	deviceAddr := args[0]
	oid := defaultOID
	if len(args) > 1 {
		oid = args[1]
	}
	if len(args) > 2 {
		return usagef("the number of arguments must be between 1 and 2. %d arguments were given.", len(args))
	}
	// Load config if needed
	deviceIP, port, connParams, err := getParams(deviceAddr, connParams)
	if err != nil {
		return err
	}
	// Establish connection
	snmp, err := newSNMP(deviceIP, port, connParams)
	if err != nil {
		// newSNMP only returns parsing errors, so any problem is a usage error
		return usageError{err}
	}
	if err := snmp.Connect(); err != nil {
		return fmt.Errorf("unable to connect to SNMP agent on %s:%d: %w", deviceIP, port, err)
	}
	defer snmp.Conn.Close()

	// Perform a snmpwalk using Walk for all versions
	err = snmp.Walk(oid, printValue)
	if err != nil {
		return fmt.Errorf("walk error: %w", err)
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
