// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package obfuscate

import (
	"fmt"
	"net"
	"net/netip"
	"regexp"
	"strings"
)

// QuantizePeerIPAddresses quantizes a comma separated list of hosts. Each entry which is an IP address is replaced using quantizeIP.
// Duplicate entries post-quantization or collapsed into a single unique value.
// Entries which are not IP addresses are left unchanged.
// Comma-separated host lists are common for peer tags like peer.cassandra.contact.points, peer.couchbase.seed.nodes, peer.kafka.bootstrap.servers
func QuantizePeerIPAddresses(raw string) string {
	values := strings.Split(raw, ",")
	uniq := values[:0]
	uniqSet := make(map[string]bool)
	for _, v := range values {
		q := quantizeIP(v)
		if !uniqSet[q] {
			uniqSet[q] = true
			uniq = append(uniq, q)
		}
	}
	return strings.Join(uniq, ",")
}

var schemes = []string{"dnspoll", "ftp", "file", "http", "https"}
var protocolRegex = regexp.MustCompile(fmt.Sprintf(`((?:%s):/{2,3}).*`, strings.Join(schemes, "|")))

var allowedIPAddresses = map[string]bool{
	// localhost
	"127.0.0.1": true,
	"::1":       true,
	// link-local cloud provider metadata server addresses
	"169.254.169.254": true,
	"fd00:ec2::254":   true,
	// ECS task metadata
	"169.254.170.2": true,
}

func splitPrefix(raw string) (prefix, after string) {
	if after, ok := strings.CutPrefix(raw, "ip-"); ok { // AWS EC2 hostnames e.g. ip-10-123-4-567.ec2.internal
		return "ip-", after
	}

	isHintFound := false
	for _, hint := range schemes {
		if strings.Contains(raw, hint) {
			isHintFound = true
			break
		}
	}

	if isHintFound {
		subMatches := protocolRegex.FindStringSubmatch(raw)
		if len(subMatches) >= 2 {
			prefix = subMatches[1]
		}
	}

	return prefix, raw[len(prefix):]
}

// quantizeIP quantizes the ip address in the provided string, only if it exactly matches an ip with an optional port
// if the string is not an ip then empty string is returned
func quantizeIP(raw string) string {
	prefix, rawNoPrefix := splitPrefix(raw)
	host, port, suffix := parseIPAndPort(rawNoPrefix)
	if host == "" {
		// not an ip address
		return raw
	}
	if allowedIPAddresses[host] {
		return raw
	}
	replacement := prefix + "blocked-ip-address"
	if port != "" {
		// we're keeping the original port as part of the key because ports are much lower cardinality
		// than ip addresses, and they also tend to correspond more closely to a protocol (i.e. 443 is HTTPS)
		// so it's likely safe and probably also useful to leave them in
		replacement = replacement + ":" + port
	}
	return replacement + suffix
}

// parseIPAndPort returns (host, port) if the host is a valid ip address with an optional port, else returns empty strings.
func parseIPAndPort(input string) (host, port, suffix string) {
	host, port, err := net.SplitHostPort(input)
	if err != nil {
		host = input
	}
	if ok, i := isParseableIP(host); ok {
		return host[:i], port, host[i:]
	}
	return "", "", ""
}

func isParseableIP(s string) (parsed bool, lastIndex int) {
	if len(s) == 0 {
		return false, -1
	}
	// Must start with a hex digit, or IPv6 can have a preceding ':'
	switch s[0] {
	case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9',
		'a', 'b', 'c', 'd', 'e', 'f',
		'A', 'B', 'C', 'D', 'E', 'F',
		':':
	default:
		return false, -1
	}
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '.', '_', '-':
			return parseIPv4(s, s[i])
		case ':':
			// IPv6
			if _, err := netip.ParseAddr(s); err == nil {
				return true, len(s)
			}
			return false, -1
		case '%':
			// Assume that this was trying to be an IPv6 address with
			// a zone specifier, but the address is missing.
			return false, -1
		}
	}
	return false, -1
}

// parseIsIPv4 parses s as an IPv4 address and returns whether it is an IP address
// modified from netip to accept alternate separators besides '.'
// also modified to return true if s is an IPv4 address with trailing characters
func parseIPv4(s string, sep byte) (parsed bool, lastIndex int) {
	var fields [4]uint8
	var val, pos int
	var digLen int // number of digits in current octet
	for i := 0; i < len(s); i++ {
		if s[i] >= '0' && s[i] <= '9' {
			if digLen == 1 && val == 0 {
				return false, -1
			}
			val = val*10 + int(s[i]) - '0'
			digLen++
			if val > 255 {
				return false, -1
			}
		} else if s[i] == sep {
			// .1.2.3
			// 1.2.3.
			// 1..2.3
			if i == 0 || i == len(s)-1 || s[i-1] == sep {
				return false, -1
			}
			// 1.2.3.4.5
			if pos == 3 {
				return true, i
			}
			fields[pos] = uint8(val)
			pos++
			val = 0
			digLen = 0
		} else {
			if pos == 3 && digLen > 0 {
				fields[3] = uint8(val)
				return true, i
			}
			return false, -1
		}
	}
	if pos < 3 {
		return false, -1
	}
	fields[3] = uint8(val)
	return true, len(s)
}
