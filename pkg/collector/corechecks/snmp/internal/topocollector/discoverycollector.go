package topocollector

import (
	"encoding/json"
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/common"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/checkconfig"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/fetch"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/report"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/session"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/topograph"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/valuestore"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
)

var graphMutex sync.RWMutex

// DiscoveryCollector TODO
type DiscoveryCollector struct {
	sender   aggregator.Sender
	hostname string
	config   *checkconfig.CheckConfig
}

// NewDiscoveryCollector TODO
func NewDiscoveryCollector(sender aggregator.Sender, hostname string, config *checkconfig.CheckConfig) *DiscoveryCollector {
	return &DiscoveryCollector{
		sender:   sender,
		hostname: hostname,
		config:   config,
	}
}

// Collect TODO
func (dc *DiscoveryCollector) Collect() {
	log.Debug("Collector: collect")
	log.Debugf("Config: %+v", dc.config)
	session, err := session.NewGosnmpSession(dc.config)
	log.Debugf("session: %+v", session)
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

	log.Debug("=== lldpLocPortTable\t\t ===")
	// INDEX { lldpLocPortNum }
	columns := []string{
		"1.0.8802.1.1.2.1.3.7.1.2", // lldpLocPortIdSubtype
		"1.0.8802.1.1.2.1.3.7.1.3", // lldpLocPortID
		"1.0.8802.1.1.2.1.4.2.1.4", // lldpLocPortDesc
	}
	columnValues := dc.collectColumnsOids(columns, session)
	lldpLocPortID := columnValues["1.0.8802.1.1.2.1.3.7.1.3"]

	var locPorts []common.LldpLocPort
	for fullIndex, value := range lldpLocPortID {
		localPortNum, _ := strconv.Atoi(fullIndex)
		portIDStr, _ := value.ToString()
		var portIDType int

		if strings.HasPrefix(portIDStr, "0x") && len(portIDStr) == 14 {
			// TODO: need better way to detect the portId type
			newValue, _ := report.FormatValue(value, "mac_address")
			portIDStr, _ = newValue.ToString()
			portIDType = 3 // macAddress
		}
		locPort := common.LldpLocPort{
			PortNum:       localPortNum,
			PortIDSubType: portIDType,
			PortID:        portIDStr,
		}
		locPorts = append(locPorts, locPort)
	}
	for _, locPort := range locPorts {
		log.Debugf("\t->")
		log.Debugf("\t\t PortNum: %d", locPort.PortNum)
		log.Debugf("\t\t PortIDSubType: %d", locPort.PortIDSubType)
		log.Debugf("\t\t PortID: %s", locPort.PortID)
		log.Debugf("\t\t PortDesc: %s", locPort.PortDesc)
	}

	log.Debugf("=== lldpRemManAddrTable\t ===")
	// INDEX { lldpRemTimeMark, lldpRemLocalPortNum, lldpRemIndex, lldpRemManAddrSubtype, lldpRemManAddr }
	// lldpRemManAddrSubtype: ipv4(1), ipv6(2), etc see more here: http://www.mibdepot.com/cgi-bin/getmib3.cgi?win=mib_a&i=1&n=LLDP-MIB&r=cisco&f=LLDP-MIB-V1SMI.my&v=v1&t=tab&o=lldpRemManAddrSubtype
	columns = []string{
		"1.0.8802.1.1.2.1.4.2.1.3", // lldpRemManAddrIfSubtype unknown(1), ifIndex(2), systemPortNumber(3)
		"1.0.8802.1.1.2.1.4.2.1.4", // lldpRemManAddrIfId
		"1.0.8802.1.1.2.1.4.2.1.5", // lldpRemManAddrOID
	}
	columnValues = dc.collectColumnsOids(columns, session)

	ifSubType := columnValues["1.0.8802.1.1.2.1.4.2.1.3"]

	var remoteMans []common.LldpRemoteManagement
	for fullIndex := range ifSubType {
		indexes := strings.Split(fullIndex, ".")
		timeMark, _ := strconv.Atoi(indexes[0])
		localPortNum, _ := strconv.Atoi(indexes[1])
		index, _ := strconv.Atoi(indexes[2])
		manAddrSubtype, _ := strconv.Atoi(indexes[3])
		var manAddr string
		if manAddrSubtype == 1 || manAddrSubtype == 2 { // ipv4, ipv6
			var ipv6buf []byte
			// TODO: first byte is 4 for ipv6 and 16 for ipv6
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
		log.Debugf("\t->")
		log.Debugf("\t\t TimeMark: %d", remoteMan.TimeMark)
		log.Debugf("\t\t LocalPortNum: %d", remoteMan.LocalPortNum)
		log.Debugf("\t\t Index: %d", remoteMan.Index)
		log.Debugf("\t\t ManAddrSubtype: %s (%d)", common.RemManAddrSubtype[remoteMan.ManAddrSubtype], remoteMan.ManAddrSubtype)
		log.Debugf("\t\t manAddr: %s", remoteMan.ManAddr)
	}

	log.Debugf("=== lldpRemTable ===")
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

		ChassisIDSubtype := colValues["1.0.8802.1.1.2.1.4.1.1.4"]
		ChassisID := colValues["1.0.8802.1.1.2.1.4.1.1.5"]
		PortIDSubtype := colValues["1.0.8802.1.1.2.1.4.1.1.6"]
		PortID := colValues["1.0.8802.1.1.2.1.4.1.1.7"]
		PortDesc := colValues["1.0.8802.1.1.2.1.4.1.1.8"]
		SysName := colValues["1.0.8802.1.1.2.1.4.1.1.9"]
		SysDesc := colValues["1.0.8802.1.1.2.1.4.1.1.10"]
		SysCapSupported := colValues["1.0.8802.1.1.2.1.4.1.1.11"]
		SysCapEnabled := colValues["1.0.8802.1.1.2.1.4.1.1.12"]

		var strVal string
		floatVal, _ := ChassisIDSubtype.ToFloat64()
		remote.ChassisIDSubtype = int(floatVal)

		if remote.ChassisIDSubtype == 4 {
			newVal, _ := report.FormatValue(ChassisID, "mac_address")
			strVal, _ = newVal.ToString()
			remote.ChassisID = strVal
		} else {
			strVal, _ = ChassisID.ToString()
			remote.ChassisID = strVal
		}

		floatVal, _ = PortIDSubtype.ToFloat64()
		remote.PortIDSubType = int(floatVal)

		if remote.PortIDSubType == 3 {
			newVal, _ := report.FormatValue(PortID, "mac_address")
			strVal, _ = newVal.ToString()
			remote.PortID = strVal
		} else {
			strVal, _ = PortID.ToString()
			remote.PortID = strVal
		}

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

		remoteMan, err := findRemote(remoteMans, remote)
		if err != nil {
			log.Debugf("\t\t Remote not found for %+v", remote)
		} else {
			remote.RemoteManagement = remoteMan
		}
		localPort, err := findLocPort(locPorts, remote.LocalPortNum)
		if err != nil {
			log.Debugf("\t\t Local port not found for %+v", remote)
		} else {
			remote.LocalPort = localPort
		}

		remotes = append(remotes, remote)
	}

	for _, remote := range remotes {
		log.Debugf("\t->")
		log.Debugf("\t\t TimeMark: %d", remote.TimeMark)
		log.Debugf("\t\t LocalPortNum: %d", remote.LocalPortNum)
		log.Debugf("\t\t Index: %d", remote.Index)
		log.Debugf("\t\t ChassisIDSubtype: %s (%d)", common.ChassisIDSubtypeMap[remote.ChassisIDSubtype], remote.ChassisIDSubtype)
		log.Debugf("\t\t ChassisID: %s", remote.ChassisID)
		log.Debugf("\t\t PortIDSubType: %s (%d)", common.PortIDSubTypeMap[remote.PortIDSubType], remote.PortIDSubType)
		log.Debugf("\t\t PortID: %s", remote.PortID)
		log.Debugf("\t\t PortDesc: %s", remote.PortDesc)
		log.Debugf("\t\t SysName: %s", remote.SysName)
		log.Debugf("\t\t SysDesc: %s", remote.SysDesc)
		log.Debugf("\t\t SysCapSupported: %s", remote.SysCapSupported)
		log.Debugf("\t\t SysCapEnabled: %s", remote.SysCapEnabled)
		if remote.RemoteManagement != nil {
			log.Debugf("\t\t ManAddr: %s", remote.RemoteManagement.ManAddr)
		}
		if remote.LocalPort != nil {
			log.Debugf("\t\t LocalPort.PortID: %s", remote.LocalPort.PortID)
		}
	}

	sysName, err := FetchSysName(session)
	if err != nil {
		log.Errorf("Error fetching sysName: %s", err)
		return
	}
	payload := buildPayload(remotes, "agent_hostname", dc.config.IPAddress, sysName)
	payloadBytes, err := json.MarshalIndent(payload, "", "    ")
	if err != nil {
		log.Errorf("Error marshalling device metadata: %s", err)
		return
	}
	log.Debugf("topology payload | %s", string(payloadBytes))

	dc.writeToFile(payloadBytes)

	dc.graph()
}

