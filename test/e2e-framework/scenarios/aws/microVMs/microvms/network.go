// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package microvms

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/pulumi/pulumi-libvirt/sdk/go/libvirt"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/namer"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/command"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/microVMs/microvms/resources"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/microVMs/vmconfig"
)

// The microvm subnet changed from /16 to /24 because the underlying libvirt sdk would identify
// the incorrect network interface. It looks like it does not respect the subnet range when the subnet
// used is /16.
// TODO: this problem only manifests when setting up VMs locally. Investigate the root cause to see what can
// be done. This solution may no longer work when the number of VMs exceeds the ips available in this subnet.
const microVMGroupSubnetTemplate = "100.%d.0.0/24"

const tcpRPCInfoPorts = "rpcinfo -p | grep -e portmapper -e mountd -e nfs | grep tcp | rev | cut -d ' ' -f 3 | rev | sort | uniq | tr '\n' ' ' | awk '{$1=$1};1' | tr ' ' ',' | tr -d '\n'"
const udpRPCInfoPorts = "rpcinfo -p | grep -e portmapper -e mountd -e nfs | grep udp | rev | cut -d ' ' -f 3 | rev | sort | uniq | tr '\n' ' ' | awk '{$1=$1};1' | tr ' ' ',' | tr -d '\n'"

const iptablesDeleteRuleFlag = "-D"
const iptablesAddRuleFlag = "-A"

const iptablesTCPRule = "iptables %s INPUT -p tcp -i %s -s %s -m multiport --dports $(%s) -m state --state NEW,ESTABLISHED -j ACCEPT"
const iptablesUDPRule = "iptables %s INPUT -p udp -i %s -s %s -m multiport --dports $(%s) -j ACCEPT"

func freeSubnet(subnet string) (bool, error) {
	startIP, _, err := net.ParseCIDR(subnet)
	if err != nil {
		return false, err
	}

	ifaces, err := net.Interfaces()
	if err != nil {
		return false, err
	}

	for _, i := range ifaces {
		addrs, err := i.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			switch v := a.(type) {
			case *net.IPNet:
				if v.Contains(startIP) {
					return false, nil
				}
			}
		}
	}

	return true, nil
}

func getMicroVMGroupSubnetPattern(subnet string) string {
	ip, _, _ := net.ParseCIDR(subnet)
	ipv4 := ip.To4()
	// this assumes a /24
	return fmt.Sprintf("%d.%d.%d.*", ipv4[0], ipv4[1], ipv4[2])
}

func subnetIsTaken(taken []string, subnet string) bool {
	return slices.Contains(taken, subnet)
}

func getMicroVMGroupSubnet(taken []string) (string, error) {
	for i := 1; i < 254; i++ {
		subnet := fmt.Sprintf(microVMGroupSubnetTemplate, i)
		if subnetIsTaken(taken, subnet) {
			continue
		}
		if free, err := freeSubnet(subnet); err == nil && free {
			return subnet, nil
		}
	}

	return "", fmt.Errorf("getMicroVMGroupSubnet: could not find subnet")
}

func allowNFSPortsForBridge(ctx *pulumi.Context, isLocal bool, bridge pulumi.StringOutput, runner command.Runner, resourceNamer namer.Namer, microVMGroupSubnet string) ([]pulumi.Resource, error) {
	sudoPassword := GetSudoPassword(ctx, isLocal)
	iptablesAllowTCPArgs := command.Args{
		Create:                   pulumi.Sprintf(iptablesTCPRule, iptablesAddRuleFlag, bridge, microVMGroupSubnet, tcpRPCInfoPorts),
		Delete:                   pulumi.Sprintf(iptablesTCPRule, iptablesDeleteRuleFlag, bridge, microVMGroupSubnet, tcpRPCInfoPorts),
		Sudo:                     true,
		RequirePasswordFromStdin: true,
		Stdin:                    sudoPassword,
	}
	iptablesAllowTCPDone, err := runner.Command(resourceNamer.ResourceName("allow-nfs-ports-tcp", microVMGroupSubnet), &iptablesAllowTCPArgs)
	if err != nil {
		return nil, err
	}

	iptablesAllowUDPArgs := command.Args{
		Create:                   pulumi.Sprintf(iptablesUDPRule, iptablesAddRuleFlag, bridge, microVMGroupSubnet, udpRPCInfoPorts),
		Delete:                   pulumi.Sprintf(iptablesUDPRule, iptablesDeleteRuleFlag, bridge, microVMGroupSubnet, udpRPCInfoPorts),
		Sudo:                     true,
		RequirePasswordFromStdin: true,
		Stdin:                    sudoPassword,
	}
	iptablesAllowUDPDone, err := runner.Command(resourceNamer.ResourceName("allow-nfs-ports-udp", microVMGroupSubnet), &iptablesAllowUDPArgs)
	if err != nil {
		return nil, err
	}

	return []pulumi.Resource{iptablesAllowTCPDone, iptablesAllowUDPDone}, nil
}

