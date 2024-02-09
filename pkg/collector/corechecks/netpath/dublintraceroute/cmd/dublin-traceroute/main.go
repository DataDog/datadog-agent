/* SPDX-License-Identifier: BSD-2-Clause */

package main

import (
	"errors"
	"fmt"
	"go/build"
	"log"
	"net"
	"os"
	"runtime"
	"time"

	flag "github.com/spf13/pflag"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/netpath/dublintraceroute"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/netpath/dublintraceroute/probes/probev4"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/netpath/dublintraceroute/probes/probev6"
)

// Program constants and default values
const (
	ProgramName         = "Dublin Traceroute"
	ProgramVersion      = "v0.2"
	ProgramAuthorName   = "Andrea Barberio"
	ProgramAuthorInfo   = "https://insomniac.slackware.it"
	DefaultSourcePort   = 12345
	DefaultDestPort     = 33434
	DefaultNumPaths     = 10
	DefaultMinTTL       = 1
	DefaultMaxTTL       = 30
	DefaultDelay        = 50 //msec
	DefaultReadTimeout  = 3 * time.Second
	DefaultOutputFormat = "json"
)

// used to hold flags
type args struct {
	version      bool
	target       string
	sport        int
	useSrcport   bool
	dport        int
	npaths       int
	minTTL       int
	maxTTL       int
	delay        int
	brokenNAT    bool
	outputFile   string
	outputFormat string
	v4           bool
}

// Args will hold the program arguments
var Args args

// resolve returns the first IP address for the given host. If `wantV6` is true,
// it will return the first IPv6 address, or nil if none. Similarly for IPv4
// when `wantV6` is false.
// If the host is already an IP address, such IP address will be returned. If
// `wantV6` is true but no IPv6 address is found, it will return an error.
// Similarly for IPv4 when `wantV6` is false.
func resolve(host string, wantV6 bool) (net.IP, error) {
	if ip := net.ParseIP(host); ip != nil {
		if wantV6 && ip.To4() != nil {
			return nil, errors.New("Wanted an IPv6 address but got an IPv4 address")
		} else if !wantV6 && ip.To4() == nil {
			return nil, errors.New("Wanted an IPv4 address but got an IPv6 address")
		}
		return ip, nil
	}
	ipaddrs, err := net.LookupIP(host)
	if err != nil {
		return nil, err
	}
	var ret net.IP
	for _, ipaddr := range ipaddrs {
		if wantV6 && ipaddr.To4() == nil {
			ret = ipaddr
			break
		} else if !wantV6 && ipaddr.To4() != nil {
			ret = ipaddr
		}
	}
	if ret == nil {
		return nil, errors.New("No IP address of the requested type was found")
	}
	return ret, nil
}

func init() {
	// Ensure that CGO is disabled
	var ctx build.Context
	if ctx.CgoEnabled {
		fmt.Println("Disabling CGo")
		ctx.CgoEnabled = false
	}

	// handle flags
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Dublin Traceroute (Go implementation) %s\n", ProgramVersion)
		fmt.Fprintf(os.Stderr, "Written by %s - %s\n", ProgramAuthorName, ProgramAuthorInfo)
		fmt.Fprintf(os.Stderr, "\n")
		flag.PrintDefaults()
	}
	// Args holds the program's arguments as parsed by `flag`
	flag.BoolVarP(&Args.version, "version", "v", false, "print the version of Dublin Traceroute")
	flag.IntVarP(&Args.sport, "sport", "s", DefaultSourcePort, "the source port to send packets from")
	flag.IntVarP(&Args.dport, "dport", "d", DefaultDestPort, "the base destination port to send packets to")
	flag.IntVarP(&Args.npaths, "npaths", "n", DefaultNumPaths, "the number of paths to probe")
	flag.IntVarP(&Args.minTTL, "min-ttl", "t", DefaultMinTTL, "the minimum TTL to probe")
	flag.IntVarP(&Args.maxTTL, "max-ttl", "T", DefaultMaxTTL, "the maximum TTL to probe")
	flag.IntVarP(&Args.delay, "delay", "D", DefaultDelay, "the inter-packet delay in milliseconds")
	flag.BoolVarP(&Args.brokenNAT, "broken-nat", "b", false, "the network has a broken NAT configuration (e.g. no payload fixup). Try this if you see fewer hops than expected")
	flag.BoolVarP(&Args.useSrcport, "use-srcport", "i", false, "generate paths using source port instead of destination port")
	flag.StringVarP(&Args.outputFile, "output-file", "o", "", "the output file name. If unspecified, or \"-\", print to stdout")
	flag.StringVarP(&Args.outputFormat, "output-format", "f", DefaultOutputFormat, "the output file format, either \"json\" or \"dot\"")
	flag.BoolVarP(&Args.v4, "force-ipv4", "4", false, "Force the use of the legacy IPv4 protocol")
	flag.CommandLine.SortFlags = false
}

