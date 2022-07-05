package discoverycollector

import (
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/networkdiscovery/common"
	"github.com/DataDog/datadog-agent/pkg/networkdiscovery/config"
	"github.com/DataDog/datadog-agent/pkg/networkdiscovery/fetch"
	"github.com/DataDog/datadog-agent/pkg/networkdiscovery/session"
	"github.com/DataDog/datadog-agent/pkg/networkdiscovery/valuestore"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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
	log.Info("=== lldpRemTable ===")
	// INDEX { lldpRemTimeMark, lldpRemLocalPortNum, lldpRemIndex }
	columns := []string{
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
	columnValues := dc.collectColumnsOids(columns, session)
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

	for fullIndex, colValues := range valuesByIndexByColumn {
		indexes := strings.Split(fullIndex, ".")
		timeMark, _ := strconv.Atoi(indexes[0])
		localPortNum, _ := strconv.Atoi(indexes[1])
		index, _ := strconv.Atoi(indexes[2])

		for columnOid, value := range colValues {
			remote := common.LldpRemote{
				TimeMark:     timeMark,
				LocalPortNum: localPortNum,
				Index:        index,
			}

			remotes = append(remotes, remote)
			strVal, err := value.ToString()

			if err != nil {
				log.Warnf("error converting %s.%s value to string: %s", columnOid, fullIndex, err)
			} else {
				log.Infof("\t%s: %v", fullIndex, strVal)
			}
		}
	}

	log.Info("=== lldpRemManAddrTable\t ===")
	// INDEX { lldpRemTimeMark, lldpRemLocalPortNum, lldpRemIndex, lldpRemManAddrSubtype, lldpRemManAddr }
	columns = []string{
		"1.0.8802.1.1.2.1.4.2.1.3", // lldpRemManAddrIfSubtype
		"1.0.8802.1.1.2.1.4.2.1.4", // lldpRemManAddrIfId
		"1.0.8802.1.1.2.1.4.2.1.5", // lldpRemManAddrOID
	}
	dc.collectColumnsOids(columns, session)
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
