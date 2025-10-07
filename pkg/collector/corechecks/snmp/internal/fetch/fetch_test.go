// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fetch

import (
	"bufio"
	"bytes"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/checkconfig"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/session"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/valuestore"
)

func Test_fetchColumnOids(t *testing.T) {
	sess := session.CreateMockSession()

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
	sess.On("GetBulk", []string{"1.1.1", "1.1.2"}, checkconfig.DefaultBulkMaxRepetitions).Return(&bulkPacket, nil)
	sess.On("GetBulk", []string{"1.1.1.3"}, checkconfig.DefaultBulkMaxRepetitions).Return(&bulkPacket2, nil)
	sess.On("GetBulk", []string{"1.1.1.5"}, checkconfig.DefaultBulkMaxRepetitions).Return(&bulkPacket3, nil)

	oids := []string{"1.1.1", "1.1.2"}

	batchSizeOptimizer := newOidBatchSizeOptimizer(snmpGetBulk, 100)

	columnValues, err := fetchColumnOidsWithBatching(sess, oids, batchSizeOptimizer, checkconfig.DefaultBulkMaxRepetitions, useGetBulk)
	assert.Nil(t, err)

	expectedColumnValues := valuestore.ColumnResultValuesType{
		"1.1.1": {
			"1": valuestore.ResultValue{Value: float64(11)},
			"2": valuestore.ResultValue{Value: float64(12)},
			"3": valuestore.ResultValue{Value: float64(13)},
			"4": valuestore.ResultValue{Value: float64(14)},
			"5": valuestore.ResultValue{Value: float64(15)},
		},
		"1.1.2": {
			"1": valuestore.ResultValue{Value: float64(21)},
			"2": valuestore.ResultValue{Value: float64(22)},
		},
	}
	assert.Equal(t, expectedColumnValues, columnValues)
}

func Test_fetchColumnOidsBatch_usingGetBulk(t *testing.T) {
	sess := session.CreateMockSession()

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
	// First bulk iteration with two batches with batch size 2
	sess.On("GetBulk", []string{"1.1.1", "1.1.2"}, checkconfig.DefaultBulkMaxRepetitions).Return(&bulkPacket, nil)

	// Second bulk iteration
	sess.On("GetBulk", []string{"1.1.1.3"}, checkconfig.DefaultBulkMaxRepetitions).Return(&bulkPacket2, nil)

	// Third bulk iteration
	sess.On("GetBulk", []string{"1.1.1.5"}, checkconfig.DefaultBulkMaxRepetitions).Return(&bulkPacket3, nil)

	oids := []string{"1.1.1", "1.1.2"}

	batchSizeOptimizer := newOidBatchSizeOptimizer(snmpGetBulk, 2)

	columnValues, err := fetchColumnOidsWithBatching(sess, oids, batchSizeOptimizer, 10, useGetBulk)
	assert.Nil(t, err)

	expectedColumnValues := valuestore.ColumnResultValuesType{
		"1.1.1": {
			"1": valuestore.ResultValue{Value: float64(11)},
			"2": valuestore.ResultValue{Value: float64(12)},
			"3": valuestore.ResultValue{Value: float64(13)},
			"4": valuestore.ResultValue{Value: float64(14)},
			"5": valuestore.ResultValue{Value: float64(15)},
		},
		"1.1.2": {
			"1": valuestore.ResultValue{Value: float64(21)},
			"2": valuestore.ResultValue{Value: float64(22)},
		},
	}
	assert.Equal(t, expectedColumnValues, columnValues)
}

func Test_fetchColumnOidsBatch_usingGetNext(t *testing.T) {
	sess := session.CreateMockSession()
	sess.Version = gosnmp.Version1

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
		},
	}

	bulkPacket2 := gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{
			{
				Name:  "1.1.1.2",
				Type:  gosnmp.TimeTicks,
				Value: 12,
			},
			{
				Name:  "1.1.9.1",
				Type:  gosnmp.TimeTicks,
				Value: 91,
			},
		},
	}
	bulkPacket3 := gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{
			{
				Name:  "1.1.9.2",
				Type:  gosnmp.TimeTicks,
				Value: 91,
			},
		},
	}

	secondBatchPacket1 := gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{
			{
				Name:  "1.1.3.1",
				Type:  gosnmp.TimeTicks,
				Value: 31,
			},
		},
	}

	secondBatchPacket2 := gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{
			{
				Name:  "1.1.9.1",
				Type:  gosnmp.TimeTicks,
				Value: 91,
			},
		},
	}

	// First bulk iteration with two batches with batch size 2
	sess.On("GetNext", []string{"1.1.1", "1.1.2"}).Return(&bulkPacket, nil)

	// Second bulk iteration
	sess.On("GetNext", []string{"1.1.1.1", "1.1.2.1"}).Return(&bulkPacket2, nil)

	// Third bulk iteration
	sess.On("GetNext", []string{"1.1.1.2"}).Return(&bulkPacket3, nil)

	// Second batch
	sess.On("GetNext", []string{"1.1.3"}).Return(&secondBatchPacket1, nil)
	sess.On("GetNext", []string{"1.1.3.1"}).Return(&secondBatchPacket2, nil)

	oids := []string{"1.1.1", "1.1.2", "1.1.3"}

	batchSizeOptimizers := NewOidBatchSizeOptimizers(2)

	columnValues, err := fetchColumnOidsWithBatching(sess, oids, batchSizeOptimizers.snmpGetBulkOptimizer, 10, useGetBulk)
	assert.EqualError(t, err, "failed to fetch column oids: GetBulk not supported in SNMP v1")
	assert.Nil(t, columnValues)

	columnValues, err = fetchColumnOidsWithBatching(sess, oids, batchSizeOptimizers.snmpGetNextOptimizer, 10, useGetNext)
	assert.Nil(t, err)

	expectedColumnValues := valuestore.ColumnResultValuesType{
		"1.1.1": {
			"1": valuestore.ResultValue{Value: float64(11)},
			"2": valuestore.ResultValue{Value: float64(12)},
		},
		"1.1.2": {
			"1": valuestore.ResultValue{Value: float64(21)},
		},
		"1.1.3": {
			"1": valuestore.ResultValue{Value: float64(31)},
		},
	}
	assert.Equal(t, expectedColumnValues, columnValues)
}

