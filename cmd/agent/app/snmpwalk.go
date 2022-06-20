// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package app

import (
	"fmt"
	"os"
	"time"

	"log"

	"github.com/gosnmp/gosnmp"
	"github.com/spf13/cobra"
)

const (
	defaultVersion = "2c"
	defaultOID     = "1.3.6.1.2.1.1"
	defaultHost    = "127.0.0.1"
	defaultPort    = 1161
	// defaultAddress = "127.0.0.1:1161"

	// snmp v1 & v2c
	defaultCommunityString = "public"

	// TODO: snmp v3
	// defaultUserName        = ""
	// defaultAuthProtocol    = ""
	// defaultAuthKey         = ""
	// defaultPrivProtocol    = ""
	// defaultprivKey         = ""

	// general communication options
	defaultTimeout = 5
	defaultRetries = 3
)

var (
	// general
	snmpVersion string
	deviceIP    string
	oid         string
	port        uint16
	version_    gosnmp.SnmpVersion

	// v1 & v2c
	communityString string

	// TODO: v3
	// user         string
	// authProtocol gosnmp.SnmpV3AuthProtocol
	// authKey      string
	// privProtocol gosnmp.SnmpV3PrivProtocol
	// privKey      string

	// TODO: communication
	retries int
	timeout int

	results []gosnmp.SnmpPDU
	snmp    gosnmp.GoSNMP

	// TODO: debugging
	// pingDevice  bool
	// checkConfig bool

)

func init() {
	AgentCmd.AddCommand(snmpwalkCmd)
	snmpwalkCmd.Flags().StringVarP(&snmpVersion, "snmp-version", "v", defaultVersion, "Specify SNMP version to use")

	// TODO: remove from flags and pass as mandatory args to match the snmpwalk command
	snmpwalkCmd.Flags().StringVarP(&deviceIP, "ip-address", "i", defaultHost, "Set the host IP address")
	snmpwalkCmd.Flags().Uint16VarP(&port, "port", "p", defaultPort, "Set the port")
	snmpwalkCmd.Flags().StringVarP(&oid, "OID", "o", defaultOID, "Set the root OID")

	// snmp v1 or v2c specific
	snmpwalkCmd.Flags().StringVarP(&communityString, "community-string", "C", defaultCommunityString, "Set the community string for version 1 or 2c")

	// TODO: snmp v3 specific
	// snmpwalkCmd.Flags().StringVarP(&user, "user-name", "u", defaultUserName, "Set the user name for v3")
	// snmpwalkCmd.Flags().StringVarP(&authProtocol, "auth-protocol", "a", defaultAuthProtocol, "Set the authenticaton protocol for v3")
	// snmpwalkCmd.Flags().StringVarP(&authKey, "auth-key", "A", defaultAuthKey, "Set the authentication passphrase for v3")
	// snmpwalkCmd.Flags().StringVarP(&privProtocol, "priv-protocol", "x", defaultPrivProtocol, "Set the privacy protocol for v3")
	// snmpwalkCmd.Flags().StringVarP(&privKey, "priv-key", "X", defaultprivKey, "Set the privacy passphrase for v3")

	// general communication options
	snmpwalkCmd.Flags().IntVarP(&retries, "retries", "r", defaultRetries, "Set the number of retries")
	snmpwalkCmd.Flags().IntVarP(&timeout, "timeout", "t", defaultTimeout, "Set the request timeout (in seconds)")

	// TODO: debugging options
	// snmpwalkCmd.Flags().BoolVarP(&pingDevice, "ping", "P", false, "Ping the device before performing the snmpwalk") // connectivity check
	// snmpwalkCmd.Flags().BoolVarP(&checkConfig, "config", "", false, "Load device configuration") // config check

	snmpwalkCmd.SetArgs([]string{})
}

var snmpwalkCmd = &cobra.Command{
	Use:   "snmpwalk",
	Short: "Perform a snmpwalk",
	Long:  ``,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Set the default values
		if port == 0 {
			port = defaultPort
		}
		if timeout == 0 {
			timeout = defaultTimeout
		}
		if retries == 0 {
			retries = defaultRetries
		}

		// TODO: add authentication check
		// if communityString == "" && user == "" {
		// 	fmt.Printf("No authentication mechanism specified")
		// 	os.Exit(1)
		// }

		if snmpVersion == "1" {
			version_ = gosnmp.Version1
		} else if snmpVersion == "2c" || (snmpVersion == "" && communityString != "") {
			version_ = gosnmp.Version2c
			// TODO: v3
			// } else if snmpVersion == "3" || (snmpVersion == "" && user != "") {
			// 	version = gosnmp.Version3
		} else {
			fmt.Printf("SNMP version not supported: %s, using default version 2c.", snmpVersion)
			version_ = gosnmp.Version2c
		}

		// TODO: version 3 authentication
		// case "3":
		// 	snmp.Version = gosnmp.Version3
		// 	snmp.SecurityParameters = &gosnmp.UsmSecurityParameters{
		// 		UserName:                 user,
		// 		AuthenticationProtocol:   authProtocol,
		// 		AuthenticationPassphrase: AuthKey,
		// 		PrivacyProtocol:          privProtocol,
		// 		PrivacyPassphrase:        privKey}

		// Set SNMP parameters
		snmp = gosnmp.GoSNMP{
			Target:    deviceIP,
			Port:      port,
			Community: communityString,
			Transport: "udp",
			Version:   version_,
			Timeout:   time.Duration(10 * time.Second), // Timeout better suited to walking
			Retries:   retries,
			// SecurityParameters: &gosnmp.UsmSecurityParameters{
			// 	UserName:                 user,
			// 	AuthenticationProtocol:   authProtocol,
			// 	AuthenticationPassphrase: AuthKey,
			// 	PrivacyProtocol:          privProtocol,
			// 	PrivacyPassphrase:        privKey,
		}

		// Estbalish connection
		err := snmp.Connect()
		if err != nil {
			fmt.Printf("Connect err: %v\n", err)
			os.Exit(1)
		}
		defer snmp.Conn.Close()

		defer timeTrack(time.Now(), snmp.Version)

		// Perform a snmpwalk
		switch snmp.Version {
		case gosnmp.Version1:
			results, err = snmp.WalkAll(oid)
			if err != nil {
				fmt.Printf("Walk Error: %v\n", err)
				os.Exit(1)
			}
			for _, pdu := range results {
				printValue(pdu)
			}
		case gosnmp.Version2c, gosnmp.Version3:
			err = snmp.BulkWalk(oid, printValue)
			if err != nil {
				fmt.Printf("Walk Error: %v\n", err)
				os.Exit(1)
			}
			// default:
			// 	fmt.Printf("SNMP version not supported: %s", snmp.Version)
			// 	os.Exit(1)

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

func timeTrack(start time.Time, versionName gosnmp.SnmpVersion) {
	elapsed := time.Since(start)
	log.Printf("SnmpWalk for version %s took %s", versionName, elapsed)
}
