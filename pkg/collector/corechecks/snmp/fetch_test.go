package snmp

import (
	"fmt"
	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_fetchColumnOids(t *testing.T) {
	session := &mockSession{}

	bulkPacket := gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{
			{
				Name:  "1.1.1.1",
				Type:  gosnmp.TimeTicks,
				Value: 11,
			},
			{
				Name:  "1.1.2.1",
				Type:  gosnmp.TimeTicks,
				Value: 21,
			},
			{
				Name:  "1.1.1.2",
				Type:  gosnmp.TimeTicks,
				Value: 12,
			},
			{
				Name:  "1.1.2.2",
				Type:  gosnmp.TimeTicks,
				Value: 22,
			},
			{
				Name:  "1.1.1.3",
				Type:  gosnmp.TimeTicks,
				Value: 13,
			},
			{
				Name:  "1.1.3.1",
				Type:  gosnmp.TimeTicks,
				Value: 31,
			},
		},
	}
	bulkPacket2 := gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{
			{
				Name:  "1.1.1.4",
				Type:  gosnmp.TimeTicks,
				Value: 14,
			},
			{
				Name:  "1.1.1.5",
				Type:  gosnmp.TimeTicks,
				Value: 15,
			},
		},
	}
	bulkPacket3 := gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{
			{
				Name:  "1.1.3.1",
				Type:  gosnmp.TimeTicks,
				Value: 34,
			},
		},
	}
	session.On("GetBulk", []string{"1.1.1", "1.1.2"}).Return(&bulkPacket, nil)
	session.On("GetBulk", []string{"1.1.1.3"}).Return(&bulkPacket2, nil)
	session.On("GetBulk", []string{"1.1.1.5"}).Return(&bulkPacket3, nil)

	oids := map[string]string{"1.1.1": "1.1.1", "1.1.2": "1.1.2"}

	columnValues, err := fetchColumnOidsWithBatching(session, oids, 100)
	assert.Nil(t, err)

	expectedColumnValues := columnResultValuesType{
		"1.1.1": {
			"1": snmpValueType{value: float64(11)},
			"2": snmpValueType{value: float64(12)},
			"3": snmpValueType{value: float64(13)},
			"4": snmpValueType{value: float64(14)},
			"5": snmpValueType{value: float64(15)},
		},
		"1.1.2": {
			"1": snmpValueType{value: float64(21)},
			"2": snmpValueType{value: float64(22)},
		},
	}
	assert.Equal(t, expectedColumnValues, columnValues)
}

func Test_fetchColumnOidsBatch(t *testing.T) {
	session := &mockSession{}

	bulkPacket := gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{
			{
				Name:  "1.1.1.1",
				Type:  gosnmp.TimeTicks,
				Value: 11,
			},
			{
				Name:  "1.1.2.1",
				Type:  gosnmp.TimeTicks,
				Value: 21,
			},
			{
				Name:  "1.1.1.2",
				Type:  gosnmp.TimeTicks,
				Value: 12,
			},
			{
				Name:  "1.1.2.2",
				Type:  gosnmp.TimeTicks,
				Value: 22,
			},
			{
				Name:  "1.1.1.3",
				Type:  gosnmp.TimeTicks,
				Value: 13,
			},
			{
				Name:  "1.1.9.1",
				Type:  gosnmp.TimeTicks,
				Value: 31,
			},
		},
	}

	bulkPacket2 := gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{
			{
				Name:  "1.1.3.1",
				Type:  gosnmp.TimeTicks,
				Value: 31,
			},
			{
				Name:  "1.1.3.2",
				Type:  gosnmp.TimeTicks,
				Value: 32,
			},
			{
				Name:  "1.1.9.1",
				Type:  gosnmp.TimeTicks,
				Value: 31,
			},
		},
	}

	bulkPacket3 := gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{
			{
				Name:  "1.1.1.4",
				Type:  gosnmp.TimeTicks,
				Value: 14,
			},
			{
				Name:  "1.1.1.5",
				Type:  gosnmp.TimeTicks,
				Value: 15,
			},
		},
	}
	bulkPacket4 := gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{
			{
				Name:  "1.1.3.1",
				Type:  gosnmp.TimeTicks,
				Value: 34,
			},
		},
	}
	// First bulk iteration with two batches with batch size 2
	session.On("GetBulk", []string{"1.1.1", "1.1.2"}).Return(&bulkPacket, nil)
	session.On("GetBulk", []string{"1.1.3"}).Return(&bulkPacket2, nil)

	// Second bulk iteration
	session.On("GetBulk", []string{"1.1.1.3"}).Return(&bulkPacket3, nil)

	// Third bulk iteration
	session.On("GetBulk", []string{"1.1.1.5"}).Return(&bulkPacket4, nil)

	oids := map[string]string{"1.1.1": "1.1.1", "1.1.2": "1.1.2"}

	columnValues, err := fetchColumnOidsWithBatching(session, oids, 2)
	assert.Nil(t, err)

	expectedColumnValues := columnResultValuesType{
		"1.1.1": {
			"1": snmpValueType{value: float64(11)},
			"2": snmpValueType{value: float64(12)},
			"3": snmpValueType{value: float64(13)},
			"4": snmpValueType{value: float64(14)},
			"5": snmpValueType{value: float64(15)},
		},
		"1.1.2": {
			"1": snmpValueType{value: float64(21)},
			"2": snmpValueType{value: float64(22)},
		},
	}
	assert.Equal(t, expectedColumnValues, columnValues)
}