func Test_fetchColumnOidsBatch_usingGetBulkAndGetNextFallback(t *testing.T) {
	sess := session.CreateMockSession()
	// When using snmp v2+, we will try GetBulk first and fallback using GetNext
	sess.Version = gosnmp.Version2c

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
		},
	}

	bulkPacket2 := gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{
			{
				Name:  "1.1.1.2",
				Type:  gosnmp.TimeTicks,
				Value: 12,
			},
			{
				Name:  "1.1.9.1",
				Type:  gosnmp.TimeTicks,
				Value: 91,
			},
		},
	}
	bulkPacket3 := gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{
			{
				Name:  "1.1.9.2",
				Type:  gosnmp.TimeTicks,
				Value: 91,
			},
		},
	}

	secondBatchPacket1 := gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{
			{
				Name:  "1.1.3.1",
				Type:  gosnmp.TimeTicks,
				Value: 31,
			},
		},
	}

	secondBatchPacket2 := gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{
			{
				Name:  "1.1.9.1",
				Type:  gosnmp.TimeTicks,
				Value: 91,
			},
		},
	}

	sess.On("GetBulk", []string{"1.1.1", "1.1.2"}, checkconfig.DefaultBulkMaxRepetitions).Return(&gosnmp.SnmpPacket{}, fmt.Errorf("bulk error"))
	sess.On("GetBulk", []string{"1.1.1"}, checkconfig.DefaultBulkMaxRepetitions).Return(&gosnmp.SnmpPacket{}, fmt.Errorf("bulk error"))

	// First batch
	sess.On("GetNext", []string{"1.1.1", "1.1.2"}).Return(&bulkPacket, nil)
	sess.On("GetNext", []string{"1.1.1.1", "1.1.2.1"}).Return(&bulkPacket2, nil)
	sess.On("GetNext", []string{"1.1.1.2"}).Return(&bulkPacket3, nil)

	// Second batch
	sess.On("GetNext", []string{"1.1.3"}).Return(&secondBatchPacket1, nil)
	sess.On("GetNext", []string{"1.1.3.1"}).Return(&secondBatchPacket2, nil)

	columnOIDs := []string{"1.1.1", "1.1.2", "1.1.3"}

	batchSizeOptimizers := NewOidBatchSizeOptimizers(2)

	columnValues, err := Fetch(sess, nil, columnOIDs, batchSizeOptimizers, checkconfig.DefaultBulkMaxRepetitions)
	assert.Nil(t, err)

	expectedColumnValues := &valuestore.ResultValueStore{
		ScalarValues: valuestore.ScalarResultValuesType{},
		ColumnValues: valuestore.ColumnResultValuesType{
			"1.1.1": {
				"1": valuestore.ResultValue{Value: float64(11)},
				"2": valuestore.ResultValue{Value: float64(12)},
			},
			"1.1.2": {
				"1": valuestore.ResultValue{Value: float64(21)},
			},
			"1.1.3": {
				"1": valuestore.ResultValue{Value: float64(31)},
			},
		},
	}
	assert.Equal(t, expectedColumnValues, columnValues)
}

func Test_fetchOidBatchSize(t *testing.T) {
	sess := session.CreateMockSession()

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

	sess.On("Get", []string{"1.1.1.1.0", "1.1.1.2.0"}).Return(&getPacket1, nil)
	sess.On("Get", []string{"1.1.1.3.0", "1.1.1.4.0"}).Return(&getPacket2, nil)
	sess.On("Get", []string{"1.1.1.5.0", "1.1.1.6.0"}).Return(&getPacket3, nil)

	oids := []string{"1.1.1.1.0", "1.1.1.2.0", "1.1.1.3.0", "1.1.1.4.0", "1.1.1.5.0", "1.1.1.6.0"}

	batchSizeOptimizer := newOidBatchSizeOptimizer(snmpGet, 2)

	columnValues, err := fetchScalarOidsWithBatching(sess, oids, batchSizeOptimizer)
	assert.Nil(t, err)

	expectedColumnValues := valuestore.ScalarResultValuesType{
		"1.1.1.1.0": {Value: float64(10)},
		"1.1.1.2.0": {Value: float64(20)},
		"1.1.1.3.0": {Value: float64(30)},
		"1.1.1.4.0": {Value: float64(40)},
		"1.1.1.5.0": {Value: float64(50)},
		"1.1.1.6.0": {Value: float64(60)},
	}
	assert.Equal(t, expectedColumnValues, columnValues)
}