func generateNetworkResource(
	ctx *pulumi.Context,
	providerFn LibvirtProviderFn,
	depends []pulumi.Resource,
	resourceNamer namer.Namer,
	dhcpEntries []interface{},
	microVMGroupSubnet string,
	setID vmconfig.VMSetID,
) (*libvirt.Network, error) {
	// Collect all DHCP entries in a single string to be
	// formatted in network XML.
	dhcpEntriesJoined := pulumi.All(dhcpEntries...).ApplyT(
		func(promises []interface{}) (string, error) {
			var sb strings.Builder

			for _, promise := range promises {
				sb.WriteString(promise.(string))
			}

			return sb.String(), nil
		},
	).(pulumi.StringInput)

	provider, err := providerFn()
	if err != nil {
		return nil, err
	}

	netXML := resources.GetDefaultNetworkXLS(
		map[string]pulumi.StringInput{
			resources.DHCPEntries: dhcpEntriesJoined,
		},
	)
	network, err := libvirt.NewNetwork(ctx, resourceNamer.ResourceName("network", setID.String()), &libvirt.NetworkArgs{
		Addresses: pulumi.StringArray{pulumi.String(microVMGroupSubnet)},
		Mode:      pulumi.String("nat"),
		// enable jumbo frames for the underlying interface. This is an optimization for NFS.
		Mtu: pulumi.Int(9000),
		Xml: libvirt.NetworkXmlArgs{
			Xslt: netXML,
		},
	}, pulumi.Provider(provider), pulumi.DeleteBeforeReplace(true), pulumi.DependsOn(depends))
	if err != nil {
		return nil, err
	}

	return network, nil
}

type dhcpLease struct {
	name string
	ip   string
	mac  string
}

// parseBootpDHCPLeases parses the dhcpd_leases file and returns a slice of dhcpLease structs.
func parseBootpDHCPLeases() ([]dhcpLease, error) {
	var leases []dhcpLease

	file, err := os.Open("/var/db/dhcpd_leases")
	if os.IsNotExist(err) {
		return leases, nil
		// Do not return error if file was not found, it only gets created when a lease is assigned.
	} else if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var parsingLease dhcpLease
	for scanner.Scan() {
		// Single lease format, for reference:
		// {
		// 	name=ddvm
		// 	ip_address=192.168.64.3
		// 	hw_address=1,28:21:40:26:78:37
		// 	identifier=1,28:21:40:26:78:37
		// 	lease=0x65ce3cb6
		// }
		line := strings.TrimSpace(scanner.Text())
		if after, ok := strings.CutPrefix(line, "name="); ok {
			parsingLease.name = after
		}
		if after, ok := strings.CutPrefix(line, "ip_address="); ok {
			parsingLease.ip = after
		}
		if after, ok := strings.CutPrefix(line, "hw_address="); ok {
			hwaddr := after
			parts := strings.Split(hwaddr, ",")

			if len(parts) != 2 {
				return nil, fmt.Errorf("parseBootpDHCPLeases: invalid hw_address format: %s", hwaddr)
			}

			if parts[0] != "1" {
				// Only parse Ethernet MAC addresses, which are identified by the first part being 1.
				continue
			}

			mac, err := normalizeMAC(parts[1])
			if err != nil {
				return nil, fmt.Errorf("parseBootpDHCPLeases: error normalizing MAC address: %w", err)
			}
			parsingLease.mac = mac
		}
		if line == "}" {
			leases = append(leases, parsingLease)
			parsingLease = dhcpLease{}
		}
	}

	return leases, nil
}