func Test_fetchOidBatchSize(t *testing.T) {
	session := &mockSession{}

	getPacket1 := gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{
			{
				Name:  "1.1.1.1.0",
				Type:  gosnmp.Gauge32,
				Value: 10,
			},
			{
				Name:  "1.1.1.2.0",
				Type:  gosnmp.Gauge32,
				Value: 20,
			},
		},
	}

	getPacket2 := gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{
			{
				Name:  "1.1.1.3.0",
				Type:  gosnmp.Gauge32,
				Value: 30,
			},
			{
				Name:  "1.1.1.4.0",
				Type:  gosnmp.Gauge32,
				Value: 40,
			},
		},
	}

	getPacket3 := gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{
			{
				Name:  "1.1.1.5.0",
				Type:  gosnmp.Gauge32,
				Value: 50,
			},
			{
				Name:  "1.1.1.6.0",
				Type:  gosnmp.Gauge32,
				Value: 60,
			},
		},
	}

	session.On("Get", []string{"1.1.1.1.0", "1.1.1.2.0"}).Return(&getPacket1, nil)
	session.On("Get", []string{"1.1.1.3.0", "1.1.1.4.0"}).Return(&getPacket2, nil)
	session.On("Get", []string{"1.1.1.5.0", "1.1.1.6.0"}).Return(&getPacket3, nil)

	oids := []string{"1.1.1.1.0", "1.1.1.2.0", "1.1.1.3.0", "1.1.1.4.0", "1.1.1.5.0", "1.1.1.6.0"}

	columnValues, err := fetchScalarOidsWithBatching(session, oids, 2)
	assert.Nil(t, err)

	expectedColumnValues := scalarResultValuesType{
		"1.1.1.1.0": {value: float64(10)},
		"1.1.1.2.0": {value: float64(20)},
		"1.1.1.3.0": {value: float64(30)},
		"1.1.1.4.0": {value: float64(40)},
		"1.1.1.5.0": {value: float64(50)},
		"1.1.1.6.0": {value: float64(60)},
	}
	assert.Equal(t, expectedColumnValues, columnValues)
}

func Test_fetchOidBatchSize_zeroSizeError(t *testing.T) {
	session := &mockSession{}

	oids := []string{"1.1.1.1.0", "1.1.1.2.0", "1.1.1.3.0", "1.1.1.4.0", "1.1.1.5.0", "1.1.1.6.0"}
	columnValues, err := fetchScalarOidsWithBatching(session, oids, 0)

	assert.EqualError(t, err, "failed to create oid batches: batch size must be positive. invalid size: 0")
	assert.Nil(t, columnValues)
}