func Test_fetchOidBatchSize_v1NoSuchName(t *testing.T) {
	sess := session.CreateMockSession()
	sess.Version = gosnmp.Version1

	getPacket1 := gosnmp.SnmpPacket{
		Error:      gosnmp.NoSuchName,
		ErrorIndex: 1,
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

	getPacket1b := gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{
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

	sess.On("Get", []string{"1.1.1.1.0", "1.1.1.2.0"}).Return(&getPacket1, nil)
	sess.On("Get", []string{"1.1.1.2.0"}).Return(&getPacket1b, nil)
	sess.On("Get", []string{"1.1.1.3.0", "1.1.1.4.0"}).Return(&getPacket2, nil)
	sess.On("Get", []string{"1.1.1.5.0", "1.1.1.6.0"}).Return(&getPacket3, nil)

	oids := []string{"1.1.1.1.0", "1.1.1.2.0", "1.1.1.3.0", "1.1.1.4.0", "1.1.1.5.0", "1.1.1.6.0"}
	origOids := slices.Clone(oids)

	batchSizeOptimizer := newOidBatchSizeOptimizer(snmpGet, 2)

	columnValues, err := fetchScalarOidsWithBatching(sess, oids, batchSizeOptimizer)
	assert.Nil(t, err)

	expectedColumnValues := valuestore.ScalarResultValuesType{
		"1.1.1.2.0": {Value: float64(20)},
		"1.1.1.3.0": {Value: float64(30)},
		"1.1.1.4.0": {Value: float64(40)},
		"1.1.1.5.0": {Value: float64(50)},
		"1.1.1.6.0": {Value: float64(60)},
	}
	assert.Equal(t, expectedColumnValues, columnValues)
	assert.Equal(t, origOids, oids)
}

func Test_fetchOidBatchSize_zeroSizeError(t *testing.T) {
	sess := session.CreateMockSession()

	oids := []string{"1.1.1.1.0", "1.1.1.2.0", "1.1.1.3.0", "1.1.1.4.0", "1.1.1.5.0", "1.1.1.6.0"}
	batchSizeOptimizer := newOidBatchSizeOptimizer(snmpGet, 0)
	columnValues, err := fetchScalarOidsWithBatching(sess, oids, batchSizeOptimizer)

	assert.EqualError(t, err, "failed to create oid batches: batch size must be positive. invalid size: 0")
	assert.Nil(t, columnValues)
}

func Test_fetchOidBatchSize_fetchError(t *testing.T) {
	sess := session.CreateMockSession()

	sess.On("Get", []string{"1.1.1.1.0", "1.1.1.2.0"}).Return(&gosnmp.SnmpPacket{}, fmt.Errorf("my error"))
	sess.On("Get", []string{"1.1.1.1.0"}).Return(&gosnmp.SnmpPacket{}, fmt.Errorf("my error"))

	oids := []string{"1.1.1.1.0", "1.1.1.2.0", "1.1.1.3.0", "1.1.1.4.0", "1.1.1.5.0", "1.1.1.6.0"}
	batchSizeOptimizer := newOidBatchSizeOptimizer(snmpGet, 2)
	columnValues, err := fetchScalarOidsWithBatching(sess, oids, batchSizeOptimizer)

	assert.EqualError(t, err, "failed to fetch scalar oids: fetch scalar: failed getting oids `[1.1.1.1.0]` using Get: my error")
	assert.Nil(t, columnValues)
}

func Test_fetchScalarOids_retry(t *testing.T) {
	sess := session.CreateMockSession()

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
			{
				Name:  "1.1.1.5",
				Type:  gosnmp.Null,
				Value: 50,
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
			{
				Name:  "1.1.1.5.0",
				Type:  gosnmp.Gauge32,
				Value: 50,
			},
		},
	}

	sess.On("Get", []string{"1.1.1.1.0", "1.1.1.2", "1.1.1.3", "1.1.1.4.0", "1.1.1.5"}).Return(&getPacket, nil)
	sess.On("Get", []string{"1.1.1.2.0", "1.1.1.3.0", "1.1.1.5.0"}).Return(&retryGetPacket, nil)

	oids := []string{"1.1.1.1.0", "1.1.1.2", "1.1.1.3", "1.1.1.4.0", "1.1.1.5"}

	columnValues, err := fetchScalarOids(sess, oids)
	assert.Nil(t, err)

	expectedColumnValues := valuestore.ScalarResultValuesType{
		"1.1.1.1.0": {Value: float64(10)},
		"1.1.1.2":   {Value: float64(20)},
		"1.1.1.3":   {Value: float64(30)},
		"1.1.1.5":   {Value: float64(50)},
	}
	assert.Equal(t, expectedColumnValues, columnValues)
}

func Test_fetchScalarOids_v1NoSuchName(t *testing.T) {
	sess := session.CreateMockSession()
	sess.Version = gosnmp.Version1

	getPacket := gosnmp.SnmpPacket{
		Error:      gosnmp.NoSuchName,
		ErrorIndex: 2,
		Variables: []gosnmp.SnmpPDU{
			{
				Name: "1.1.1.1.0",
				Type: gosnmp.Null,
			},
			{
				Name: "1.1.1.2.0",
				Type: gosnmp.Null,
			},
			{
				Name: "1.1.1.3.0",
				Type: gosnmp.Null,
			},
			{
				Name: "1.1.1.4.0",
				Type: gosnmp.Null,
			},
		},
	}

	getPacket2 := gosnmp.SnmpPacket{
		Error:      gosnmp.NoSuchName,
		ErrorIndex: 3,
		Variables: []gosnmp.SnmpPDU{
			{
				Name: "1.1.1.1.0",
				Type: gosnmp.Null,
			},
			{
				Name: "1.1.1.3.0",
				Type: gosnmp.Null,
			},
			{
				Name: "1.1.1.4.0",
				Type: gosnmp.Null,
			},
		},
	}

	getPacket3 := gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{
			{
				Name:  "1.1.1.1.0",
				Type:  gosnmp.Gauge32,
				Value: 10,
			},
			{
				Name:  "1.1.1.3.0",
				Type:  gosnmp.Gauge32,
				Value: 30,
			},
		},
	}

	sess.On("Get", []string{"1.1.1.1.0", "1.1.1.2", "1.1.1.3", "1.1.1.4.0"}).Return(&getPacket, nil)
	sess.On("Get", []string{"1.1.1.1.0", "1.1.1.3", "1.1.1.4.0"}).Return(&getPacket2, nil)
	sess.On("Get", []string{"1.1.1.1.0", "1.1.1.3"}).Return(&getPacket3, nil)

	oids := []string{"1.1.1.1.0", "1.1.1.2", "1.1.1.3", "1.1.1.4.0"}
	origOids := slices.Clone(oids)

	columnValues, err := fetchScalarOids(sess, oids)
	assert.Nil(t, err)

	expectedColumnValues := valuestore.ScalarResultValuesType{
		"1.1.1.1.0": {Value: float64(10)},
		"1.1.1.3.0": {Value: float64(30)},
	}
	assert.Equal(t, expectedColumnValues, columnValues)
	assert.Equal(t, origOids, oids)
}

