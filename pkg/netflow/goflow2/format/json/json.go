package json

import (
	"context"
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/netflow/goflow2/format/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/golang/protobuf/proto"
	"github.com/jpillora/go-tld"
	flowmessage "github.com/netsampler/goflow2/pb"
	"github.com/oschwald/geoip2-golang"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Driver desc
type Driver struct {
	MetricChan chan []metrics.MetricSample
}

// Prepare desc
func (d *Driver) Prepare() error {
	common.HashFlag()
	common.SelectorFlag()
	return nil
}

// Init desc
func (d *Driver) Init(context.Context) error {
	err := common.ManualHashInit()
	if err != nil {
		return err
	}
	return common.ManualSelectorInit()
}

// Format desc
func (d *Driver) Format(data interface{}) ([]byte, []byte, error) {
	flowmsg, ok := data.(*flowmessage.FlowMessage)
	if !ok {
		return nil, nil, fmt.Errorf("message is not flowmessage.FlowMessage")
	}
	srcAddr := net.IP(flowmsg.SrcAddr)
	dstAddr := net.IP(flowmsg.DstAddr)

	protoName, ok := common.ProtoName[flowmsg.Proto]
	if !ok {
		protoName = "unknown"
	}

	eType, ok := common.EtypeName[flowmsg.Etype]
	if !ok {
		eType = "unknown"
	}

	dstL7ProtoName, _ := common.L7ProtoName[flowmsg.DstPort]
	srcL7ProtoName, _ := common.L7ProtoName[flowmsg.SrcPort]

	icmpType := common.IcmpCodeType(flowmsg.Proto, flowmsg.IcmpCode, flowmsg.IcmpType)
	if icmpType == "" {
		icmpType = "unknown"
	}
	dstDomainList, err := net.LookupAddr(dstAddr.String())
	if err != nil {
		log.Debugf("DNS lookup error for addr `%s`:", dstAddr, err)
	}
	srcDomainList, err := net.LookupAddr(srcAddr.String())
	if err != nil {
		log.Debugf("DNS lookup error for addr `%s`:", srcAddr, err)
	}

	tags := []string{
		fmt.Sprintf("src_addr:%s", srcAddr),
		fmt.Sprintf("src_port:%s", sanitizePort(flowmsg.SrcPort)),
		fmt.Sprintf("proto:%d", flowmsg.Proto),
		fmt.Sprintf("proto_name:%s", protoName),
		fmt.Sprintf("dst_addr:%s", dstAddr),
		fmt.Sprintf("dst_port:%s", sanitizePort(flowmsg.DstPort)),
		fmt.Sprintf("type:%s", eType),
		fmt.Sprintf("icmp_type:%s", icmpType),
	}
	if dstL7ProtoName != "" {
		tags = append(tags, fmt.Sprintf("dst_l7_proto_name:%s", dstL7ProtoName))
	}

	if srcL7ProtoName != "" {
		tags = append(tags, fmt.Sprintf("src_l7_proto_name:%s", srcL7ProtoName))
	}

	for _, domain := range dstDomainList {
		tags = append(tags, fmt.Sprintf("dst_domain:%s", domain))
		rootDomain, err := tld.Parse(domain)
		if err != nil {
			log.Debugf("error parsing dns `%s`:", domain, err)
		} else {
			tags = append(tags, fmt.Sprintf("dst_root_domain:%s", rootDomain))
		}
	}

	for _, domain := range srcDomainList {
		tags = append(tags, fmt.Sprintf("src_domain:%s", domain))
		rootDomain, err := tld.Parse(domain)
		if err != nil {
			log.Debugf("error parsing dns `%s`:", domain, err)
		} else {
			tags = append(tags, fmt.Sprintf("src_root_domain:%s", rootDomain))
		}
	}

	db, err := geoip2.Open("/opt/geoip_files/GeoIP2-City.mmdb")
	if err != nil {
		log.Debugf("error opening geoip2:", err)
	} else {
		defer db.Close()

		geoCity, err := db.City(dstAddr)
		if err != nil {
			log.Debugf("error getting city `%s`:", dstAddr, err)
		} else {
			if geoCity.Country.IsoCode != "" {
				tags = append(tags, fmt.Sprintf("dst_country_code:%s", geoCity.Country.IsoCode))
			}
			for _, countryName := range geoCity.Country.Names {
				tags = append(tags, fmt.Sprintf("dst_country_name:%s", countryName))
			}
			for _, cityName := range geoCity.City.Names {
				tags = append(tags, fmt.Sprintf("dst_city_name:%s", cityName))
			}
		}
		//geoASN, err := db.ASN(dstAddr)
		//if err != nil {
		//	log.Debugf("error getting ASN `%s`:", dstAddr, err)
		//} else {
		//	tags = append(tags, fmt.Sprintf("dst_as_number:%d", geoASN.AutonomousSystemNumber))
		//	tags = append(tags, fmt.Sprintf("dst_as_org:%s", geoASN.AutonomousSystemOrganization))
		//}
		//connType, err := db.ConnectionType(dstAddr)
		//if err != nil {
		//	log.Debugf("error getting ConnectionType `%s`:", dstAddr, err)
		//} else {
		//	tags = append(tags, fmt.Sprintf("dst_conn_type:%s", connType.ConnectionType))
		//}
		//domain, err := db.Domain(dstAddr)
		//if err != nil {
		//	log.Debugf("error getting ConnectionType `%s`:", dstAddr, err)
		//} else {
		//	tags = append(tags, fmt.Sprintf("dst_domain:%s", domain.Domain))
		//}
		//isp, err := db.ISP(dstAddr)
		//if err != nil {
		//	log.Debugf("error getting ConnectionType `%s`:", dstAddr, err)
		//} else {
		//	tags = append(tags, fmt.Sprintf("dst_isp:%s", isp.ISP))
		//	tags = append(tags, fmt.Sprintf("dst_isp_org:%s", isp.ISP))
		//	tags = append(tags, fmt.Sprintf("dst_isp_as_number:%d", isp.AutonomousSystemNumber))
		//	tags = append(tags, fmt.Sprintf("dst_isp_as_org:%s", isp.AutonomousSystemOrganization))
		//}
	}

	log.Debugf("tags: %v", tags)
	timestamp := float64(time.Now().UnixNano())
	enhancedMetrics := []metrics.MetricSample{
		{
			Name:       "netflow.bytes",
			Value:      float64(flowmsg.Bytes),
			Mtype:      metrics.CountType,
			Tags:       tags,
			SampleRate: 1,
			Timestamp:  timestamp,
		},
		{
			Name:       "netflow.packets",
			Value:      float64(flowmsg.Packets),
			Mtype:      metrics.CountType,
			Tags:       tags,
			SampleRate: 1,
			Timestamp:  timestamp,
		},
	}
	d.MetricChan <- enhancedMetrics

	msg, ok := data.(proto.Message)
	if !ok {
		return nil, nil, fmt.Errorf("message is not protobuf")
	}

	key := common.HashProtoLocal(msg)
	return []byte(key), []byte(common.FormatMessageReflectJSON(msg, "")), nil
}

func sanitizePort(port uint32) string {
	// TODO: this is a naive way to sanitze port
	var strPort string
	if port > 1024 {
		strPort = "x"
	} else {
		strPort = strconv.Itoa(int(port))
	}
	return strPort
}

// Replacer structure to store regex matching logs parts to replace
type Replacer struct {
	Regex *regexp.Regexp
	Repl  string
}

var replacers = []Replacer{
	{
		Regex: regexp.MustCompile(`ec2-.*\.amazonaws\.com`),
		Repl:  "ec2-*.amazonaws.com",
	},
	{
		Regex: regexp.MustCompile(`.*\.awsglobalaccelerator\.com`),
		Repl:  "*.awsglobalaccelerator.com",
	},
	{
		Regex: regexp.MustCompile(`.*\.bc\.googleusercontent\.com`),
		Repl:  "*.bc.googleusercontent.com",
	},
	{
		Regex: regexp.MustCompile(`par.+-in-.+\.1e100\.net`),
		Repl:  "par*-in-*.1e100.net",
	},
	{
		Regex: regexp.MustCompile(`lb-.*\.github\.com`),
		Repl:  "lb-*.github.com",
	},
	{
		Regex: regexp.MustCompile(`.*\.ipv6\.abo\.wanadoo\.fr`),
		Repl:  "*.ipv6.abo.wanadoo.fr",
	},
	{
		Regex: regexp.MustCompile(`server-.*\.cloudfront\.net`),
		Repl:  "server-*.cloudfront.net",
	},
}

func sanitizeHost(host string) string {
	host = strings.TrimSuffix(host, ".")
	if strings.HasPrefix(host, "ec2-") && strings.HasSuffix(host, "amazon.com") {
	}
	for _, replacer := range replacers {
		if replacer.Regex.MatchString(host) {
			host = replacer.Regex.ReplaceAllLiteralString(host, replacer.Repl)
		}
	}
	return host
}
