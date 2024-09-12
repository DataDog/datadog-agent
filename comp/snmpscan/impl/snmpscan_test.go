//go:build test

package snmpscanimpl

import (
	"testing"

	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer/demultiplexerimpl"
	"github.com/DataDog/datadog-agent/comp/core"
	snmpscan "github.com/DataDog/datadog-agent/comp/snmpscan/def"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"
)

func TestSnmpScanComp(t *testing.T) {
	deps := fxutil.Test[snmpscan.Requires](
		t,
		fx.Supply(core.BundleParams{}),
		demultiplexerimpl.MockModule(),
		core.MockBundle(),
	)
	snmpScanner, err := NewComponent(deps)
	assert.NoError(t, err)

	snmpConnection := gosnmp.Default
	snmpConnection.LocalAddr = "127.0.0.1"
	snmpConnection.Port = 0

	err = snmpScanner.Comp.RunDeviceScan(snmpConnection, "default")
	assert.ErrorContains(t, err, "&GoSNMP.Conn is missing. Provide a connection or use Connect()")

	err = snmpScanner.Comp.RunDeviceScan(snmpConnection, "default")
	assert.ErrorContains(t, err, "&GoSNMP.Conn is missing. Provide a connection or use Connect()")
}