func (dc *DiscoveryCollector) graph() {
	graphMutex.Lock()
	defer graphMutex.Unlock()
	topograph.GraphTopology()
}

func (dc *DiscoveryCollector) writeToFile(payloadBytes []byte) {
	graphMutex.Lock()
	defer graphMutex.Unlock()
	fileName := dc.config.IPAddress
	folderName := "/tmp/topology"
	filePath := folderName + "/" + fileName + ".json"
	err := os.MkdirAll("/tmp/topology", os.ModePerm)
	if err != nil {
		log.Errorf("Error creating topology folder: %s", err)
		return
	}
	err = os.WriteFile(filePath, payloadBytes, 0644)
	if err != nil {
		log.Errorf("Error writing to file: %s", err)
		return
	}
	log.Debugf("Payload written to file: %s", fileName)
}

func findRemote(mans []common.LldpRemoteManagement, remote common.LldpRemote) (*common.LldpRemoteManagement, error) {
	for _, remoteMan := range mans {
		if remoteMan.LocalPortNum == remote.LocalPortNum && remoteMan.Index == remote.Index {
			return &remoteMan, nil
		}
	}
	return nil, fmt.Errorf("not found")
}

func findLocPort(locPorts []common.LldpLocPort, locPortNum int) (*common.LldpLocPort, error) {
	for _, locPort := range locPorts {
		if locPort.PortNum == locPortNum {
			return &locPort, nil
		}
	}
	return nil, fmt.Errorf("not found")
}