func parseArpDhcpLeases() ([]dhcpLease, error) {
	var leases []dhcpLease

	cmd := exec.Command("arp", "-a", "-n")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("cannot run arp command (arp -an): %w", err)
	}

	lines := strings.SplitSeq(string(out), "\n")
	for line := range lines {
		if len(line) == 0 {
			continue
		}

		lease, err := parseArpLine(line)
		if err != nil {
			// Ignore invalid lines
			fmt.Printf("warning: error parsing arp line: %v\n", err)
			continue
		}
		if lease.mac == "" {
			continue
		}

		leases = append(leases, lease)
	}

	return leases, nil
}

func parseArpLine(line string) (dhcpLease, error) {
	// Single lease format, for reference:
	// ? (10.211.55.4) at 0:1c:42:a:70:b on bridge100 ifscope [bridge]
	parts := strings.Fields(line)
	if len(parts) < 4 {
		return dhcpLease{}, fmt.Errorf("line %s has not enough fields", line)
	}

	ip := strings.Trim(parts[1], "()")
	if parts[3] == "(incomplete)" {
		return dhcpLease{}, nil
	}

	mac, err := normalizeMAC(parts[3])
	if err != nil {
		return dhcpLease{}, fmt.Errorf("error normalizing MAC address: %w", err)
	}

	return dhcpLease{
		ip:  ip,
		mac: mac,
	}, nil
}

// normalizeMAC normalizes a MAC address to the format XX:XX:XX:XX:XX:XX, in lowercase and with leading zeros.
// We need to use this custom function instead of net.ParseMAC because the latter does not support MAC addresses
// without leading zeros, which is the format that we can find in both the BootP and the ARP tables.
func normalizeMAC(mac string) (string, error) {
	parts := strings.Split(mac, ":")
	if len(parts) != 6 {
		return "", fmt.Errorf("normalizeMAC: invalid MAC address %s, not enough fields", mac)
	}

	var addr net.HardwareAddr = make([]byte, 6)
	for i, part := range parts {
		num, err := strconv.ParseUint(part, 16, 8)
		if err != nil {
			return "", fmt.Errorf("normalizeMAC: invalid MAC address %s, cannot parse %s: %w", mac, part, err)
		}
		addr[i] = byte(num)
	}

	return addr.String(), nil
}

// waitForDHCPLeases waits for the macOS DHCP server (BootP) to assign an IP address to the VM based on its MAC address, and
// returns that IP address.
func waitForDHCPLeases(mac string) (string, error) {
	normalizedMac, err := normalizeMAC(mac)
	if err != nil {
		return "", fmt.Errorf("waitForBootpDHCPLeases: invalid MAC address: %w", err)
	}

	// The DHCP server will assign an IP address to the VM based on its MAC address, wait until it is assigned
	// and then return the IP address.
	maxWait := 5 * time.Minute
	interval := 500 * time.Millisecond
	for totalWait := 0 * time.Second; totalWait < maxWait; totalWait += interval {
		// Try to get the address asignment from the DHCP lease and also via ARP. The reason is that
		// it seems that sometimes the DHCP lease will not include the correct mac address but will
		// use another type of identifier. Combining the DHCP lease and ARP address table should
		// give us more coverage to find the correct IP address.
		leases, err := parseBootpDHCPLeases()
		if err != nil {
			return "", fmt.Errorf("waitForBootpDHCPLeases: error parsing leases: %s", err)
		}

		arpLeases, err := parseArpDhcpLeases()
		if err != nil {
			return "", fmt.Errorf("waitForBootpDHCPLeases: error parsing arp leases: %s", err)
		}

		leases = append(leases, arpLeases...)

		foundIP := ""
		for _, lease := range leases {
			if lease.mac == normalizedMac {
				if foundIP != "" && foundIP != lease.ip {
					return "", fmt.Errorf("waitForBootpDHCPLeases: found multiple conflicting leases for MAC %s: %s and %s", mac, foundIP, lease.ip)
				}
				foundIP = lease.ip
			}
		}

		if foundIP != "" {
			return foundIP, nil
		}

		time.Sleep(interval)
	}
	return "", fmt.Errorf("waitForBootpDHCPLeases: timed out waiting for lease")
}
