package discoverycollector

import (
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/networkdiscovery/common"
	"github.com/DataDog/datadog-agent/pkg/networkdiscovery/config"
	"github.com/DataDog/datadog-agent/pkg/networkdiscovery/enrichment"
	"github.com/DataDog/datadog-agent/pkg/networkdiscovery/fetch"
	"github.com/DataDog/datadog-agent/pkg/networkdiscovery/session"
	"github.com/DataDog/datadog-agent/pkg/networkdiscovery/valuestore"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"net"
	"strconv"
	"strings"
)

// DiscoveryCollector TODO
type DiscoveryCollector struct {
	sender   aggregator.Sender
	hostname string
	config   *config.NetworkDiscoveryConfig
}

// NewDiscoveryCollector TODO
func NewDiscoveryCollector(sender aggregator.Sender, hostname string, config *config.NetworkDiscoveryConfig) *DiscoveryCollector {
	return &DiscoveryCollector{
		sender:   sender,
		hostname: hostname,
		config:   config,
	}
}

// Collect TODO
func (dc *DiscoveryCollector) Collect() {
	log.Info("Collector: collect")
	log.Infof("Config: %+v", dc.config)
	session, err := session.NewGosnmpSession(dc.config)
	log.Infof("session: %+v", session)
	if err != nil {
		log.Errorf("error creating session: %s", err)
		return
	}
	err = session.Connect()
	if err != nil {
		log.Errorf("error session connection: %s", err)
		return
	}
	defer session.Close()

	log.Info("=== lldpRemManAddrTable\t ===")
	// INDEX { lldpRemTimeMark, lldpRemLocalPortNum, lldpRemIndex, lldpRemManAddrSubtype, lldpRemManAddr }
	// lldpRemManAddrSubtype: ipv4(1), ipv6(2), etc see more here: http://www.mibdepot.com/cgi-bin/getmib3.cgi?win=mib_a&i=1&n=LLDP-MIB&r=cisco&f=LLDP-MIB-V1SMI.my&v=v1&t=tab&o=lldpRemManAddrSubtype
	columns := []string{
		"1.0.8802.1.1.2.1.4.2.1.3", // lldpRemManAddrIfSubtype unknown(1), ifIndex(2), systemPortNumber(3)
		"1.0.8802.1.1.2.1.4.2.1.4", // lldpRemManAddrIfId
		"1.0.8802.1.1.2.1.4.2.1.5", // lldpRemManAddrOID
	}
	columnValues := dc.collectColumnsOids(columns, session)

	ifSubType := columnValues["1.0.8802.1.1.2.1.4.2.1.3"]

	var remoteMans []common.LldpRemoteManagement
	for fullIndex, _ := range ifSubType {
		indexes := strings.Split(fullIndex, ".")
		timeMark, _ := strconv.Atoi(indexes[0])
		localPortNum, _ := strconv.Atoi(indexes[1])
		index, _ := strconv.Atoi(indexes[2])
		manAddrSubtype, _ := strconv.Atoi(indexes[3])
		var manAddr string
		if manAddrSubtype == 1 { // ipv4
			var ipV4Bytes []byte
			for _, val := range indexes[4:] {
				intVal, _ := strconv.Atoi(val)
				ipV4Bytes = append(ipV4Bytes, byte(intVal))
			}
			manAddr = net.IP(ipV4Bytes).String()
		} else if manAddrSubtype == 2 { // ipv6
			var ipv6buf []byte
			// TODO: skip IPv6 should be indexes[4:], but first byte seems to be always 16
			for _, val := range indexes[5:] {
				intVal, _ := strconv.Atoi(val)
				ipv6buf = append(ipv6buf, byte(intVal))
			}
			manAddr = net.IP(ipv6buf).String()
		} else { // ipv4 and others
			manAddr = strings.Join(indexes[4:], ".")
		}

		remoteMan := common.LldpRemoteManagement{
			TimeMark:       timeMark,
			LocalPortNum:   localPortNum,
			Index:          index,
			ManAddrSubtype: manAddrSubtype,
			ManAddr:        manAddr,
		}
		remoteMans = append(remoteMans, remoteMan)
	}
	for _, remoteMan := range remoteMans {
		log.Infof("\t->")
		log.Infof("\t\t TimeMark: %d", remoteMan.TimeMark)
		log.Infof("\t\t LocalPortNum: %d", remoteMan.LocalPortNum)
		log.Infof("\t\t Index: %d", remoteMan.Index)
		log.Infof("\t\t ManAddrSubtype: %s (%d)", common.RemManAddrSubtype[remoteMan.ManAddrSubtype], remoteMan.ManAddrSubtype)
		log.Infof("\t\t manAddr: %s", remoteMan.ManAddr)
	}

	log.Info("=== lldpRemTable ===")
	// INDEX { lldpRemTimeMark, lldpRemLocalPortNum, lldpRemIndex }
	columns = []string{
		"1.0.8802.1.1.2.1.4.1.1.4",  // lldpRemChassisIdSubtype
		"1.0.8802.1.1.2.1.4.1.1.5",  // lldpRemChassisId
		"1.0.8802.1.1.2.1.4.1.1.6",  // lldpRemPortIdSubtype
		"1.0.8802.1.1.2.1.4.1.1.7",  // lldpRemPortId
		"1.0.8802.1.1.2.1.4.1.1.8",  // lldpRemPortDesc
		"1.0.8802.1.1.2.1.4.1.1.9",  // lldpRemSysName
		"1.0.8802.1.1.2.1.4.1.1.10", // lldpRemSysDesc
		"1.0.8802.1.1.2.1.4.1.1.11", // lldpRemSysCapSupported
		"1.0.8802.1.1.2.1.4.1.1.12", // lldpRemSysCapEnabled
	}
	columnValues = dc.collectColumnsOids(columns, session)
	var remotes []common.LldpRemote
	valuesByIndexByColumn := make(map[string]map[string]valuestore.ResultValue)
	for columnOid, values := range columnValues {
		log.Info(columnOid)
		for fullIndex, value := range values {
			if _, ok := valuesByIndexByColumn[fullIndex]; !ok {
				valuesByIndexByColumn[fullIndex] = make(map[string]valuestore.ResultValue)
			}
			valuesByIndexByColumn[fullIndex][columnOid] = value
		}
	}
	dc.collectColumnsOids(columns, session)

	for fullIndex, colValues := range valuesByIndexByColumn {
		indexes := strings.Split(fullIndex, ".")
		timeMark, _ := strconv.Atoi(indexes[0])
		localPortNum, _ := strconv.Atoi(indexes[1])
		index, _ := strconv.Atoi(indexes[2])
		remote := common.LldpRemote{
			TimeMark:     timeMark,
			LocalPortNum: localPortNum,
			Index:        index,
		}

		ChassisIdSubtype := colValues["1.0.8802.1.1.2.1.4.1.1.4"]
		ChassisId := colValues["1.0.8802.1.1.2.1.4.1.1.5"]
		PortIdSubtype := colValues["1.0.8802.1.1.2.1.4.1.1.6"]
		PortId := colValues["1.0.8802.1.1.2.1.4.1.1.7"]
		PortDesc := colValues["1.0.8802.1.1.2.1.4.1.1.8"]
		SysName := colValues["1.0.8802.1.1.2.1.4.1.1.9"]
		SysDesc := colValues["1.0.8802.1.1.2.1.4.1.1.10"]
		SysCapSupported := colValues["1.0.8802.1.1.2.1.4.1.1.11"]
		SysCapEnabled := colValues["1.0.8802.1.1.2.1.4.1.1.12"]

		var strVal string
		floatVal, _ := ChassisIdSubtype.ToFloat64()
		remote.ChassisIdSubtype = int(floatVal)

		if remote.ChassisIdSubtype == 4 {
			newVal, _ := enrichment.FormatValue(ChassisId, "mac_address")
			strVal, _ = newVal.ToString()
			remote.ChassisId = strVal
		} else {
			strVal, _ = ChassisId.ToString()
			remote.ChassisId = strVal
		}

		floatVal, _ = PortIdSubtype.ToFloat64()
		remote.PortIdSubType = int(floatVal)

		strVal, _ = PortId.ToString()
		remote.PortId = strVal

		strVal, _ = PortDesc.ToString()
		remote.PortDesc = strVal

		strVal, _ = SysName.ToString()
		remote.SysName = strVal

		strVal, _ = SysDesc.ToString()
		remote.SysDesc = strVal

		strVal, _ = SysCapSupported.ToString()
		remote.SysCapSupported = strVal

		strVal, _ = SysCapEnabled.ToString()
		remote.SysCapEnabled = strVal

		remotes = append(remotes, remote)
	}

	for _, remote := range remotes {
		log.Infof("\t->")
		log.Infof("\t\t TimeMark: %d", remote.TimeMark)
		log.Infof("\t\t LocalPortNum: %d", remote.LocalPortNum)
		log.Infof("\t\t Index: %d", remote.Index)
		log.Infof("\t\t ChassisIdSubtype: %s (%d)", common.ChassisIdSubtypeMap[remote.ChassisIdSubtype], remote.ChassisIdSubtype)
		log.Infof("\t\t ChassisId: %s", remote.ChassisId)
		log.Infof("\t\t PortIdSubType: %s (%d)", common.PortIdSubTypeMap[remote.PortIdSubType], remote.PortIdSubType)
		log.Infof("\t\t PortId: %s", remote.PortId)
		log.Infof("\t\t PortDesc: %s", remote.PortDesc)
		log.Infof("\t\t SysName: %s", remote.SysName)
		log.Infof("\t\t SysDesc: %s", remote.SysDesc)
		log.Infof("\t\t SysCapSupported: %s", remote.SysCapSupported)
		log.Infof("\t\t SysCapEnabled: %s", remote.SysCapEnabled)
	}
}

func (dc *DiscoveryCollector) collectColumnsOids(columns []string, session session.Session) valuestore.ColumnResultValuesType {
	// fetch column values
	oids := make(map[string]string, len(columns))
	for _, value := range columns {
		oids[value] = value
	}
	log.Infof("session2: %+v", session)
	columnValues, err := fetch.FetchColumnOidsWithBatching(session, oids, dc.config.OidBatchSize, 10, fetch.UseGetNext)
	if err != nil {
		log.Errorf("error FetchColumnOidsWithBatching: %s", err)
		return nil
	}
	return columnValues
}
