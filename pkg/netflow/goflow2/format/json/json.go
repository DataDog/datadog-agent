package json

import (
	"context"
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/netflow/goflow2/format/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/golang/protobuf/proto"
	"github.com/oschwald/geoip2-golang"
	flowmessage "github.com/netsampler/goflow2/pb"
	"net"
	"time"
)

type JsonDriver struct {
	MetricChan chan []metrics.MetricSample
}

func (d *JsonDriver) Prepare() error {
	common.HashFlag()
	common.SelectorFlag()
	return nil
}

func (d *JsonDriver) Init(context.Context) error {
	err := common.ManualHashInit()
	if err != nil {
		return err
	}
	return common.ManualSelectorInit()
}

func (d *JsonDriver) Format(data interface{}) ([]byte, []byte, error) {
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
	dstDnsList, err := net.LookupAddr(dstAddr.String())
	if err != nil {
		log.Debugf("DNS lookup error for addr `%s`:", dstAddr, err)
	}
	srcDnsList, err := net.LookupAddr(srcAddr.String())
	if err != nil {
		log.Debugf("DNS lookup error for addr `%s`:", srcAddr, err)
	}

	tags := []string{
		fmt.Sprintf("src_addr:%s", srcAddr),
		fmt.Sprintf("src_port:%d", flowmsg.SrcPort),
		fmt.Sprintf("proto:%d", flowmsg.Proto),
		fmt.Sprintf("proto_name:%s", protoName),
		fmt.Sprintf("dst_addr:%s", dstAddr),
		fmt.Sprintf("dst_port:%d", flowmsg.DstPort),
		fmt.Sprintf("type:%s", eType),
		fmt.Sprintf("icmp_type:%s", icmpType),
	}
	if dstL7ProtoName != "" {
		tags = append(tags, fmt.Sprintf("dst_l7_proto_name:%s", dstL7ProtoName))
	}

	if srcL7ProtoName != "" {
		tags = append(tags, fmt.Sprintf("src_l7_proto_name:%s", srcL7ProtoName))
	}

	for _, dns := range dstDnsList {
		tags = append(tags, fmt.Sprintf("dst_dns:%s", dns))
	}

	for _, dns := range srcDnsList {
		tags = append(tags, fmt.Sprintf("src_dns:%s", dns))
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
			tags = append(tags, fmt.Sprintf("dst_country_code:%s", geoCity.Country.IsoCode))
			for _, countryName := range geoCity.Country.Names {
				tags = append(tags, fmt.Sprintf("dst_country_name:%s", countryName))
			}
			for _, cityName := range geoCity.City.Names {
				tags = append(tags, fmt.Sprintf("dst_city_name:%s", cityName))
			}
		}
		geoASN, err := db.ASN(dstAddr)
		if err != nil {
			log.Debugf("error getting ASN `%s`:", dstAddr, err)
		} else {
			tags = append(tags, fmt.Sprintf("dst_as_number:%d", geoASN.AutonomousSystemNumber))
			tags = append(tags, fmt.Sprintf("dst_as_org:%s", geoASN.AutonomousSystemOrganization))
		}
		connType, err := db.ConnectionType(dstAddr)
		if err != nil {
			log.Debugf("error getting ConnectionType `%s`:", dstAddr, err)
		} else {
			tags = append(tags, fmt.Sprintf("dst_conn_type:%s", connType.ConnectionType))
		}
		domain, err := db.Domain(dstAddr)
		if err != nil {
			log.Debugf("error getting ConnectionType `%s`:", dstAddr, err)
		} else {
			tags = append(tags, fmt.Sprintf("dst_domain:%s", domain.Domain))
		}
		isp, err := db.ISP(dstAddr)
		if err != nil {
			log.Debugf("error getting ConnectionType `%s`:", dstAddr, err)
		} else {
			tags = append(tags, fmt.Sprintf("dst_isp:%s", isp.ISP))
			tags = append(tags, fmt.Sprintf("dst_isp_org:%s", isp.ISP))
			tags = append(tags, fmt.Sprintf("dst_isp_as_number:%d", isp.AutonomousSystemNumber))
			tags = append(tags, fmt.Sprintf("dst_isp_as_org:%s", isp.AutonomousSystemOrganization))
		}
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

func (d *JsonDriver) getGeoCity(db *geoip2.Reader, ip net.IP) (countryCode *geoip2.City, err error) {
	record, err := db.City(ip)
	if err != nil {
		return nil, err
	}
	return record, nil
}

//func init() {
//	d := &JsonDriver{}
//	format.RegisterFormatDriver("json", d)
//}
