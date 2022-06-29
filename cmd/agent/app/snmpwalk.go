// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package app

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gosnmp/gosnmp"
	"github.com/spf13/cobra"
)

const (
	defaultVersion = "" // match default version of the core check
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
	defaultContext       = "public"
	defaultSecurityLevel = ""

	// general communication options
	defaultTimeout = 10 // Timeout better suited to walking
	defaultRetries = 3
)

var (
	// general
	snmpVersion string
	address     string
	deviceIP    string
	oid         string
	port        uint16
	value       uint64
	setVersion  gosnmp.SnmpVersion
	snmp        gosnmp.GoSNMP

	// v1 & v2c
	communityString string

	// v3
	user          string
	authProtocol  gosnmp.SnmpV3AuthProtocol
	authProt      string
	authKey       string
	privProtocol  gosnmp.SnmpV3PrivProtocol
	privProt      string
	privKey       string
	snmpContext   string
	msgFlags      gosnmp.SnmpV3MsgFlags
	securityLevel string

	// communication
	retries int
	timeout int
)

func init() {
	AgentCmd.AddCommand(snmpwalkCmd)
	snmpwalkCmd.Flags().StringVarP(&snmpVersion, "snmp-version", "v", defaultVersion, "Specify SNMP version to use")

	// snmp v1 or v2c specific
	snmpwalkCmd.Flags().StringVarP(&communityString, "community-string", "C", "", "Set the community string")

	// snmp v3 specific
	snmpwalkCmd.Flags().StringVarP(&authProt, "auth-protocol", "a", defaultAuthProtocol, "Set authentication protocol (MD5|SHA|SHA-224|SHA-256|SHA-384|SHA-512)")
	snmpwalkCmd.Flags().StringVarP(&authKey, "auth-key", "A", defaultAuthKey, "Set authentication protocol pass phrase")
	snmpwalkCmd.Flags().StringVarP(&securityLevel, "security-level", "l", defaultSecurityLevel, "set security level (noAuthNoPriv|authNoPriv|authPriv)")
	snmpwalkCmd.Flags().StringVarP(&snmpContext, "context", "N", defaultContext, "Set context name")
	snmpwalkCmd.Flags().StringVarP(&user, "user-name", "u", defaultUserName, "Set security name")
	snmpwalkCmd.Flags().StringVarP(&privProt, "priv-protocol", "x", defaultPrivProtocol, "Set privacy protocol (DES|AES|AES192|AES192C|AES256|AES256C)")
	snmpwalkCmd.Flags().StringVarP(&privKey, "priv-key", "X", defaultPrivKey, "Set privacy protocol pass phrase")

	// general communication options
	snmpwalkCmd.Flags().IntVarP(&retries, "retries", "r", defaultRetries, "Set the number of retries")
	snmpwalkCmd.Flags().IntVarP(&timeout, "timeout", "t", defaultTimeout, "Set the request timeout (in seconds)")

	snmpwalkCmd.SetArgs([]string{})
}

