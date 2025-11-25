// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fetch

import (
	"bufio"
	"bytes"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

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

	sess.On("GetBulk", []string{"1.1.1", "1.1.2"}, checkconfig.DefaultBulkMaxRepetitions).Return(&gosnmp.SnmpPacket{}, errors.New("bulk error"))
	sess.On("GetBulk", []string{"1.1.1"}, checkconfig.DefaultBulkMaxRepetitions).Return(&gosnmp.SnmpPacket{}, errors.New("bulk error"))

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

	sess.On("Get", []string{"1.1.1.1.0", "1.1.1.2.0"}).Return(&gosnmp.SnmpPacket{}, errors.New("my error"))
	sess.On("Get", []string{"1.1.1.1.0"}).Return(&gosnmp.SnmpPacket{}, errors.New("my error"))

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
			expectedError: errors.New("failed to fetch scalar oids with batching: failed to create oid batches: batch size must be positive. invalid size: 0"),
		},
		{
			name:          "get fetch error",
			maxReps:       checkconfig.DefaultBulkMaxRepetitions,
			batchSize:     10,
			ScalarOIDs:    []string{"1.1", "2.2"},
			expectedError: errors.New("failed to fetch scalar oids with batching: failed to fetch scalar oids: fetch scalar: failed getting oids `[1.1]` using Get: get error"),
		},
		{
			name:          "bulk fetch error",
			maxReps:       checkconfig.DefaultBulkMaxRepetitions,
			batchSize:     10,
			ScalarOIDs:    []string{},
			ColumnOIDs:    []string{"1.1", "2.2"},
			expectedError: errors.New("failed to fetch oids with GetNext batching: failed to fetch column oids: fetch column: failed getting oids `[1.1]` using GetNext: getnext error"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sess := session.CreateMockSession()
			sess.On("Get", []string{"1.1", "2.2"}).Return(&gosnmp.SnmpPacket{}, errors.New("get error"))
			sess.On("Get", []string{"1.1"}).Return(&gosnmp.SnmpPacket{}, errors.New("get error"))
			sess.On("GetBulk", []string{"1.1", "2.2"}, checkconfig.DefaultBulkMaxRepetitions).Return(&gosnmp.SnmpPacket{}, errors.New("bulk error"))
			sess.On("GetBulk", []string{"1.1"}, checkconfig.DefaultBulkMaxRepetitions).Return(&gosnmp.SnmpPacket{}, errors.New("bulk error"))
			sess.On("GetNext", []string{"1.1", "2.2"}).Return(&gosnmp.SnmpPacket{}, errors.New("getnext error"))
			sess.On("GetNext", []string{"1.1"}).Return(&gosnmp.SnmpPacket{}, errors.New("getnext error"))

			batchSizeOptimizers := NewOidBatchSizeOptimizers(tt.batchSize)

			_, err := Fetch(sess, tt.ScalarOIDs, tt.ColumnOIDs, batchSizeOptimizers, tt.maxReps)

			assert.Equal(t, tt.expectedError, err)
		})
	}
}

