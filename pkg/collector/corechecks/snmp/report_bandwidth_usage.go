package snmp

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var bandwidthMetricNameToUsage = map[string]string{
	"ifHCInOctets":  "ifBandwidthInUsage",
	"ifHCOutOctets": "ifBandwidthOutUsage",
}

var ifHighSpeedOID = "1.3.6.1.2.1.31.1.1.1.15"

func (ms *metricSender) trySendBandwidthUsageMetric(symbol symbolConfig, fullIndex string, values *resultValueStore, tags []string) {
	err := ms.sendBandwidthUsageMetric(symbol, fullIndex, values, tags)
	if err != nil {
		log.Debugf("failed to send bandwidth usage metric: %s", err)
	}
}

/* sendBandwidthUsageMetric evaluate and report input/output bandwidth usage.
   If any of `ifHCInOctets`, `ifHCOutOctets`  or `ifHighSpeed` is missing then bandwidth will not be reported.

   Bandwidth usage is:

   interface[In|Out]Octets(t+dt) - interface[In|Out]Octets(t)
   ----------------------------------------------------------
                   dt*interfaceSpeed

   Given:
   * ifHCInOctets: the total number of octets received on the interface.
   * ifHCOutOctets: The total number of octets transmitted out of the interface.
   * ifHighSpeed: An estimate of the interface's current bandwidth in Mb/s (10^6 bits
                  per second). It is constant in time, can be overwritten by the system admin.
                  It is the total available bandwidth.
   Bandwidth usage is evaluated as: ifHC[In|Out]Octets/ifHighSpeed and reported as *rate*
*/
func (ms *metricSender) sendBandwidthUsageMetric(symbol symbolConfig, fullIndex string, values *resultValueStore, tags []string) error {
	usageName, ok := bandwidthMetricNameToUsage[symbol.Name]
	if !ok {
		return nil
	}

	ifHighSpeedValues, err := values.getColumnValues(ifHighSpeedOID)
	if err != nil {
		return fmt.Errorf("bandwidth usage: missing `ifHighSpeed` metric, skipping metric. fullIndex=%s", fullIndex)
	}

	metricValues, err := values.getColumnValues(symbol.OID)
	if err != nil {
		return fmt.Errorf("bandwidth usage: missing `%s` metric, skipping this row. fullIndex=%s", symbol.Name, fullIndex)
	}

	octetsValue, ok := metricValues[fullIndex]
	if !ok {
		return fmt.Errorf("bandwidth usage: missing value for `%s` metric, skipping this row. fullIndex=%s", symbol.Name, fullIndex)
	}

	ifHighSpeedValue, ok := ifHighSpeedValues[fullIndex]
	if !ok {
		return fmt.Errorf("bandwidth usage: missing value for `ifHighSpeed`, skipping this row. fullIndex=%s", fullIndex)
	}

	ifHighSpeedFloatValue, err := ifHighSpeedValue.toFloat64()
	if err != nil {
		return fmt.Errorf("failed to convert ifHighSpeedValue to float64: %s", err)
	}
	if ifHighSpeedFloatValue == 0.0 {
		return fmt.Errorf("bandwidth usage: zero or invalid value for ifHighSpeed, skipping this row. fullIndex=%s, ifHighSpeedValue=%#v", fullIndex, ifHighSpeedValue)
	}
	octetsFloatValue, err := octetsValue.toFloat64()
	if err != nil {
		return fmt.Errorf("failed to convert octetsValue to float64: %s", err)
	}
	usageValue := ((octetsFloatValue * 8) / (ifHighSpeedFloatValue * (1e6))) * 100.0

	ms.sendMetric(usageName+".rate", snmpValueType{"counter", usageValue}, tags, "counter", metricsConfigOption{}, nil)
	return nil
}
