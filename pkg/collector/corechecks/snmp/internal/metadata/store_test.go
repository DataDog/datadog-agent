// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metadata

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/valuestore"
)

func TestStore_Scalar(t *testing.T) {
	store := NewMetadataStore()
	store.AddScalarValue("device.name", valuestore.ResultValue{Value: "someName"})
	store.AddScalarValue("device.description", valuestore.ResultValue{Value: "someDescription"})

	assert.Equal(t, "someName", store.GetScalarAsString("device.name"))
	assert.Equal(t, "someDescription", store.GetScalarAsString("device.description"))

	// error cases
	assert.Equal(t, "", store.GetScalarAsString("does_not_exist"))

	store.AddScalarValue("device.invalid_value_type", valuestore.ResultValue{Value: byte(1)})
	assert.Equal(t, "", store.GetScalarAsString("device.invalid_value_type"))
}

func TestStore_ScalarFieldHasValue(t *testing.T) {
	store := NewMetadataStore()
	store.AddScalarValue("device.name", valuestore.ResultValue{Value: "someName"})
	store.AddScalarValue("device.description", valuestore.ResultValue{Value: "someDescription"})

	assert.Equal(t, true, store.ScalarFieldHasValue("device.name"))
	assert.Equal(t, true, store.ScalarFieldHasValue("device.description"))
	assert.Equal(t, false, store.ScalarFieldHasValue("device.invalid_value_type"))
	assert.Equal(t, false, store.ScalarFieldHasValue("invalid.invalid_value_type"))
}

func TestStore_Column(t *testing.T) {
	store := NewMetadataStore()
	store.AddColumnValue("interface.name", "1", valuestore.ResultValue{Value: "ifName1"})
	store.AddColumnValue("interface.description", "1", valuestore.ResultValue{Value: "ifDesc1"})
	store.AddColumnValue("interface.admin_status", "1", valuestore.ResultValue{Value: float64(1)})
	store.AddColumnValue("interface.name", "2", valuestore.ResultValue{Value: "ifName2"})
	store.AddColumnValue("interface.description", "2", valuestore.ResultValue{Value: "ifDesc2"})
	store.AddColumnValue("interface.admin_status", "2", valuestore.ResultValue{Value: float64(2)})
	store.AddColumnValue("interface.admin_status", "3", valuestore.ResultValue{Value: float64(2)})
	store.AddColumnValue("interface.oper_status", "3", valuestore.ResultValue{Value: float64(2)})
	store.AddColumnValue("interface.mac_address", "1", valuestore.ResultValue{Value: []byte{1, 2, 3, 4, 5, 6}})
	store.AddColumnValue("interface.invalid_value_type", "3", valuestore.ResultValue{Value: byte(1)})

	// test GetColumnAsString
	assert.Equal(t, "ifName1", store.GetColumnAsString("interface.name", "1"))
	assert.Equal(t, "ifDesc1", store.GetColumnAsString("interface.description", "1"))
	assert.Equal(t, "ifName2", store.GetColumnAsString("interface.name", "2"))
	assert.Equal(t, "ifDesc2", store.GetColumnAsString("interface.description", "2"))
	assert.Equal(t, "", store.GetColumnAsString("interface.does_not_exist", "2"))
	assert.Equal(t, "", store.GetColumnAsString("interface.description", "1.2.3")) // missing index
	assert.Equal(t, "", store.GetColumnAsString("interface.invalid_value_type", "3"))

	// test GetColumnAsFloat
	assert.Equal(t, float64(1), store.GetColumnAsFloat("interface.admin_status", "1"))
	assert.Equal(t, float64(2), store.GetColumnAsFloat("interface.admin_status", "2"))
	assert.Equal(t, float64(0), store.GetColumnAsFloat("interface.does_not_exist", "2"))
	assert.Equal(t, float64(0), store.GetColumnAsFloat("interface.admin_status", "1.2.3"))   // missing index
	assert.Equal(t, float64(0), store.GetColumnAsFloat("interface.invalid_value_type", "3")) // missing index

	// test GetColumnIndexes
	assert.ElementsMatch(t, []string{"1", "2"}, store.GetColumnIndexes("interface.name"))
	assert.ElementsMatch(t, []string{"1", "2", "3"}, store.GetColumnIndexes("interface.admin_status"))
	assert.Equal(t, []string{"3"}, store.GetColumnIndexes("interface.oper_status"))
	assert.Equal(t, []string(nil), store.GetColumnIndexes("interface.does_not_exist"))

	// test GetColumnAsByteArray
	assert.Equal(t, []byte{0x1, 0x2, 0x3, 0x4, 0x5, 0x6}, store.GetColumnAsByteArray("interface.mac_address", "1"))
	assert.Equal(t, []byte(nil), store.GetColumnAsByteArray("interface.mac_address", "2"))
	assert.Equal(t, []byte(nil), store.GetColumnAsByteArray("interface.does_not_exist", "1"))
}

func TestStore_IDTags(t *testing.T) {
	store := NewMetadataStore()
	store.AddIDTags("interface", "1", []string{"aa"})
	store.AddIDTags("interface", "1", []string{"bb"})
	store.AddIDTags("interface", "2", []string{"cc"})
	store.AddIDTags("interface", "2", []string{"dd"})

	// test GetIDTags
	assert.Equal(t, []string{"aa", "bb"}, store.GetIDTags("interface", "1"))
	assert.Equal(t, []string{"cc", "dd"}, store.GetIDTags("interface", "2"))
	assert.Equal(t, []string(nil), store.GetIDTags("does_not_exist", "2"))
	assert.Equal(t, []string(nil), store.GetIDTags("interface", "9")) // does not exist
}