func Test_fetchOidBatchSize_fetchError(t *testing.T) {
	session := &mockSession{}

	session.On("Get", []string{"1.1.1.1.0", "1.1.1.2.0"}).Return(&gosnmp.SnmpPacket{}, fmt.Errorf("my error"))

	oids := []string{"1.1.1.1.0", "1.1.1.2.0", "1.1.1.3.0", "1.1.1.4.0", "1.1.1.5.0", "1.1.1.6.0"}
	columnValues, err := fetchScalarOidsWithBatching(session, oids, 2)

	assert.EqualError(t, err, "failed to fetch scalar oids: error getting oids: my error")
	assert.Nil(t, columnValues)
}

func Test_fetchScalarOids_retry(t *testing.T) {
	session := &mockSession{}

	getPacket := gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{
			{
				Name:  "1.1.1.1.0",
				Type:  gosnmp.Gauge32,
				Value: 10,
			},
			{
				Name:  "1.1.1.2",
				Type:  gosnmp.NoSuchInstance,
				Value: 20,
			},
			{
				Name:  ".1.1.1.3", // test `.` prefix is trimmed
				Type:  gosnmp.NoSuchObject,
				Value: 30,
			},
			{
				Name:  "1.1.1.4.0",
				Type:  gosnmp.NoSuchInstance,
				Value: 40,
			},
		},
	}
	retryGetPacket := gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{
			{
				Name:  "1.1.1.2.0",
				Type:  gosnmp.Gauge32,
				Value: 20,
			},
			{
				Name:  "1.1.1.3.0",
				Type:  gosnmp.Gauge32,
				Value: 30,
			},
		},
	}

	session.On("Get", []string{"1.1.1.1.0", "1.1.1.2", "1.1.1.3", "1.1.1.4.0"}).Return(&getPacket, nil)
	session.On("Get", []string{"1.1.1.2.0", "1.1.1.3.0"}).Return(&retryGetPacket, nil)

	oids := []string{"1.1.1.1.0", "1.1.1.2", "1.1.1.3", "1.1.1.4.0"}

	columnValues, err := fetchScalarOids(session, oids)
	assert.Nil(t, err)

	expectedColumnValues := scalarResultValuesType{
		"1.1.1.1.0": {value: float64(10)},
		"1.1.1.2":   {value: float64(20)},
		"1.1.1.3":   {value: float64(30)},
	}
	assert.Equal(t, expectedColumnValues, columnValues)
}

func Test_fetchValues_errors(t *testing.T) {
	tests := []struct {
		name          string
		config        snmpConfig
		bulkPacket    gosnmp.SnmpPacket
		expectedError error
	}{
		{
			name: "invalid batch size",
			config: snmpConfig{
				oidConfig: oidConfig{
					scalarOids: []string{"1.1", "1.2"},
				},
			},
			expectedError: fmt.Errorf("failed to fetch scalar oids with batching: failed to create oid batches: batch size must be positive. invalid size: 0"),
		},
		{
			name: "get fetch error",
			config: snmpConfig{
				oidBatchSize: 10,
				oidConfig: oidConfig{
					scalarOids: []string{"1.1", "2.2"},
				},
			},
			expectedError: fmt.Errorf("failed to fetch scalar oids with batching: failed to fetch scalar oids: error getting oids: get error"),
		},
		{
			name: "bulk fetch error",
			config: snmpConfig{
				oidBatchSize: 10,
				oidConfig: oidConfig{
					scalarOids: []string{},
					columnOids: []string{"1.1", "2.2"},
				},
			},
			expectedError: fmt.Errorf("failed to fetch oids with batching: failed to fetch column oids: GetBulk failed: bulk error"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			session := &mockSession{}
			session.On("Get", []string{"1.1", "2.2"}).Return(&gosnmp.SnmpPacket{}, fmt.Errorf("get error"))
			session.On("GetBulk", []string{"1.1", "2.2"}).Return(&gosnmp.SnmpPacket{}, fmt.Errorf("bulk error"))

			_, err := fetchValues(session, tt.config)

			assert.Equal(t, tt.expectedError, err)
		})
	}
}