func (dc *DiscoveryCollector) collectColumnsOids(columns []string, session session.Session) valuestore.ColumnResultValuesType {
	// fetch column values
	oids := make(map[string]string, len(columns))
	for _, value := range columns {
		oids[value] = value
	}
	columnValues, err := fetch.DoFetchColumnOidsWithBatching(session, oids, dc.config.OidBatchSize, 10, fetch.UseGetNext)
	if err != nil {
		log.Errorf("error DoFetchColumnOidsWithBatching: %s", err)
		return nil
	}
	return columnValues
}

// FetchSysName TODO
func FetchSysName(session session.Session) (string, error) {
	sysName := "1.3.6.1.2.1.1.5.0"
	result, err := session.Get([]string{sysName}) // sysName
	if err != nil {
		return "", fmt.Errorf("cannot get sysobjectid: %s", err)
	}
	if len(result.Variables) != 1 {
		return "", fmt.Errorf("expected 1 value, but got %d: variables=%v", len(result.Variables), result.Variables)
	}
	pduVar := result.Variables[0]
	oid, value, err := valuestore.GetResultValueFromPDU(pduVar)
	if err != nil {
		return "", fmt.Errorf("error getting value from pdu: %s", err)
	}
	if oid != sysName {
		return "", fmt.Errorf("expect `%s` OID but got `%s` OID with value `%v`", sysName, oid, value)
	}
	strValue, err := value.ToString()
	if err != nil {
		return "", fmt.Errorf("error converting value (%#v) to string : %v", value, err)
	}
	return strValue, err
}