func Test_fetchColumnOids_alreadyProcessed(t *testing.T) {
	var b bytes.Buffer
	w := bufio.NewWriter(&b)
	l, err := log.LoggerFromWriterWithMinLevelAndLvlFuncMsgFormat(w, log.DebugLvl)
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

func Test_batchSizeOptimizers(t *testing.T) {
	now := time.Now()

	scalarVariable1 := gosnmp.SnmpPDU{Name: "1.1.1.1.0", Type: gosnmp.Gauge32, Value: 10}
	scalarVariable2 := gosnmp.SnmpPDU{Name: "1.1.1.2.0", Type: gosnmp.Gauge32, Value: 20}
	scalarVariable3 := gosnmp.SnmpPDU{Name: "1.1.1.3.0", Type: gosnmp.Gauge32, Value: 30}
	scalarVariable4 := gosnmp.SnmpPDU{Name: "1.1.1.4.0", Type: gosnmp.Gauge32, Value: 40}
	bulkVariable1 := gosnmp.SnmpPDU{Name: "1.1.1.1", Type: gosnmp.TimeTicks, Value: 11}
	bulkVariable2 := gosnmp.SnmpPDU{Name: "1.1.2.1", Type: gosnmp.TimeTicks, Value: 21}
	bulkVariable3 := gosnmp.SnmpPDU{Name: "1.1.3.1", Type: gosnmp.TimeTicks, Value: 31}
	bulkVariable4 := gosnmp.SnmpPDU{Name: "1.1.4.1", Type: gosnmp.TimeTicks, Value: 41}
	bulkVariable5 := gosnmp.SnmpPDU{Name: "1.1.5.1", Type: gosnmp.TimeTicks, Value: 51}

	tests := []struct {
		name                        string
		sessionFactory              func() session.Session
		scalarOids                  []string
		columnOids                  []string
		batchSizeOptimizers         *OidBatchSizeOptimizers
		expectedValues              *valuestore.ResultValueStore
		expectedError               error
		expectedBatchSizeOptimizers *OidBatchSizeOptimizers
	}{
		{
			name: "batch size is not increased when new batch size already failed too much",
			sessionFactory: func() session.Session {
				sess := session.CreateMockSession()

				scalarPacket1 := gosnmp.SnmpPacket{
					Variables: []gosnmp.SnmpPDU{scalarVariable1, scalarVariable2, scalarVariable3},
				}
				scalarPacket2 := gosnmp.SnmpPacket{
					Variables: []gosnmp.SnmpPDU{scalarVariable4},
				}

				sess.On("Get", []string{"1.1.1.1.0", "1.1.1.2.0", "1.1.1.3.0"}).Return(&scalarPacket1, nil)
				sess.On("Get", []string{"1.1.1.4.0"}).Return(&scalarPacket2, nil)

				return sess
			},
			scalarOids: []string{"1.1.1.1.0", "1.1.1.2.0", "1.1.1.3.0", "1.1.1.4.0"},
			columnOids: []string{},
			batchSizeOptimizers: &OidBatchSizeOptimizers{
				snmpGetOptimizer: &oidBatchSizeOptimizer{
					snmpOperation:           snmpGet,
					configBatchSize:         4,
					batchSize:               3,
					failuresByBatchSize:     map[int]int{4: maxFailuresPerWindow},
					lastSuccessfulBatchSize: 2,
				},
				lastRefreshTs: now,
			},
			expectedValues: &valuestore.ResultValueStore{
				ScalarValues: valuestore.ScalarResultValuesType{
					"1.1.1.1.0": {Value: float64(10)},
					"1.1.1.2.0": {Value: float64(20)},
					"1.1.1.3.0": {Value: float64(30)},
					"1.1.1.4.0": {Value: float64(40)},
				},
				ColumnValues: valuestore.ColumnResultValuesType{},
			},
			expectedError: nil,
			expectedBatchSizeOptimizers: &OidBatchSizeOptimizers{
				snmpGetOptimizer: &oidBatchSizeOptimizer{
					snmpOperation:           snmpGet,
					configBatchSize:         4,
					batchSize:               3,
					failuresByBatchSize:     map[int]int{4: maxFailuresPerWindow},
					lastSuccessfulBatchSize: 3,
				},
				lastRefreshTs: now,
			},
		},
		{
			name: "fetch is retried with lower batch size after fetch error",
			sessionFactory: func() session.Session {
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

				sess.On("Get", []string{"1.1.1.1.0", "1.1.1.2.0", "1.1.1.3.0", "1.1.1.4.0"}).Return(&gosnmp.SnmpPacket{}, errors.New("my error"))
				sess.On("Get", []string{"1.1.1.1.0", "1.1.1.2.0"}).Return(&scalarPacket1, nil)
				sess.On("Get", []string{"1.1.1.3.0", "1.1.1.4.0"}).Return(&scalarPacket2, nil)

				sess.On("GetBulk", []string{"1.1.1", "1.1.2", "1.1.3", "1.1.4"}, checkconfig.DefaultBulkMaxRepetitions).Return(&gosnmp.SnmpPacket{}, errors.New("bulk error"))
				sess.On("GetBulk", []string{"1.1.1", "1.1.2"}, checkconfig.DefaultBulkMaxRepetitions).Return(&bulkPacket1, nil)
				sess.On("GetBulk", []string{"1.1.3", "1.1.4"}, checkconfig.DefaultBulkMaxRepetitions).Return(&bulkPacket2, nil)

				return sess
			},
			scalarOids: []string{"1.1.1.1.0", "1.1.1.2.0", "1.1.1.3.0", "1.1.1.4.0"},
			columnOids: []string{"1.1.1", "1.1.2", "1.1.3", "1.1.4"},
			batchSizeOptimizers: &OidBatchSizeOptimizers{
				snmpGetOptimizer: &oidBatchSizeOptimizer{
					snmpOperation:           snmpGet,
					configBatchSize:         4,
					batchSize:               4,
					failuresByBatchSize:     map[int]int{},
					lastSuccessfulBatchSize: 0,
				},
				snmpGetBulkOptimizer: &oidBatchSizeOptimizer{
					snmpOperation:           snmpGetBulk,
					configBatchSize:         4,
					batchSize:               4,
					failuresByBatchSize:     map[int]int{},
					lastSuccessfulBatchSize: 0,
				},
				snmpGetNextOptimizer: &oidBatchSizeOptimizer{
					snmpOperation:           snmpGetNext,
					configBatchSize:         4,
					batchSize:               4,
					failuresByBatchSize:     map[int]int{},
					lastSuccessfulBatchSize: 0,
				},
				lastRefreshTs: now,
			},
			expectedValues: &valuestore.ResultValueStore{
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
			},
			expectedError: nil,
			expectedBatchSizeOptimizers: &OidBatchSizeOptimizers{
				snmpGetOptimizer: &oidBatchSizeOptimizer{
					snmpOperation:           snmpGet,
					configBatchSize:         4,
					batchSize:               4/onFailureDecreaseFactor + onSuccessIncreaseValue,
					failuresByBatchSize:     map[int]int{4: 1},
					lastSuccessfulBatchSize: 2,
				},
				snmpGetBulkOptimizer: &oidBatchSizeOptimizer{
					snmpOperation:           snmpGetBulk,
					configBatchSize:         4,
					batchSize:               4/onFailureDecreaseFactor + onSuccessIncreaseValue,
					failuresByBatchSize:     map[int]int{4: 1},
					lastSuccessfulBatchSize: 2,
				},
				snmpGetNextOptimizer: &oidBatchSizeOptimizer{
					snmpOperation:           snmpGetNext,
					configBatchSize:         4,
					batchSize:               4,
					failuresByBatchSize:     map[int]int{},
					lastSuccessfulBatchSize: 0,
				},
				lastRefreshTs: now,
			},
		},
		{
			name: "last successful batch size is used after a fetch error with bigger batch size",
			sessionFactory: func() session.Session {
				sess := session.CreateMockSession()

				scalarPacket1 := gosnmp.SnmpPacket{
					Variables: []gosnmp.SnmpPDU{scalarVariable1, scalarVariable2, scalarVariable3},
				}
				scalarPacket2 := gosnmp.SnmpPacket{
					Variables: []gosnmp.SnmpPDU{scalarVariable4},
				}

				sess.On("Get", []string{"1.1.1.1.0", "1.1.1.2.0", "1.1.1.3.0", "1.1.1.4.0"}).Return(&gosnmp.SnmpPacket{}, errors.New("my error"))
				sess.On("Get", []string{"1.1.1.1.0", "1.1.1.2.0", "1.1.1.3.0"}).Return(&scalarPacket1, nil)
				sess.On("Get", []string{"1.1.1.4.0"}).Return(&scalarPacket2, nil)

				return sess
			},
			scalarOids: []string{"1.1.1.1.0", "1.1.1.2.0", "1.1.1.3.0", "1.1.1.4.0"},
			columnOids: []string{},
			batchSizeOptimizers: &OidBatchSizeOptimizers{
				snmpGetOptimizer: &oidBatchSizeOptimizer{
					snmpOperation:           snmpGet,
					configBatchSize:         4,
					batchSize:               4,
					failuresByBatchSize:     map[int]int{},
					lastSuccessfulBatchSize: 3,
				},
				lastRefreshTs: now,
			},
			expectedValues: &valuestore.ResultValueStore{
				ScalarValues: valuestore.ScalarResultValuesType{
					"1.1.1.1.0": {Value: float64(10)},
					"1.1.1.2.0": {Value: float64(20)},
					"1.1.1.3.0": {Value: float64(30)},
					"1.1.1.4.0": {Value: float64(40)},
				},
				ColumnValues: valuestore.ColumnResultValuesType{},
			},
			expectedError: nil,
			expectedBatchSizeOptimizers: &OidBatchSizeOptimizers{
				snmpGetOptimizer: &oidBatchSizeOptimizer{
					snmpOperation:           snmpGet,
					configBatchSize:         4,
					batchSize:               4,
					failuresByBatchSize:     map[int]int{4: 1},
					lastSuccessfulBatchSize: 3,
				},
				lastRefreshTs: now,
			},
		},
		{
			name: "last successful batch size is not used after a fetch error with smaller batch size",
			sessionFactory: func() session.Session {
				sess := session.CreateMockSession()

				scalarPacket1 := gosnmp.SnmpPacket{
					Variables: []gosnmp.SnmpPDU{scalarVariable1},
				}
				scalarPacket2 := gosnmp.SnmpPacket{
					Variables: []gosnmp.SnmpPDU{scalarVariable2},
				}
				scalarPacket3 := gosnmp.SnmpPacket{
					Variables: []gosnmp.SnmpPDU{scalarVariable3},
				}
				scalarPacket4 := gosnmp.SnmpPacket{
					Variables: []gosnmp.SnmpPDU{scalarVariable4},
				}

				sess.On("Get", []string{"1.1.1.1.0", "1.1.1.2.0"}).Return(&gosnmp.SnmpPacket{}, errors.New("my error"))
				sess.On("Get", []string{"1.1.1.1.0"}).Return(&scalarPacket1, nil)
				sess.On("Get", []string{"1.1.1.2.0"}).Return(&scalarPacket2, nil)
				sess.On("Get", []string{"1.1.1.3.0"}).Return(&scalarPacket3, nil)
				sess.On("Get", []string{"1.1.1.4.0"}).Return(&scalarPacket4, nil)

				return sess
			},
			scalarOids: []string{"1.1.1.1.0", "1.1.1.2.0", "1.1.1.3.0", "1.1.1.4.0"},
			columnOids: []string{},
			batchSizeOptimizers: &OidBatchSizeOptimizers{
				snmpGetOptimizer: &oidBatchSizeOptimizer{
					snmpOperation:           snmpGet,
					configBatchSize:         4,
					batchSize:               2,
					failuresByBatchSize:     map[int]int{},
					lastSuccessfulBatchSize: 3,
				},
				lastRefreshTs: now,
			},
			expectedValues: &valuestore.ResultValueStore{
				ScalarValues: valuestore.ScalarResultValuesType{
					"1.1.1.1.0": {Value: float64(10)},
					"1.1.1.2.0": {Value: float64(20)},
					"1.1.1.3.0": {Value: float64(30)},
					"1.1.1.4.0": {Value: float64(40)},
				},
				ColumnValues: valuestore.ColumnResultValuesType{},
			},
			expectedError: nil,
			expectedBatchSizeOptimizers: &OidBatchSizeOptimizers{
				snmpGetOptimizer: &oidBatchSizeOptimizer{
					snmpOperation:           snmpGet,
					configBatchSize:         4,
					batchSize:               2,
					failuresByBatchSize:     map[int]int{2: 1},
					lastSuccessfulBatchSize: 1,
				},
				lastRefreshTs: now,
			},
		},
		{
			name: "fetch with bulk fails and it fallbacks to next",
			sessionFactory: func() session.Session {
				sess := session.CreateMockSession()

				nextPacket1 := gosnmp.SnmpPacket{
					Variables: []gosnmp.SnmpPDU{
						bulkVariable1, bulkVariable2, bulkVariable3, bulkVariable4,
					},
				}

				sess.On("GetBulk", []string{"1.1.1", "1.1.2", "1.1.3"}, checkconfig.DefaultBulkMaxRepetitions).Return(&gosnmp.SnmpPacket{}, errors.New("bulk error"))
				sess.On("GetBulk", []string{"1.1.1", "1.1.2"}, checkconfig.DefaultBulkMaxRepetitions).Return(&gosnmp.SnmpPacket{}, errors.New("bulk error"))
				sess.On("GetBulk", []string{"1.1.1"}, checkconfig.DefaultBulkMaxRepetitions).Return(&gosnmp.SnmpPacket{}, errors.New("bulk error"))

				sess.On("GetNext", []string{"1.1.1", "1.1.2", "1.1.3", "1.1.4"}).Return(&nextPacket1, nil)
				sess.On("GetNext", []string{"1.1.1.1", "1.1.2.1", "1.1.3.1", "1.1.4.1"}).Return(&gosnmp.SnmpPacket{}, nil)

				return sess
			},
			scalarOids: []string{},
			columnOids: []string{"1.1.1", "1.1.2", "1.1.3", "1.1.4"},
			batchSizeOptimizers: &OidBatchSizeOptimizers{
				snmpGetOptimizer: &oidBatchSizeOptimizer{
					snmpOperation:           snmpGet,
					configBatchSize:         4,
					batchSize:               4,
					failuresByBatchSize:     map[int]int{},
					lastSuccessfulBatchSize: 4,
				},
				snmpGetBulkOptimizer: &oidBatchSizeOptimizer{
					snmpOperation:           snmpGetBulk,
					configBatchSize:         4,
					batchSize:               3,
					failuresByBatchSize:     map[int]int{4: 1},
					lastSuccessfulBatchSize: 2,
				},
				snmpGetNextOptimizer: &oidBatchSizeOptimizer{
					snmpOperation:           snmpGetNext,
					configBatchSize:         4,
					batchSize:               4,
					failuresByBatchSize:     map[int]int{},
					lastSuccessfulBatchSize: 0,
				},
				lastRefreshTs: now,
			},
			expectedValues: &valuestore.ResultValueStore{
				ScalarValues: valuestore.ScalarResultValuesType{},
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
			},
			expectedError: nil,
			expectedBatchSizeOptimizers: &OidBatchSizeOptimizers{
				snmpGetOptimizer: &oidBatchSizeOptimizer{
					snmpOperation:           snmpGet,
					configBatchSize:         4,
					batchSize:               4,
					failuresByBatchSize:     map[int]int{},
					lastSuccessfulBatchSize: 4,
				},
				snmpGetBulkOptimizer: &oidBatchSizeOptimizer{
					snmpOperation:           snmpGetBulk,
					configBatchSize:         4,
					batchSize:               1,
					failuresByBatchSize:     map[int]int{4: 1, 3: 1, 2: 1, 1: 1},
					lastSuccessfulBatchSize: 2,
				},
				snmpGetNextOptimizer: &oidBatchSizeOptimizer{
					snmpOperation:           snmpGetNext,
					configBatchSize:         4,
					batchSize:               4,
					failuresByBatchSize:     map[int]int{},
					lastSuccessfulBatchSize: 4,
				},
				lastRefreshTs: now,
			},
		},
		{
			name: "fetches with whathever batch size fail for bulk and next",
			sessionFactory: func() session.Session {
				sess := session.CreateMockSession()

				scalarPacket1 := gosnmp.SnmpPacket{
					Variables: []gosnmp.SnmpPDU{scalarVariable1},
				}

				sess.On("Get", []string{"1.1.1.1.0", "1.1.1.2.0", "1.1.1.3.0", "1.1.1.4.0"}).Return(&scalarPacket1, nil)

				sess.On("GetBulk", []string{"1.1.1", "1.1.2", "1.1.3", "1.1.4"}, checkconfig.DefaultBulkMaxRepetitions).Return(&gosnmp.SnmpPacket{}, errors.New("bulk error"))
				sess.On("GetBulk", []string{"1.1.1", "1.1.2"}, checkconfig.DefaultBulkMaxRepetitions).Return(&gosnmp.SnmpPacket{}, errors.New("bulk error"))
				sess.On("GetBulk", []string{"1.1.1"}, checkconfig.DefaultBulkMaxRepetitions).Return(&gosnmp.SnmpPacket{}, errors.New("bulk error"))

				sess.On("GetNext", []string{"1.1.1", "1.1.2", "1.1.3", "1.1.4"}).Return(&gosnmp.SnmpPacket{}, errors.New("next error"))
				sess.On("GetNext", []string{"1.1.1", "1.1.2"}).Return(&gosnmp.SnmpPacket{}, errors.New("next error"))
				sess.On("GetNext", []string{"1.1.1"}).Return(&gosnmp.SnmpPacket{}, errors.New("next error"))

				return sess
			},
			scalarOids: []string{"1.1.1.1.0", "1.1.1.2.0", "1.1.1.3.0", "1.1.1.4.0"},
			columnOids: []string{"1.1.1", "1.1.2", "1.1.3", "1.1.4"},
			batchSizeOptimizers: &OidBatchSizeOptimizers{
				snmpGetOptimizer: &oidBatchSizeOptimizer{
					snmpOperation:           snmpGet,
					configBatchSize:         4,
					batchSize:               4,
					failuresByBatchSize:     map[int]int{},
					lastSuccessfulBatchSize: 4,
				},
				snmpGetBulkOptimizer: &oidBatchSizeOptimizer{
					snmpOperation:           snmpGetBulk,
					configBatchSize:         4,
					batchSize:               4,
					failuresByBatchSize:     map[int]int{},
					lastSuccessfulBatchSize: 0,
				},
				snmpGetNextOptimizer: &oidBatchSizeOptimizer{
					snmpOperation:           snmpGetNext,
					configBatchSize:         4,
					batchSize:               4,
					failuresByBatchSize:     map[int]int{},
					lastSuccessfulBatchSize: 0,
				},
				lastRefreshTs: now,
			},
			expectedValues: nil,
			expectedError:  errors.New("failed to fetch oids with GetNext batching: failed to fetch column oids: fetch column: failed getting oids `[1.1.1]` using GetNext: next error"),
			expectedBatchSizeOptimizers: &OidBatchSizeOptimizers{
				snmpGetOptimizer: &oidBatchSizeOptimizer{
					snmpOperation:           snmpGet,
					configBatchSize:         4,
					batchSize:               4,
					failuresByBatchSize:     map[int]int{},
					lastSuccessfulBatchSize: 4,
				},
				snmpGetBulkOptimizer: &oidBatchSizeOptimizer{
					snmpOperation:           snmpGetBulk,
					configBatchSize:         4,
					batchSize:               1,
					failuresByBatchSize:     map[int]int{4: 1, 2: 1, 1: 1},
					lastSuccessfulBatchSize: 0,
				},
				snmpGetNextOptimizer: &oidBatchSizeOptimizer{
					snmpOperation:           snmpGetNext,
					configBatchSize:         4,
					batchSize:               1,
					failuresByBatchSize:     map[int]int{4: 1, 2: 1, 1: 1},
					lastSuccessfulBatchSize: 0,
				},
				lastRefreshTs: now,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sess := tt.sessionFactory()
			values, err := Fetch(sess, tt.scalarOids, tt.columnOids, tt.batchSizeOptimizers, checkconfig.DefaultBulkMaxRepetitions)
			if tt.expectedError != nil {
				assert.Equal(t, tt.expectedError, err)
			} else {
				assert.Nil(t, err)
				assert.Equal(t, tt.expectedValues, values)
			}
			assert.Equal(t, tt.expectedBatchSizeOptimizers, tt.batchSizeOptimizers)
		})
	}
}

func Test_batchSizeOptimizers_notFetchErrors(t *testing.T) {
	// This tests that fetch is not retried when we have an other error than a fetchErr

	sess := session.CreateMockSession()
	sess.Version = gosnmp.Version1

	batchSizeOptimizers := NewOidBatchSizeOptimizers(2)
	lastRefreshTs := batchSizeOptimizers.lastRefreshTs

	sess.On("Get", []string{"1.1.1.1.0"}).Return(&gosnmp.SnmpPacket{
		Error:      gosnmp.NoSuchName,
		ErrorIndex: 200,
	}, nil)

	scalarValues, err := fetchScalarOidsWithBatching(sess, []string{"1.1.1.1.0"}, batchSizeOptimizers.snmpGetOptimizer)
	assert.EqualError(t, err, "failed to fetch scalar oids: invalid ErrorIndex `200` when fetching oids `[1.1.1.1.0]`")
	assert.Nil(t, scalarValues)
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

	columnValues, err := fetchColumnOidsWithBatching(sess, []string{"1.1.1"}, batchSizeOptimizers.snmpGetBulkOptimizer,
		checkconfig.DefaultBulkMaxRepetitions, useGetBulk)
	assert.EqualError(t, err, "failed to fetch column oids: GetBulk not supported in SNMP v1")
	assert.Nil(t, columnValues)
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
	batchSizeOptimizers.snmpGetOptimizer.failuresByBatchSize = map[int]int{4: 2, 3: 1}
	batchSizeOptimizers.snmpGetBulkOptimizer.failuresByBatchSize = map[int]int{4: 2, 3: 1}
	batchSizeOptimizers.snmpGetNextOptimizer.failuresByBatchSize = map[int]int{4: 2, 3: 1}
	batchSizeOptimizers.lastRefreshTs = batchSizeOptimizers.lastRefreshTs.Add(-failuresWindowDuration * 2)

	oldLastRefreshTs := batchSizeOptimizers.lastRefreshTs

	values, err := Fetch(sess, nil, nil, batchSizeOptimizers, checkconfig.DefaultBulkMaxRepetitions)
	assert.Nil(t, err)
	assert.Equal(t, &valuestore.ResultValueStore{
		ScalarValues: valuestore.ScalarResultValuesType{},
		ColumnValues: valuestore.ColumnResultValuesType{},
	}, values)

	assert.Empty(t, batchSizeOptimizers.snmpGetOptimizer.failuresByBatchSize)
	assert.Empty(t, batchSizeOptimizers.snmpGetBulkOptimizer.failuresByBatchSize)
	assert.Empty(t, batchSizeOptimizers.snmpGetNextOptimizer.failuresByBatchSize)
	assert.True(t, batchSizeOptimizers.lastRefreshTs.After(oldLastRefreshTs))
}