func Test_fetchScalarOids_v1NoSuchName_noValidOidsLeft(t *testing.T) {
	sess := session.CreateMockSession()
	sess.Version = gosnmp.Version1

	getPacket := gosnmp.SnmpPacket{
		Error:      gosnmp.NoSuchName,
		ErrorIndex: 1,
		Variables: []gosnmp.SnmpPDU{
			{
				Name: "1.1.1.1.0",
				Type: gosnmp.Null,
			},
		},
	}

	sess.On("Get", []string{"1.1.1.1.0"}).Return(&getPacket, nil)

	oids := []string{"1.1.1.1.0"}

	columnValues, err := fetchScalarOids(sess, oids)
	assert.Nil(t, err)

	expectedColumnValues := valuestore.ScalarResultValuesType{}
	assert.Equal(t, expectedColumnValues, columnValues)
}

func Test_fetchScalarOids_v1NoSuchName_errorIndexTooHigh(t *testing.T) {
	sess := session.CreateMockSession()
	sess.Version = gosnmp.Version1

	getPacket := gosnmp.SnmpPacket{
		Error:      gosnmp.NoSuchName,
		ErrorIndex: 3,
		Variables: []gosnmp.SnmpPDU{
			{
				Name: "1.1.1.1.0",
				Type: gosnmp.Null,
			},
			{
				Name: "1.1.1.2.0",
				Type: gosnmp.Null,
			},
		},
	}

	sess.On("Get", []string{"1.1.1.1.0", "1.1.1.2"}).Return(&getPacket, nil)

	oids := []string{"1.1.1.1.0", "1.1.1.2"}

	columnValues, err := fetchScalarOids(sess, oids)
	assert.EqualError(t, err, "invalid ErrorIndex `3` when fetching oids `[1.1.1.1.0 1.1.1.2]`")
	assert.Nil(t, columnValues)
}

func Test_fetchScalarOids_v1NoSuchName_errorIndexTooLow(t *testing.T) {
	sess := session.CreateMockSession()
	sess.Version = gosnmp.Version1

	getPacket := gosnmp.SnmpPacket{
		Error:      gosnmp.NoSuchName,
		ErrorIndex: 0,
		Variables: []gosnmp.SnmpPDU{
			{
				Name: "1.1.1.1.0",
				Type: gosnmp.Null,
			},
			{
				Name: "1.1.1.2.0",
				Type: gosnmp.Null,
			},
		},
	}

	sess.On("Get", []string{"1.1.1.1.0", "1.1.1.2"}).Return(&getPacket, nil)

	oids := []string{"1.1.1.1.0", "1.1.1.2"}

	columnValues, err := fetchScalarOids(sess, oids)
	assert.EqualError(t, err, "invalid ErrorIndex `0` when fetching oids `[1.1.1.1.0 1.1.1.2]`")
	assert.Nil(t, columnValues)
}

func Test_fetchValues_errors(t *testing.T) {
	tests := []struct {
		name          string
		maxReps       uint32
		batchSize     int
		ScalarOIDs    []string
		ColumnOIDs    []string
		bulkPacket    gosnmp.SnmpPacket
		expectedError error
	}{
		{
			name:          "invalid batch size",
			maxReps:       checkconfig.DefaultBulkMaxRepetitions,
			ScalarOIDs:    []string{"1.1", "1.2"},
			expectedError: fmt.Errorf("failed to fetch scalar oids with batching: failed to create oid batches: batch size must be positive. invalid size: 0"),
		},
		{
			name:          "get fetch error",
			maxReps:       checkconfig.DefaultBulkMaxRepetitions,
			batchSize:     10,
			ScalarOIDs:    []string{"1.1", "2.2"},
			expectedError: fmt.Errorf("failed to fetch scalar oids with batching: failed to fetch scalar oids: fetch scalar: failed getting oids `[1.1]` using Get: get error"),
		},
		{
			name:          "bulk fetch error",
			maxReps:       checkconfig.DefaultBulkMaxRepetitions,
			batchSize:     10,
			ScalarOIDs:    []string{},
			ColumnOIDs:    []string{"1.1", "2.2"},
			expectedError: fmt.Errorf("failed to fetch oids with GetNext batching: failed to fetch column oids: fetch column: failed getting oids `[1.1]` using GetNext: getnext error"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sess := session.CreateMockSession()
			sess.On("Get", []string{"1.1", "2.2"}).Return(&gosnmp.SnmpPacket{}, fmt.Errorf("get error"))
			sess.On("Get", []string{"1.1"}).Return(&gosnmp.SnmpPacket{}, fmt.Errorf("get error"))
			sess.On("GetBulk", []string{"1.1", "2.2"}, checkconfig.DefaultBulkMaxRepetitions).Return(&gosnmp.SnmpPacket{}, fmt.Errorf("bulk error"))
			sess.On("GetBulk", []string{"1.1"}, checkconfig.DefaultBulkMaxRepetitions).Return(&gosnmp.SnmpPacket{}, fmt.Errorf("bulk error"))
			sess.On("GetNext", []string{"1.1", "2.2"}).Return(&gosnmp.SnmpPacket{}, fmt.Errorf("getnext error"))
			sess.On("GetNext", []string{"1.1"}).Return(&gosnmp.SnmpPacket{}, fmt.Errorf("getnext error"))

			batchSizeOptimizers := NewOidBatchSizeOptimizers(tt.batchSize)

			_, err := Fetch(sess, tt.ScalarOIDs, tt.ColumnOIDs, batchSizeOptimizers, tt.maxReps)

			assert.Equal(t, tt.expectedError, err)
		})
	}
}

