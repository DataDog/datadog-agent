// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package netpath

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/netpath/dublintraceroute"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/netpath/dublintraceroute/probes/probev4"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/netpath/dublintraceroute/results"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/netpath/traceroute"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
	"github.com/google/uuid"
	"github.com/shirou/gopsutil/v3/cpu"
	"net"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
)

// Program constants and default values
const (
	ProgramName         = "Dublin NetworkPath"
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

const CheckName = "netpath"

// TODO: FIXME The mutex is used to prevent multiple checks running at the same
//
//	It seems there are some concurrency issues
var globalMu = &sync.Mutex{}

// Check doesn't need additional fields
type Check struct {
	core.CheckBase
	nbCPU         float64
	lastNbCycle   float64
	lastTimes     cpu.TimesStat
	config        *CheckConfig
	lastCheckTime time.Time
}

// Run executes the check
func (c *Check) Run() error {
	startTime := time.Now()
	//globalMu.Lock()
	//defer globalMu.Unlock()
	senderInstance, err := c.GetSender()
	if err != nil {
		return err
	}

	hopCount, err := c.traceroute(senderInstance)
	if err != nil {
		return err
	}

	tags := []string{
		"dest_hostname:" + c.config.DestHostname,
		"dest_name:" + c.config.DestName,
	}
	duration := time.Since(startTime)
	senderInstance.Gauge("netpath.telemetry.count", 1, "", tags)
	senderInstance.Gauge("netpath.telemetry.duration", duration.Seconds(), "", tags)

	if !c.lastCheckTime.IsZero() {
		interval := startTime.Sub(c.lastCheckTime)
		senderInstance.Gauge("netpath.telemetry.interval", interval.Seconds(), "", tags)
	}
	senderInstance.Commit()

	numWorkers := config.Datadog.GetInt("check_runners")
	senderInstance.Gauge("netpath.telemetry.check_runners", float64(numWorkers), "", tags)
	senderInstance.Gauge("netpath.telemetry.fake_event_multiplier", float64(c.config.FakeEventMultiplier), "", tags)
	senderInstance.Gauge("netpath.telemetry.hop_count", float64(hopCount), "", tags)
	c.lastCheckTime = startTime

	senderInstance.Commit()
	return nil
}

func (c *Check) traceroute(senderInstance sender.Sender) (int, error) {
	rawTarget := c.config.DestHostname
	target, err := resolve(rawTarget, false)
	if err != nil {
		return 0, fmt.Errorf("Cannot resolve %s: %v", rawTarget, err)
	}

	numpaths := 1
	if numpaths == 0 {
		numpaths = DefaultNumPaths
	}

	var dt dublintraceroute.DublinTraceroute
	dt = &probev4.UDPv4{
		Target:     target,
		SrcPort:    uint16(DefaultSourcePort),
		DstPort:    uint16(DefaultDestPort),
		UseSrcPort: false,
		NumPaths:   uint16(numpaths),
		MinTTL:     uint8(DefaultMinTTL),
		MaxTTL:     uint8(15),
		Delay:      time.Duration(DefaultDelay) * time.Millisecond,
		Timeout:    DefaultReadTimeout,
		BrokenNAT:  false,
	}
	results, err := dt.Traceroute()
	if err != nil {
		return 0, fmt.Errorf("NetworkPath() failed: %v", err)
	}

	hname, err := hostname.Get(context.TODO())
	if err != nil {
		return 0, err
	}

	err = c.traceRouteDublinAsPath(senderInstance, results, hname, rawTarget, target)
	if err != nil {
		return 0, err
	}

	//err = c.traceRouteDublin(senderInstance, results, hname, rawTarget)
	//if err != nil {
	//	return 0, err
	//}
	log.Debugf("results: %+v", results)

	if len(results.Flows) == 1 {
		for k := range results.Flows {
			hops := results.Flows[k]
			return len(hops), nil
		}
	}

	//results.Flows[0].hop
	return 0, nil
	//options := traceroute.TracerouteOptions{}
	//options.SetRetries(1)
	//options.SetMaxHops(15)
	////options.SetFirstHop(traceroute.DEFAULT_FIRST_HOP)
	//times := 1
	//destinationHost := c.config.DestHostname
	//
	//ipAddr, err := net.ResolveIPAddr("ip", destinationHost)
	//if err != nil {
	//	return 0, nil
	//}
	//
	//fmt.Printf("traceroute to %v (%v), %v hops max, %v byte packets\n", destinationHost, ipAddr, options.MaxHops(), options.PacketSize())
	//
	//hostHops := getHops(options, times, err, destinationHost)
	//if len(hostHops) == 0 {
	//	return 0, errors.New("no hops")
	//}
	//
	//err = c.traceRouteV1(senderInstance, hostHops, hname, destinationHost)
	//if err != nil {
	//	return 0, err
	//}
	//
	//err = c.traceRouteV2(senderInstance, hostHops, hname, destinationHost)
	//if err != nil {
	//	return 0, err
	//}
	//
	//return len(hostHops[0]), nil
}

//func (c *Check) traceRouteV1(sender sender.Sender, hostHops [][]traceroute.TracerouteHop, hname string, destinationHost string) error {
//	tr := metadata.NetworkPath{
//		Timestamp: time.Now().UnixMilli(),
//		Source: metadata.NetworkPathSource{
//			Hostname: hname,
//		},
//		Destination: metadata.NetworkPathDestination{
//			Hostname: destinationHost,
//		},
//	}
//
//	hops := hostHops[0]
//	for _, hop := range hops {
//		ipAddress := hop.AddressString()
//		hop := metadata.NetworkPathHop{
//			TTL:       hop.TTL,
//			IpAddress: ipAddress,
//			Hostname:  hop.HostOrAddressString(),
//			RTT:       hop.ElapsedTime.Seconds(),
//			Success:   hop.Success,
//		}
//		tr.Hops = append(tr.Hops, hop)
//	}
//
//	flushTime := time.Now()
//	metadataPayloads := metadata.BatchPayloads(
//		"default",
//		"",
//		flushTime,
//		metadata.PayloadMetadataBatchSize,
//		nil,
//		nil,
//		nil,
//		nil,
//		nil,
//		[]metadata.NetworkPath{tr},
//	)
//
//	log.Debugf("metadataPayloads: %d", len(metadataPayloads))
//	for _, payload := range metadataPayloads {
//		payloadBytes, err := json.Marshal(payload)
//		if err != nil {
//			log.Errorf("Error marshalling device metadata: %s", err)
//			continue
//		}
//		log.Debugf("traceroute path metadata payload: %s", string(payloadBytes))
//		sender.EventPlatformEvent(payloadBytes, eventplatform.EventTypeNetworkDevicesMetadata)
//	}
//
//	return nil
//}

func (c *Check) traceRouteV2(sender sender.Sender, hostHops [][]traceroute.TracerouteHop, hname string, destinationHost string) error {
	hops := hostHops[0]
	var prevHop traceroute.TracerouteHop
	for _, hop := range hops {
		ip := hop.AddressString()
		durationMs := hop.ElapsedTime.Seconds() * 10e3
		tr := TracerouteV2{
			TracerouteSource: "netpath_integration",
			Timestamp:        time.Now().UnixMilli(),
			AgentHost:        hname,
			DestinationHost:  destinationHost,
			TTL:              hop.TTL,
			IpAddress:        ip,
			Host:             hop.HostOrAddressString(),
			Duration:         durationMs,
			Success:          hop.Success,
		}
		tracerouteStr, err := json.MarshalIndent(tr, "", "\t")
		if err != nil {
			return err
		}

		log.Debugf("traceroute: %s", tracerouteStr)

		sender.EventPlatformEvent(tracerouteStr, eventplatform.EventTypeNetworkPath)
		tags := []string{
			"dest_name:" + c.config.DestName,
			"agent_host:" + hname,
			"dest_hostname:" + destinationHost,
			"hop_ip_address:" + ip,
			"hop_host:" + hop.HostOrAddressString(),
			"ttl:" + strconv.Itoa(hop.TTL),
		}
		if prevHop.TTL > 0 {
			prevIp := prevHop.AddressString()
			tags = append(tags, "prev_hop_ip_address:"+prevIp)
			tags = append(tags, "prev_hop_host:"+prevHop.HostOrAddressString())
		}
		log.Infof("[netpath] tags: %s", tags)
		sender.Gauge("netpath.hop.duration", durationMs, "", CopyStrings(tags))
		sender.Gauge("netpath.hop.record", float64(1), "", CopyStrings(tags))

		prevHop = hop
	}

	return nil
}

func (c *Check) traceRouteDublin(sender sender.Sender, r *results.Results, hname string, destinationHost string) error {
	var err error
	type node struct {
		node  string
		probe *results.Probe
	}

	pathId := uuid.New().String()

	for idx, probes := range r.Flows {
		log.Debugf("flow idx: %d\n", idx)
		for probleIndex, probe := range probes {
			//log.Debugf("probleIndex: %d, probe %+v\n", probleIndex, probe)
			log.Debugf("%d - %d - %s\n", probleIndex, probe.Sent.IP.TTL, probe.Name)
		}
	}

	flowIDs := make([]int, 0, len(r.Flows))
	for flowID := range r.Flows {
		flowIDs = append(flowIDs, int(flowID))
	}
	sort.Ints(flowIDs)

	for _, flowID := range flowIDs {
		hops := r.Flows[uint16(flowID)]
		if len(hops) == 0 {
			log.Debugf("No hops for flow ID %d", flowID)
			continue
		}
		var nodes []node
		// add first hop
		firstNodeName := hops[0].Sent.IP.SrcIP.String()
		//firstHop, err := graph.CreateNode(firstNodeName)
		if err != nil {
			return fmt.Errorf("failed to create first node: %w", err)
		}
		//firstHop.SetShape(cgraph.RectShape)
		nodes = append(nodes, node{node: firstNodeName, probe: &hops[0]})

		// then add all the other hops
		for _, hop := range hops {
			hop := hop
			nodename := fmt.Sprintf("unknown_hop_%d)", hop.Sent.IP.TTL)
			label := "*"
			hostname := ""
			if hop.Received != nil {
				nodename = hop.Received.IP.SrcIP.String()
				if hop.Name != nodename {
					hostname = "\n" + hop.Name
				}
				// MPLS labels
				mpls := ""
				if len(hop.Received.ICMP.MPLSLabels) > 0 {
					mpls = "MPLS labels: \n"
					for _, mplsLabel := range hop.Received.ICMP.MPLSLabels {
						mpls += fmt.Sprintf(" - %d, ttl: %d\n", mplsLabel.Label, mplsLabel.TTL)
					}
				}
				label = fmt.Sprintf("%s%s\n%s\n%s", nodename, hostname, hop.Received.ICMP.Description, mpls)
			}
			//n, err := graph.CreateNode(nodename)
			//if err != nil {
			//	return "", fmt.Errorf("failed to create node '%s': %w", nodename, err)
			//}
			//if hop.IsLast {
			//	n.SetShape(cgraph.RectShape)
			//}
			//n.SetLabel(label)
			nodes = append(nodes, node{node: nodename, probe: &hop})

			if hop.IsLast {
				break
			}
			log.Debugf("label: %s", label)
		}
		// add edges
		if len(nodes) <= 1 {
			// no edges to add if there is only one node
			continue
		}
		//color := rand.Intn(0xffffff)
		// start at node 1. Each node back-references the previous one
		for idx := 1; idx < len(nodes); idx++ {
			if idx >= len(nodes) {
				// we are at the second-to-last node
				break
			}
			prev := nodes[idx-1]
			cur := nodes[idx]
			//edgeName := fmt.Sprintf("%s - %s - %d - %d", prev.node, cur.node, idx, flowID)
			edgeLabel := ""
			if idx == 1 {
				edgeLabel += fmt.Sprintf(
					"srcport %d\ndstport %d",
					cur.probe.Sent.UDP.SrcPort,
					cur.probe.Sent.UDP.DstPort,
				)
			}
			if prev.probe.NATID != cur.probe.NATID {
				edgeLabel += "\nNAT detected"
			}
			edgeLabel += fmt.Sprintf("\n%d.%d ms", int(cur.probe.RttUsec/1000), int(cur.probe.RttUsec%1000))

			//tags := []string{
			//	"dest_name:" + c.config.DestName,
			//	"agent_host:" + hname,
			//	"dest_hostname:" + destinationHost,
			//	"hop_ip_address:" + cur.node,
			//	"hop_host:" + c.getHostname(cur.node),
			//	"ttl:" + strconv.Itoa(idx),
			//}
			//tags = append(tags, "prev_hop_ip_address:"+prev.node)
			//tags = append(tags, "prev_hop_host:"+c.getHostname(prev.node))
			//log.Infof("[netpath] tags: %s", tags)
			//sender.Gauge("netpath.hop.duration", float64(cur.probe.RttUsec)/1000, "", CopyStrings(tags))
			//sender.Count("netpath.hop.record", float64(1), "", CopyStrings(tags))

			ip := cur.node
			durationMs := float64(cur.probe.RttUsec) / 1000
			tr := TracerouteV2{
				PathId:           pathId,
				TracerouteSource: "netpath_integration",
				Timestamp:        time.Now().UnixMilli(),
				AgentHost:        hname,
				DestinationHost:  destinationHost,
				TTL:              idx,
				IpAddress:        ip,
				Host:             c.getHostname(cur.node),
				Duration:         durationMs,
				//Success:          hop.Success,
			}
			tracerouteStr, err := json.MarshalIndent(tr, "", "\t")
			if err != nil {
				return err
			}

			log.Debugf("traceroute: %s", tracerouteStr)

			//sender.EventPlatformEvent(tracerouteStr, eventplatform.EventTypeNetworkPath)

			//prevHop = hop

			//edge, err := graph.CreateEdge(edgeName, prev.node, cur.node)
			//if err != nil {
			//	return "", fmt.Errorf("failed to create edge '%s': %w", edgeName, err)
			//}
			//edge.SetLabel(edgeLabel)
			//edge.SetColor(fmt.Sprintf("#%06x", color))
		}
	}
	//var buf bytes.Buffer
	//if err := gv.Render(graph, "dot", &buf); err != nil {
	//	return "", fmt.Errorf("failed to render graph: %w", err)
	//}
	//if err := graph.Close(); err != nil {
	//	return "", fmt.Errorf("failed to close graph: %w", err)
	//}
	//gv.Close()
	//return buf.String(), nil
	return nil
}

func (c *Check) traceRouteDublinAsPath(sender sender.Sender, r *results.Results, hname string, destinationHost string, destinationIP net.IP) error {
	var err error
	type node struct {
		node  string
		probe *results.Probe
	}

	pathId := uuid.New().String()

	traceroutePath := NetworkPath{
		PathId:    pathId,
		Timestamp: time.Now().UnixMilli(),
		Source: NetworkPathSource{
			Hostname: hname,
		},
		Destination: NetworkPathDestination{
			Hostname:  destinationHost,
			IpAddress: destinationIP.String(),
		},
	}

	for idx, probes := range r.Flows {
		log.Debugf("flow idx: %d\n", idx)
		for probleIndex, probe := range probes {
			//log.Debugf("probleIndex: %d, probe %+v\n", probleIndex, probe)
			log.Debugf("%d - %d - %s\n", probleIndex, probe.Sent.IP.TTL, probe.Name)
		}
	}

	flowIDs := make([]int, 0, len(r.Flows))
	for flowID := range r.Flows {
		flowIDs = append(flowIDs, int(flowID))
	}
	sort.Ints(flowIDs)

	for _, flowID := range flowIDs {
		hops := r.Flows[uint16(flowID)]
		if len(hops) == 0 {
			log.Debugf("No hops for flow ID %d", flowID)
			continue
		}
		var nodes []node
		// add first hop
		firstNodeName := hops[0].Sent.IP.SrcIP.String()
		//firstHop, err := graph.CreateNode(firstNodeName)
		if err != nil {
			return fmt.Errorf("failed to create first node: %w", err)
		}
		//firstHop.SetShape(cgraph.RectShape)
		nodes = append(nodes, node{node: firstNodeName, probe: &hops[0]})

		// then add all the other hops
		for _, hop := range hops {
			hop := hop
			nodename := fmt.Sprintf("unknown_hop_%d)", hop.Sent.IP.TTL)
			label := "*"
			hostname := ""
			if hop.Received != nil {
				nodename = hop.Received.IP.SrcIP.String()
				if hop.Name != nodename {
					hostname = "\n" + hop.Name
				}
				// MPLS labels
				mpls := ""
				if len(hop.Received.ICMP.MPLSLabels) > 0 {
					mpls = "MPLS labels: \n"
					for _, mplsLabel := range hop.Received.ICMP.MPLSLabels {
						mpls += fmt.Sprintf(" - %d, ttl: %d\n", mplsLabel.Label, mplsLabel.TTL)
					}
				}
				label = fmt.Sprintf("%s%s\n%s\n%s", nodename, hostname, hop.Received.ICMP.Description, mpls)
			}
			//n, err := graph.CreateNode(nodename)
			//if err != nil {
			//	return "", fmt.Errorf("failed to create node '%s': %w", nodename, err)
			//}
			//if hop.IsLast {
			//	n.SetShape(cgraph.RectShape)
			//}
			//n.SetLabel(label)
			nodes = append(nodes, node{node: nodename, probe: &hop})

			if hop.IsLast {
				break
			}
			log.Debugf("label: %s", label)
		}
		// add edges
		if len(nodes) <= 1 {
			// no edges to add if there is only one node
			continue
		}
		//color := rand.Intn(0xffffff)
		// start at node 1. Each node back-references the previous one
		for idx := 1; idx < len(nodes); idx++ {
			if idx >= len(nodes) {
				// we are at the second-to-last node
				break
			}
			prev := nodes[idx-1]
			cur := nodes[idx]
			//edgeName := fmt.Sprintf("%s - %s - %d - %d", prev.node, cur.node, idx, flowID)
			edgeLabel := ""
			if idx == 1 {
				edgeLabel += fmt.Sprintf(
					"srcport %d\ndstport %d",
					cur.probe.Sent.UDP.SrcPort,
					cur.probe.Sent.UDP.DstPort,
				)
			}
			if prev.probe.NATID != cur.probe.NATID {
				edgeLabel += "\nNAT detected"
			}
			edgeLabel += fmt.Sprintf("\n%d.%d ms", int(cur.probe.RttUsec/1000), int(cur.probe.RttUsec%1000))

			//tags := []string{
			//	"dest_name:" + c.config.DestName,
			//	"agent_host:" + hname,
			//	"dest_hostname:" + destinationHost,
			//	"hop_ip_address:" + cur.node,
			//	"hop_host:" + c.getHostname(cur.node),
			//	"ttl:" + strconv.Itoa(idx),
			//}
			//tags = append(tags, "prev_hop_ip_address:"+prev.node)
			//tags = append(tags, "prev_hop_host:"+c.getHostname(prev.node))
			//log.Infof("[netpath] tags: %s", tags)
			//sender.Gauge("netpath.hop.duration", float64(cur.probe.RttUsec)/1000, "", CopyStrings(tags))
			//sender.Count("netpath.hop.record", float64(1), "", CopyStrings(tags))
			isSuccess := cur.probe.Received != nil

			ip := cur.node
			durationMs := float64(cur.probe.RttUsec) / 1000
			//tr := TracerouteV2{
			//	PathId:           pathId,
			//	TracerouteSource: "netpath_integration",
			//	Timestamp:        time.Now().UnixMilli(),
			//	AgentHost:        hname,
			//	DestinationHost:  destinationHost,
			//	TTL:              idx,
			//	IpAddress:        ip,
			//	Host:             c.getHostname(cur.node),
			//	Duration:         durationMs,
			//	Success:          isSuccess,
			//}
			//tracerouteStr, err := json.MarshalIndent(tr, "", "\t")
			//if err != nil {
			//	return err
			//}
			//
			//log.Debugf("cur node: %+v", cur)
			//log.Debugf("cur node probe: %+v", cur.probe)
			//log.Debugf("traceroute: %s", tracerouteStr)

			//sender.EventPlatformEvent(tracerouteStr, eventplatform.EventTypeNetworkPath)

			hop := NetworkPathHop{
				TTL:       idx,
				IpAddress: ip,
				Hostname:  c.getHostname(cur.node),
				RTT:       durationMs,
				Success:   isSuccess,
			}
			traceroutePath.Hops = append(traceroutePath.Hops, hop)

			//prevHop = hop

			//edge, err := graph.CreateEdge(edgeName, prev.node, cur.node)
			//if err != nil {
			//	return "", fmt.Errorf("failed to create edge '%s': %w", edgeName, err)
			//}
			//edge.SetLabel(edgeLabel)
			//edge.SetColor(fmt.Sprintf("#%06x", color))
		}
	}

	payloadBytes, err := json.Marshal(traceroutePath)
	if err != nil {
		return fmt.Errorf("error marshalling device metadata: %s", err)
	}
	log.Debugf("traceroute path metadata payload: %s", string(payloadBytes))
	sender.EventPlatformEvent(payloadBytes, eventplatform.EventTypeNetworkPath)
	return nil
}

func (c *Check) getHostname(ipAddr string) string {
	// TODO: this reverse lookup appears to have some standard timeout that is relatively
	// high. Consider switching to something where there is greater control.
	currHost := ""
	currHostList, _ := net.LookupAddr(ipAddr)
	log.Debugf("Reverse DNS List: %+v", currHostList)

	if len(currHostList) > 0 {
		// TODO: Reverse DNS: Do we need to handle cases with multiple DNS being returned?
		currHost = currHostList[0]
	} else {
		currHost = ipAddr
	}
	return currHost
}

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

// Configure the CPU check
func (c *Check) Configure(senderManager sender.SenderManager, integrationConfigDigest uint64, rawInstance integration.Data, rawInitConfig integration.Data, source string) error {
	err := c.CommonConfigure(senderManager, integrationConfigDigest, rawInitConfig, rawInstance, source)
	if err != nil {
		return fmt.Errorf("common configure failed: %s", err)
	}

	// Must be called before c.CommonConfigure
	c.BuildID(integrationConfigDigest, rawInstance, rawInitConfig)

	config, err := NewCheckConfig(rawInstance, rawInitConfig)
	if err != nil {
		return err
	}
	c.config = config
	return nil
}

// Factory creates a new check factory
func Factory() optional.Option[func() check.Check] {
	return optional.NewOption(newCheck)
}

func newCheck() check.Check {
	return &Check{
		CheckBase: core.NewCheckBase(CheckName),
	}
}
