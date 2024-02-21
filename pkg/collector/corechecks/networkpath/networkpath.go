package networkpath

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sort"
	"time"

	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/optional"

	"github.com/google/uuid"
	"github.com/ken-schneider/dublin-traceroute/go/dublintraceroute"
	"github.com/ken-schneider/dublin-traceroute/go/dublintraceroute/probes/probev4"
	"github.com/ken-schneider/dublin-traceroute/go/dublintraceroute/results"
)

const CheckName = "networkpath"

// Check doesn't need additional fields
type Check struct {
	core.CheckBase
	config        *CheckConfig
	lastCheckTime time.Time
}

// Program constants and default values
const (
	DefaultSourcePort   = 12345
	DefaultDestPort     = 33434
	DefaultNumPaths     = 10
	DefaultMinTTL       = 1
	DefaultMaxTTL       = 30
	DefaultDelay        = 50 //msec
	DefaultReadTimeout  = 3 * time.Second
	DefaultOutputFormat = "json"
)

// Run executes the check
func (c *Check) Run() error {
	startTime := time.Now()
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
	senderInstance.Gauge("networkpath.telemetry.count", 1, "", tags)
	senderInstance.Gauge("networkpath.telemetry.duration", duration.Seconds(), "", tags)

	if !c.lastCheckTime.IsZero() {
		interval := startTime.Sub(c.lastCheckTime)
		senderInstance.Gauge("networkpath.telemetry.interval", interval.Seconds(), "", tags)
	}
	senderInstance.Commit()

	numWorkers := config.Datadog.GetInt("check_runners")
	senderInstance.Gauge("networkpath.telemetry.check_runners", float64(numWorkers), "", tags)
	senderInstance.Gauge("networkpath.telemetry.fake_event_multiplier", float64(c.config.FakeEventMultiplier), "", tags)
	senderInstance.Gauge("networkpath.telemetry.hop_count", float64(hopCount), "", tags)
	c.lastCheckTime = startTime

	senderInstance.Commit()
	return nil
}

func (c *Check) traceroute(senderInstance sender.Sender) (int, error) {
	rawDest := c.config.DestHostname
	dests, err := net.LookupIP(rawDest)
	if err != nil || len(dests) == 0 {
		return 0, fmt.Errorf("Cannot resolve %s: %v", rawDest, err)
	}

	//TODO: should we get smarter about IP address resolution?
	// if it's a hostname, perhaps we could run multiple traces
	// for each of the different IPs it resolves to up to a threshold?
	// use first resolved IP for now
	dest := dests[0]

	numpaths := 1
	if numpaths == 0 {
		numpaths = DefaultNumPaths
	}

	var dt dublintraceroute.DublinTraceroute
	dt = &probev4.UDPv4{
		Target:     dest,
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

	err = c.traceRouteDublinAsPath(senderInstance, results, hname, rawDest, dest)
	if err != nil {
		return 0, err
	}

	log.Debugf("results: %+v", results)

	if len(results.Flows) == 1 {
		for k := range results.Flows {
			hops := results.Flows[k]
			return len(hops), nil
		}
	}

	return 0, nil
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
		if err != nil {
			return fmt.Errorf("failed to create first node: %w", err)
		}
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

			hop := NetworkPathHop{
				TTL:       idx,
				IpAddress: ip,
				Hostname:  c.getHostname(cur.node),
				RTT:       durationMs,
				Success:   isSuccess,
			}
			traceroutePath.Hops = append(traceroutePath.Hops, hop)
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

// Configure the networkpath check
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