func Test_fetchColumnOids_alreadyProcessed(t *testing.T) {
	var b bytes.Buffer
	w := bufio.NewWriter(&b)
	l, err := log.LoggerFromWriterWithMinLevelAndFormat(w, log.DebugLvl, "[%LEVEL] %FuncShort: %Msg")
	require.NoError(t, err)
	log.SetupLogger(l, "debug")

	sess := session.CreateMockSession()

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
				Name:  "1.1.2.3",
				Type:  gosnmp.TimeTicks,
				Value: 23,
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
				Name:  "1.1.2.4",
				Type:  gosnmp.TimeTicks,
				Value: 24,
			},
			{
				Name:  "1.1.1.5",
				Type:  gosnmp.TimeTicks,
				Value: 15,
			},
			{
				Name:  "1.1.2.5",
				Type:  gosnmp.TimeTicks,
				Value: 25,
			},
		},
	}
	bulkPacket3 := gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{
			{
				// this OID is already process, we won't try to fetch it again
				Name:  "1.1.1.4",
				Type:  gosnmp.TimeTicks,
				Value: 14,
			},
			{
				// not processed yet
				Name:  "1.1.2.6",
				Type:  gosnmp.TimeTicks,
				Value: 26,
			},
			{
				// this OID is already process, we won't try to fetch it again
				Name:  "1.1.1.5",
				Type:  gosnmp.TimeTicks,
				Value: 15,
			},
			{
				// this OID is already process, we won't try to fetch it again
				Name:  "1.1.2.5",
				Type:  gosnmp.TimeTicks,
				Value: 25,
			},
		},
	}
	sess.On("GetBulk", []string{"1.1.1", "1.1.2"}, checkconfig.DefaultBulkMaxRepetitions).Return(&bulkPacket, nil)
	sess.On("GetBulk", []string{"1.1.1.3", "1.1.2.3"}, checkconfig.DefaultBulkMaxRepetitions).Return(&bulkPacket2, nil)
	sess.On("GetBulk", []string{"1.1.1.5", "1.1.2.5"}, checkconfig.DefaultBulkMaxRepetitions).Return(&bulkPacket3, nil)

	oids := []string{"1.1.1", "1.1.2"}

	batchSizeOptimizer := newOidBatchSizeOptimizer(snmpGetBulk, 100)

	columnValues, err := fetchColumnOidsWithBatching(sess, oids, batchSizeOptimizer, checkconfig.DefaultBulkMaxRepetitions, useGetBulk)
	assert.Nil(t, err)

	expectedColumnValues := valuestore.ColumnResultValuesType{
		"1.1.1": {
			"1": valuestore.ResultValue{Value: float64(11)},
			"2": valuestore.ResultValue{Value: float64(12)},
			"3": valuestore.ResultValue{Value: float64(13)},
			"4": valuestore.ResultValue{Value: float64(14)},
			"5": valuestore.ResultValue{Value: float64(15)},
		},
		"1.1.2": {
			"1": valuestore.ResultValue{Value: float64(21)},
			"2": valuestore.ResultValue{Value: float64(22)},
			"3": valuestore.ResultValue{Value: float64(23)},
			"4": valuestore.ResultValue{Value: float64(24)},
			"5": valuestore.ResultValue{Value: float64(25)},
			"6": valuestore.ResultValue{Value: float64(26)},
		},
	}
	assert.Equal(t, expectedColumnValues, columnValues)

	_ = w.Flush()
	logs := b.String()
	assert.Nil(t, err)

	assert.Equal(t, 1, strings.Count(logs, "[DEBUG] fetchColumnOids: fetch column: OID already processed: 1.1.1.5"), logs)
	assert.Equal(t, 1, strings.Count(logs, "[DEBUG] fetchColumnOids: fetch column: OID already processed: 1.1.2.5"), logs)
}