func main() {
	SetColourPurple := "\x1b[0;35m"
	UnsetColour := "\x1b[0m"
	if os.Geteuid() == 0 {
		if runtime.GOOS == "linux" {
			fmt.Fprintf(os.Stderr, "%sWARNING: you are running this program as root. Consider setting the CAP_NET_RAW capability and running as non-root user as a more secure alternative%s\n", SetColourPurple, UnsetColour)
		}
	}

	flag.Parse()
	if Args.version {
		fmt.Printf("%v %v\n", ProgramName, ProgramVersion)
		os.Exit(0)
	}

	if len(flag.Args()) != 1 {
		log.Fatal("Exactly one target is required")
	}

	Args.target = flag.Arg(0)
	target, err := resolve(Args.target, !Args.v4)
	if err != nil {
		log.Fatalf("Cannot resolve %s: %v", flag.Arg(0), err)
	}
	fmt.Fprintf(os.Stderr, "Traceroute configuration:\n")
	fmt.Fprintf(os.Stderr, "Target                : %v\n", target)
	fmt.Fprintf(os.Stderr, "Base source port      : %v\n", Args.sport)
	fmt.Fprintf(os.Stderr, "Base destination port : %v\n", Args.dport)
	fmt.Fprintf(os.Stderr, "Use srcport for paths : %v\n", Args.useSrcport)
	fmt.Fprintf(os.Stderr, "Number of paths       : %v\n", Args.npaths)
	fmt.Fprintf(os.Stderr, "Minimum TTL           : %v\n", Args.minTTL)
	fmt.Fprintf(os.Stderr, "Maximum TTL           : %v\n", Args.maxTTL)
	fmt.Fprintf(os.Stderr, "Inter-packet delay    : %v\n", Args.delay)
	fmt.Fprintf(os.Stderr, "Timeout               : %v\n", time.Duration(Args.delay)*time.Millisecond)
	fmt.Fprintf(os.Stderr, "Treat as broken NAT   : %v\n", Args.brokenNAT)

	var dt dublintraceroute.DublinTraceroute
	if Args.v4 {
		dt = &probev4.UDPv4{
			Target:     target,
			SrcPort:    uint16(Args.sport),
			DstPort:    uint16(Args.dport),
			UseSrcPort: Args.useSrcport,
			NumPaths:   uint16(Args.npaths),
			MinTTL:     uint8(Args.minTTL),
			MaxTTL:     uint8(Args.maxTTL),
			Delay:      time.Duration(Args.delay) * time.Millisecond,
			Timeout:    DefaultReadTimeout,
			BrokenNAT:  Args.brokenNAT,
		}
	} else {
		dt = &probev6.UDPv6{
			Target:      target,
			SrcPort:     uint16(Args.sport),
			DstPort:     uint16(Args.dport),
			UseSrcPort:  Args.useSrcport,
			NumPaths:    uint16(Args.npaths),
			MinHopLimit: uint8(Args.minTTL),
			MaxHopLimit: uint8(Args.maxTTL),
			Delay:       time.Duration(Args.delay) * time.Millisecond,
			Timeout:     DefaultReadTimeout,
			BrokenNAT:   Args.brokenNAT,
		}
	}
	results, err := dt.Traceroute()
	if err != nil {
		log.Fatalf("Traceroute() failed: %v", err)
	}
	var (
		output string
	)
	switch Args.outputFormat {
	case "json":
		output, err = results.ToJSON(true, "  ")
	case "dot":
		output, err = results.ToDOT()
	default:
		log.Fatalf("Unknown output format \"%s\"", Args.outputFormat)
	}
	if err != nil {
		log.Fatalf("Failed to generate output in format \"%s\": %v", Args.outputFormat, err)
	}
	if Args.outputFile == "-" || Args.outputFile == "" {
		fmt.Println(output)
	} else {
		err := os.WriteFile(Args.outputFile, []byte(output), 0644)
		if err != nil {
			log.Fatalf("Failed to write to file: %v", err)
		}
		log.Printf("Saved results to to \"%s\"", Args.outputFile)
		if Args.outputFormat == "json" {
			log.Printf("You can convert it to DOT by running `todot \"%s\"", Args.outputFile)
		}
		log.Printf("You can convert the DOT file to PNG by running `dot -Tpng \"%s\" -o \"%s.png\"`", Args.outputFile, Args.outputFile)
	}
}