var snmpwalkCmd = &cobra.Command{
	Use:   "snmpwalk <IP Address> <OID> [OPTIONS]",
	Short: "Perform a snmpwalk",
	Long:  ``,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Get args
		if len(args) == 0 {
			fmt.Print("Missing argument: IP address\n")
			cmd.Help()
			os.Exit(1)
		} else if len(args) == 1 {
			address = args[0]
			oid = defaultOID
		} else if len(args) == 2 {
			address = args[0]
			oid = args[1]
		} else {
			fmt.Printf("The number of arguments must be between 1 and 2. %d arguments were given.\n", len(args))
			cmd.Help()
			os.Exit(1)
		}
		if strings.Contains(address, ":") {
			deviceIP = address[:strings.Index(address, ":")]
			value, _ = strconv.ParseUint(address[strings.Index(address, ":")+1:], 0, 16)
			port = uint16(value)
			if port == 0 {
				port = defaultPort
			}
		} else {
			deviceIP = address
			port = defaultPort
		}

		// Communication options check
		if timeout == 0 {
			fmt.Printf("Timeout can not be 0 \n")
			cmd.Help()
			os.Exit(1)
		}
		if retries == 0 {
			fmt.Printf("The number of retries can not be 0 \n")
			cmd.Help()
			os.Exit(1)
		}

		// Authentication check
		if communityString == "" && user == "" {
			// Set default community string if version 1 or 2c and no given community string
			if snmpVersion == "1" || snmpVersion == "2c" {
				communityString = defaultCommunityString
			} else {
				fmt.Printf("No authentication mechanism specified \n")
				cmd.Help()
				os.Exit(1)
			}
		}

		// Set the snmp version
		if snmpVersion == "1" {
			setVersion = gosnmp.Version1
		} else if snmpVersion == "2c" || (snmpVersion == "" && communityString != "") {
			setVersion = gosnmp.Version2c
		} else if snmpVersion == "3" || (snmpVersion == "" && user != "") {
			setVersion = gosnmp.Version3
		} else {
			fmt.Printf("SNMP version not supported: %s, using default version 2c. \n", snmpVersion)
			setVersion = gosnmp.Version2c
		}

		// Set v3 security parameters
		if setVersion == gosnmp.Version3 {
			// Authentication Protocol
			switch strings.ToLower(authProt) {
			case "":
				authProtocol = gosnmp.NoAuth
			case "md5":
				authProtocol = gosnmp.MD5
			case "sha":
				authProtocol = gosnmp.SHA
			case "sha224":
				authProtocol = gosnmp.SHA224
			case "sha256":
				authProtocol = gosnmp.SHA256
			case "sha384":
				authProtocol = gosnmp.SHA384
			case "sha512":
				authProtocol = gosnmp.SHA512
			default:
				fmt.Printf("Unsupported authentication protocol: %s \n", authProt)
				cmd.Help()
				os.Exit(1)
			}

			// Privacy Protocol
			switch strings.ToLower(privProt) {
			case "":
				privProtocol = gosnmp.NoPriv
			case "des":
				privProtocol = gosnmp.DES
			case "aes":
				privProtocol = gosnmp.AES
			case "aes192":
				privProtocol = gosnmp.AES192
			case "aes192c":
				privProtocol = gosnmp.AES192C
			case "aes256":
				privProtocol = gosnmp.AES256
			case "aes256c":
				privProtocol = gosnmp.AES256C
			default:
				fmt.Printf("Unsupported privacy protocol: %s \n", privProt)
				cmd.Help()
				os.Exit(1)
			}

			// MsgFlags
			switch strings.ToLower(securityLevel) {
			case "":
				msgFlags = gosnmp.NoAuthNoPriv
				if privKey != "" {
					msgFlags = gosnmp.AuthPriv
				} else if authKey != "" {
					msgFlags = gosnmp.AuthNoPriv
				}

			case "noauthnopriv":
				msgFlags = gosnmp.NoAuthNoPriv
			case "authpriv":
				msgFlags = gosnmp.AuthPriv
			case "authnopriv":
				msgFlags = gosnmp.AuthNoPriv
			default:
				fmt.Printf("Unsupported security level: %s \n", securityLevel)
				cmd.Help()
				os.Exit(1)
			}
		}
		// Set SNMP parameters
		snmp = gosnmp.GoSNMP{
			Target:    deviceIP,
			Port:      port,
			Community: communityString,
			Transport: "udp",
			Version:   setVersion,
			Timeout:   time.Duration(timeout * int(time.Second)),
			Retries:   retries,
			// v3
			SecurityModel: gosnmp.UserSecurityModel,
			ContextName:   snmpContext,
			MsgFlags:      msgFlags,
			SecurityParameters: &gosnmp.UsmSecurityParameters{
				UserName:                 user,
				AuthenticationProtocol:   authProtocol,
				AuthenticationPassphrase: authKey,
				PrivacyProtocol:          privProtocol,
				PrivacyPassphrase:        privKey,
			},
		}
		// Establish connection
		err := snmp.Connect()
		if err != nil {
			fmt.Printf("Connect err: %v\n", err)
			os.Exit(1)
		}
		defer snmp.Conn.Close()

		// Perform a snmpwalk using Walk for all versions
		err = snmp.Walk(oid, printValue)
		if err != nil {
			fmt.Printf("Walk Error: %v\n", err)
			os.Exit(1)
		}

		return nil
	},
}

func printValue(pdu gosnmp.SnmpPDU) error {

	fmt.Printf("%s = ", pdu.Name)

	switch pdu.Type {
	case gosnmp.OctetString:
		b := pdu.Value.([]byte)
		fmt.Printf("STRING: %s\n", string(b))
	case gosnmp.ObjectIdentifier:
		fmt.Printf("OID: %s\n", pdu.Value)
	case gosnmp.TimeTicks:
		fmt.Print(pdu.Value, "\n")
	default:
		fmt.Printf("TYPE %d: %d\n", pdu.Type, gosnmp.ToBigInt(pdu.Value))
	}

	return nil
}