func Test_batchSizeOptimizers_fetchErrors(t *testing.T) {
	oidBatchSize := 4
	batchSizeOptimizers := NewOidBatchSizeOptimizers(oidBatchSize)
	lastRefreshTs := batchSizeOptimizers.lastRefreshTs

	scalarVariable1 := gosnmp.SnmpPDU{Name: "1.1.1.1.0", Type: gosnmp.Gauge32, Value: 10}
	scalarVariable2 := gosnmp.SnmpPDU{Name: "1.1.1.2.0", Type: gosnmp.Gauge32, Value: 20}
	scalarVariable3 := gosnmp.SnmpPDU{Name: "1.1.1.3.0", Type: gosnmp.Gauge32, Value: 30}
	scalarVariable4 := gosnmp.SnmpPDU{Name: "1.1.1.4.0", Type: gosnmp.Gauge32, Value: 40}
	bulkVariable1 := gosnmp.SnmpPDU{Name: "1.1.1.1", Type: gosnmp.TimeTicks, Value: 11}
	bulkVariable2 := gosnmp.SnmpPDU{Name: "1.1.2.1", Type: gosnmp.TimeTicks, Value: 21}
	bulkVariable3 := gosnmp.SnmpPDU{Name: "1.1.3.1", Type: gosnmp.TimeTicks, Value: 31}
	bulkVariable4 := gosnmp.SnmpPDU{Name: "1.1.4.1", Type: gosnmp.TimeTicks, Value: 41}
	bulkVariable5 := gosnmp.SnmpPDU{Name: "1.1.5.1", Type: gosnmp.TimeTicks, Value: 51}

	expectedValues := &valuestore.ResultValueStore{
		ScalarValues: valuestore.ScalarResultValuesType{
			"1.1.1.1.0": {Value: float64(10)},
			"1.1.1.2.0": {Value: float64(20)},
			"1.1.1.3.0": {Value: float64(30)},
			"1.1.1.4.0": {Value: float64(40)},
		},
		ColumnValues: valuestore.ColumnResultValuesType{
			"1.1.1": {
				"1": valuestore.ResultValue{Value: float64(11)},
			},
			"1.1.2": {
				"1": valuestore.ResultValue{Value: float64(21)},
			},
			"1.1.3": {
				"1": valuestore.ResultValue{Value: float64(31)},
			},
			"1.1.4": {
				"1": valuestore.ResultValue{Value: float64(41)},
			},
		},
	}

	/*
		Fetch 1:
			- Get and GetBulk fail with batch size 4.
			- Get and GetBulk batch sizes are now 2.
			- Get and GetBulk are retried with batch size 2 and it successes.
			- Get and GetBulk batch size are increased to 3 for the next fetch.
	*/
	sess := session.CreateMockSession()

	scalarPacket1 := gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{scalarVariable1, scalarVariable2},
	}
	scalarPacket2 := gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{scalarVariable3, scalarVariable4},
	}
	bulkPacket1 := gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{
			bulkVariable1, bulkVariable2,
			bulkVariable2, bulkVariable3,
		},
	}
	bulkPacket2 := gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{
			bulkVariable3, bulkVariable4,
			bulkVariable4, bulkVariable5,
		},
	}

	sess.On("Get", []string{"1.1.1.1.0", "1.1.1.2.0", "1.1.1.3.0", "1.1.1.4.0"}).Return(&gosnmp.SnmpPacket{}, fmt.Errorf("my error"))
	sess.On("Get", []string{"1.1.1.1.0", "1.1.1.2.0"}).Return(&scalarPacket1, nil)
	sess.On("Get", []string{"1.1.1.3.0", "1.1.1.4.0"}).Return(&scalarPacket2, nil)

	sess.On("GetBulk", []string{"1.1.1", "1.1.2", "1.1.3", "1.1.4"}, checkconfig.DefaultBulkMaxRepetitions).Return(&gosnmp.SnmpPacket{}, fmt.Errorf("bulk error"))
	sess.On("GetBulk", []string{"1.1.1", "1.1.2"}, checkconfig.DefaultBulkMaxRepetitions).Return(&bulkPacket1, nil)
	sess.On("GetBulk", []string{"1.1.3", "1.1.4"}, checkconfig.DefaultBulkMaxRepetitions).Return(&bulkPacket2, nil)

	scalarOids := []string{"1.1.1.1.0", "1.1.1.2.0", "1.1.1.3.0", "1.1.1.4.0"}
	columnOids := []string{"1.1.1", "1.1.2", "1.1.3", "1.1.4"}

	values, err := Fetch(sess, scalarOids, columnOids, batchSizeOptimizers, checkconfig.DefaultBulkMaxRepetitions)
	assert.Nil(t, err)
	assert.Equal(t, expectedValues, values)

	assert.Equal(t, &OidBatchSizeOptimizers{
		snmpGetOptimizer: &oidBatchSizeOptimizer{
			snmpOperation:       snmpGet,
			configBatchSize:     4,
			batchSize:           3,
			failuresByBatchSize: map[int]int{4: 1},
		},
		snmpGetBulkOptimizer: &oidBatchSizeOptimizer{
			snmpOperation:       snmpGetBulk,
			configBatchSize:     4,
			batchSize:           3,
			failuresByBatchSize: map[int]int{4: 1},
		},
		snmpGetNextOptimizer: &oidBatchSizeOptimizer{
			snmpOperation:       snmpGetNext,
			configBatchSize:     4,
			batchSize:           4,
			failuresByBatchSize: map[int]int{},
		},
		lastRefreshTs: lastRefreshTs,
	}, batchSizeOptimizers)

	/*
		Fetch 2:
			- Get successes with batch size 3, its batch size is now 4 for the next fetch.
			- GetBulk fails for batch size 3, it retries with batch size 1, but still fails. We stop with GetBulk.
			- Since GetBulk failed, GetNext is used with batch size 4.
			- GetNext successes with batch size 4, its batch size is not increased (max is 4).
	*/
	sess = session.CreateMockSession()

	scalarPacket1 = gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{scalarVariable1, scalarVariable2, scalarVariable3},
	}
	scalarPacket2 = gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{scalarVariable4},
	}
	nextPacket1 := gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{
			bulkVariable1, bulkVariable2, bulkVariable3, bulkVariable4,
		},
	}

	sess.On("Get", []string{"1.1.1.1.0", "1.1.1.2.0", "1.1.1.3.0"}).Return(&scalarPacket1, nil)
	sess.On("Get", []string{"1.1.1.4.0"}).Return(&scalarPacket2, nil)

	sess.On("GetBulk", []string{"1.1.1", "1.1.2", "1.1.3"}, checkconfig.DefaultBulkMaxRepetitions).Return(&gosnmp.SnmpPacket{}, fmt.Errorf("bulk error"))
	sess.On("GetBulk", []string{"1.1.1"}, checkconfig.DefaultBulkMaxRepetitions).Return(&gosnmp.SnmpPacket{}, fmt.Errorf("bulk error"))

	sess.On("GetNext", []string{"1.1.1", "1.1.2", "1.1.3", "1.1.4"}).Return(&nextPacket1, nil)
	sess.On("GetNext", []string{"1.1.1.1", "1.1.2.1", "1.1.3.1", "1.1.4.1"}).Return(&gosnmp.SnmpPacket{}, nil)

	values, err = Fetch(sess, scalarOids, columnOids, batchSizeOptimizers, checkconfig.DefaultBulkMaxRepetitions)
	assert.Nil(t, err)
	assert.Equal(t, expectedValues, values)

	assert.Equal(t, &OidBatchSizeOptimizers{
		snmpGetOptimizer: &oidBatchSizeOptimizer{
			snmpOperation:       snmpGet,
			configBatchSize:     4,
			batchSize:           4,
			failuresByBatchSize: map[int]int{4: 1},
		},
		snmpGetBulkOptimizer: &oidBatchSizeOptimizer{
			snmpOperation:       snmpGetBulk,
			configBatchSize:     4,
			batchSize:           1,
			failuresByBatchSize: map[int]int{4: 1, 3: 1, 1: 1},
		},
		snmpGetNextOptimizer: &oidBatchSizeOptimizer{
			snmpOperation:       snmpGetNext,
			configBatchSize:     4,
			batchSize:           4,
			failuresByBatchSize: map[int]int{},
		},
		lastRefreshTs: lastRefreshTs,
	}, batchSizeOptimizers)

	/*
		Fetch 3:
			- Get fails with batch size 4, and it successes with batch size 2. Its batch size is increased to 3 for the next fetch.
			- GetBulk fails with batch size 1.
			- GetNext successes with batch size 4.
	*/
	sess = session.CreateMockSession()

	scalarPacket1 = gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{scalarVariable1, scalarVariable2},
	}
	scalarPacket2 = gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{scalarVariable3, scalarVariable4},
	}
	nextPacket1 = gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{
			bulkVariable1, bulkVariable2, bulkVariable3, bulkVariable4,
		},
	}

	sess.On("Get", []string{"1.1.1.1.0", "1.1.1.2.0", "1.1.1.3.0", "1.1.1.4.0"}).Return(&gosnmp.SnmpPacket{}, fmt.Errorf("my error"))
	sess.On("Get", []string{"1.1.1.1.0", "1.1.1.2.0"}).Return(&scalarPacket1, nil)
	sess.On("Get", []string{"1.1.1.3.0", "1.1.1.4.0"}).Return(&scalarPacket2, nil)

	sess.On("GetBulk", []string{"1.1.1"}, checkconfig.DefaultBulkMaxRepetitions).Return(&gosnmp.SnmpPacket{}, fmt.Errorf("bulk error"))

	sess.On("GetNext", []string{"1.1.1", "1.1.2", "1.1.3", "1.1.4"}).Return(&nextPacket1, nil)
	sess.On("GetNext", []string{"1.1.1.1", "1.1.2.1", "1.1.3.1", "1.1.4.1"}).Return(&gosnmp.SnmpPacket{}, nil)

	values, err = Fetch(sess, scalarOids, columnOids, batchSizeOptimizers, checkconfig.DefaultBulkMaxRepetitions)
	assert.Nil(t, err)
	assert.Equal(t, expectedValues, values)

	assert.Equal(t, &OidBatchSizeOptimizers{
		snmpGetOptimizer: &oidBatchSizeOptimizer{
			snmpOperation:       snmpGet,
			configBatchSize:     4,
			batchSize:           3,
			failuresByBatchSize: map[int]int{4: 2},
		},
		snmpGetBulkOptimizer: &oidBatchSizeOptimizer{
			snmpOperation:       snmpGetBulk,
			configBatchSize:     4,
			batchSize:           1,
			failuresByBatchSize: map[int]int{4: 1, 3: 1, 1: 2},
		},
		snmpGetNextOptimizer: &oidBatchSizeOptimizer{
			snmpOperation:       snmpGetNext,
			configBatchSize:     4,
			batchSize:           4,
			failuresByBatchSize: map[int]int{},
		},
		lastRefreshTs: lastRefreshTs,
	}, batchSizeOptimizers)

	/*
		Fetch 4:
			- Get successes with batch size 3, but its batch size is not increased to 4 since it already failed 2 times.
			- GetBulk fails with batch size 1.
			- GetNext successes with batch size 4.
	*/
	sess = session.CreateMockSession()

	scalarPacket1 = gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{scalarVariable1, scalarVariable2, scalarVariable3},
	}
	scalarPacket2 = gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{scalarVariable4},
	}

	sess.On("Get", []string{"1.1.1.1.0", "1.1.1.2.0", "1.1.1.3.0"}).Return(&scalarPacket1, nil)
	sess.On("Get", []string{"1.1.1.4.0"}).Return(&scalarPacket2, nil)

	sess.On("GetBulk", []string{"1.1.1"}, checkconfig.DefaultBulkMaxRepetitions).Return(&gosnmp.SnmpPacket{}, fmt.Errorf("bulk error"))

	sess.On("GetNext", []string{"1.1.1", "1.1.2", "1.1.3", "1.1.4"}).Return(&nextPacket1, nil)
	sess.On("GetNext", []string{"1.1.1.1", "1.1.2.1", "1.1.3.1", "1.1.4.1"}).Return(&gosnmp.SnmpPacket{}, nil)

	values, err = Fetch(sess, scalarOids, columnOids, batchSizeOptimizers, checkconfig.DefaultBulkMaxRepetitions)
	assert.Nil(t, err)
	assert.Equal(t, expectedValues, values)

	assert.Equal(t, &OidBatchSizeOptimizers{
		snmpGetOptimizer: &oidBatchSizeOptimizer{
			snmpOperation:       snmpGet,
			configBatchSize:     4,
			batchSize:           3,
			failuresByBatchSize: map[int]int{4: 2},
		},
		snmpGetBulkOptimizer: &oidBatchSizeOptimizer{
			snmpOperation:       snmpGetBulk,
			configBatchSize:     4,
			batchSize:           1,
			failuresByBatchSize: map[int]int{4: 1, 3: 1, 1: 3},
		},
		snmpGetNextOptimizer: &oidBatchSizeOptimizer{
			snmpOperation:       snmpGetNext,
			configBatchSize:     4,
			batchSize:           4,
			failuresByBatchSize: map[int]int{},
		},
		lastRefreshTs: lastRefreshTs,
	}, batchSizeOptimizers)

	/*
		Fetch 5:
			- Get successes with batch size 3.
			- GetBulk fails with batch size 4.
			- GetNext fails with batch size 4, 2, and 1.
	*/
	sess = session.CreateMockSession()

	scalarPacket1 = gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{scalarVariable1, scalarVariable2, scalarVariable3},
	}
	scalarPacket2 = gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{scalarVariable4},
	}

	sess.On("Get", []string{"1.1.1.1.0", "1.1.1.2.0", "1.1.1.3.0"}).Return(&scalarPacket1, nil)
	sess.On("Get", []string{"1.1.1.4.0"}).Return(&scalarPacket2, nil)

	sess.On("GetBulk", []string{"1.1.1"}, checkconfig.DefaultBulkMaxRepetitions).Return(&gosnmp.SnmpPacket{}, fmt.Errorf("bulk error"))

	sess.On("GetNext", []string{"1.1.1", "1.1.2", "1.1.3", "1.1.4"}).Return(&gosnmp.SnmpPacket{}, fmt.Errorf("next error"))
	sess.On("GetNext", []string{"1.1.1", "1.1.2"}).Return(&gosnmp.SnmpPacket{}, fmt.Errorf("next error"))
	sess.On("GetNext", []string{"1.1.1"}).Return(&gosnmp.SnmpPacket{}, fmt.Errorf("next error"))

	values, err = Fetch(sess, scalarOids, columnOids, batchSizeOptimizers, checkconfig.DefaultBulkMaxRepetitions)
	assert.EqualError(t, err, "failed to fetch oids with GetNext batching: failed to fetch column oids: fetch column: failed getting oids `[1.1.1]` using GetNext: next error")
	assert.Nil(t, values)

	assert.Equal(t, &OidBatchSizeOptimizers{
		snmpGetOptimizer: &oidBatchSizeOptimizer{
			snmpOperation:       snmpGet,
			configBatchSize:     4,
			batchSize:           3,
			failuresByBatchSize: map[int]int{4: 2},
		},
		snmpGetBulkOptimizer: &oidBatchSizeOptimizer{
			snmpOperation:       snmpGetBulk,
			configBatchSize:     4,
			batchSize:           1,
			failuresByBatchSize: map[int]int{4: 1, 3: 1, 1: 4},
		},
		snmpGetNextOptimizer: &oidBatchSizeOptimizer{
			snmpOperation:       snmpGetNext,
			configBatchSize:     4,
			batchSize:           1,
			failuresByBatchSize: map[int]int{4: 1, 2: 1, 1: 1},
		},
		lastRefreshTs: lastRefreshTs,
	}, batchSizeOptimizers)
}

func Test_batchSizeOptimizers_otherErrors(t *testing.T) {
	sess := session.CreateMockSession()
	sess.Version = gosnmp.Version1

	batchSizeOptimizers := NewOidBatchSizeOptimizers(2)
	lastRefreshTs := batchSizeOptimizers.lastRefreshTs

	// Batch size should not be retried when we encounter an other error than a fetchError
	values, err := fetchColumnOidsWithBatching(sess, []string{"1.1.1"}, batchSizeOptimizers.snmpGetBulkOptimizer,
		checkconfig.DefaultBulkMaxRepetitions, useGetBulk)
	assert.EqualError(t, err, "failed to fetch column oids: GetBulk not supported in SNMP v1")
	assert.Nil(t, values)

	assert.Equal(t, &OidBatchSizeOptimizers{
		snmpGetOptimizer: &oidBatchSizeOptimizer{
			snmpOperation:       snmpGet,
			configBatchSize:     2,
			batchSize:           2,
			failuresByBatchSize: map[int]int{},
		},
		snmpGetBulkOptimizer: &oidBatchSizeOptimizer{
			snmpOperation:       snmpGetBulk,
			configBatchSize:     2,
			batchSize:           2,
			failuresByBatchSize: map[int]int{},
		},
		snmpGetNextOptimizer: &oidBatchSizeOptimizer{
			snmpOperation:       snmpGetNext,
			configBatchSize:     2,
			batchSize:           2,
			failuresByBatchSize: map[int]int{},
		},
		lastRefreshTs: lastRefreshTs,
	}, batchSizeOptimizers)
}

func Test_batchSizeOptimizers_areRefreshed(t *testing.T) {
	sess := session.CreateMockSession()

	batchSizeOptimizers := NewOidBatchSizeOptimizers(2)
	batchSizeOptimizers.lastRefreshTs = batchSizeOptimizers.lastRefreshTs.Add(-failuresTimeInterval * 2)

	oldLastRefreshTs := batchSizeOptimizers.lastRefreshTs

	values, err := Fetch(sess, nil, nil, batchSizeOptimizers, checkconfig.DefaultBulkMaxRepetitions)
	assert.Nil(t, err)
	assert.Equal(t, &valuestore.ResultValueStore{
		ScalarValues: valuestore.ScalarResultValuesType{},
		ColumnValues: valuestore.ColumnResultValuesType{},
	}, values)

	assert.True(t, batchSizeOptimizers.lastRefreshTs.After(oldLastRefreshTs))
}
