// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"encoding/binary"
	"fmt"
	"math/bits"
	"slices"

	"github.com/twmb/murmur3"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
	compression "github.com/DataDog/datadog-agent/comp/serializer/metricscompression/def"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/serializer/internal/stream"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
)

var (
	tlmColumnSize = telemetry.NewCounter("serializer", "v3_column_size",
		[]string{"column", "compressed"},
		"Number of bytes occupied by each column",
	)
	tlmSplitReason = telemetry.NewCounter("serializer", "v3_payload_split_reason",
		[]string{"reason"},
		"Why payload was split",
	)
)

const (
	payloadFieldMetricData = 3
)

const (
	// References to a dictionary entry have suffix Ref
	columnDictNameStr = 1 + iota
	columnDictTagsStr
	columnDictTagsets
	columnDictResourceStr
	columnDictResourcesLen
	columnDictResourceType
	columnDictResourceName
	columnDictSourceTypeName
	columnDictOrigin
	columnType
	columnNameRef
	columnTagsRef
	columnResourcesRef
	columnInterval
	columnNumPoints
	columnTimestamp
	columnValueSint64
	columnValueFloat32
	columnValueFloat64
	columnSketchNBins
	columnSketchBinKeys
	columnSketchBinCounts
	columnSourceTypeNameRef
	columnOriginRef
	numberOfColumns
)

var columnNames = []string{
	"reserved",
	"DictNameStr",
	"DictTagsStr",
	"DictTagsets",
	"DictResourceStr",
	"DictResourcesLen",
	"DictResourceType",
	"DictResourceName",
	"DictSourceTypeName",
	"DictOriginInfo",
	"Type",
	"Name",
	"Tags",
	"Resources",
	"Interval",
	"NumPoints",
	"Timestamp",
	"ValueSint64",
	"ValueFloat32",
	"ValueFloat64",
	"SketchNBins",
	"SketchBinKeys",
	"SketchBinCounts",
	"SourceTypeName",
	"OriginInfo",
}

// Constants for type column
const (
	metricCount = 0x01
	metricRate  = 0x02
	metricGauge = 0x03

	valueZero    int64 = 0x00
	valueSint64  int64 = 0x10
	valueFloat32 int64 = 0x20
	valueFloat64 int64 = 0x30

	flagNoIndex = 0x100
)

type payloadsBuilderV3 struct {
	compression compression.Component
	compressor  stream.ColumnCompressor
	txn         *stream.ColumnTransaction
	dict        *dictionaryBuilder

	deltaNameRef           deltaEncoder
	deltaTagsRef           deltaEncoder
	deltaResourcesRef      deltaEncoder
	deltaInterval          deltaEncoder
	deltaTimestamp         deltaEncoder
	deltaSourceTypeNameRef deltaEncoder
	deltaOriginRef         deltaEncoder

	pointsThisPayload   int
	maxPointsPerPayload int

	payloadHeaderSizeBound int
	columnHeaderSizeBound  int

	pipelineConfig  PipelineConfig
	pipelineContext *PipelineContext

	resourcesBuf []metrics.Resource

	scratchBuf []byte

	stats struct {
		valuesZero, valuesSint64, valuesFloat32, valuesFloat64 uint64
	}
}

func newPayloadsBuilderV3WithConfig(
	config config.Component,
	compression compression.Component,
	pipelineConfig PipelineConfig,
	pipelineContext *PipelineContext,
) (*payloadsBuilderV3, error) {
	maxCompressedSize := config.GetInt("serializer_max_series_payload_size")
	maxUncompressedSize := config.GetInt("serializer_max_series_uncompressed_payload_size")
	maxPointsPerPayload := config.GetInt("serializer_max_series_points_per_payload")

	return newPayloadsBuilderV3(
		maxCompressedSize,
		maxUncompressedSize,
		maxPointsPerPayload,
		compression,
		pipelineConfig,
		pipelineContext,
	)
}

func newPayloadsBuilderV3(
	maxCompressedSize int,
	maxUncompressedSize int,
	maxPointsPerPayload int,
	compression compression.Component,
	pipelineConfig PipelineConfig,
	pipelineContext *PipelineContext,
) (*payloadsBuilderV3, error) {
	payloadHeaderSize := protobufFieldHeaderLen(payloadFieldMetricData, maxUncompressedSize)
	payloadHeaderSizeBound := compression.CompressBound(payloadHeaderSize)
	columnHeaderSize := protobufFieldHeaderLen(numberOfColumns-1, maxUncompressedSize)
	columnHeaderSizeBound := compression.CompressBound(columnHeaderSize)
	reservedCompressedSize := payloadHeaderSizeBound + columnHeaderSizeBound*numberOfColumns
	reservedUncompressedSize := payloadHeaderSize + columnHeaderSize*numberOfColumns
	maxCompressedSize -= reservedCompressedSize
	maxUncompressedSize -= reservedUncompressedSize
	if maxCompressedSize < 0 {
		return nil, fmt.Errorf("maxCompressedSize is too small, must be larger than %d bytes", reservedCompressedSize)
	}
	if maxUncompressedSize < 0 {
		return nil, fmt.Errorf("maxUncompressedSize is too small, must be larger than %d bytes", reservedUncompressedSize)
	}

	compressor := stream.NewColumnCompressor(
		compression,
		numberOfColumns,
		maxCompressedSize,
		maxUncompressedSize,
	)

	txn := compressor.NewTransaction()
	return &payloadsBuilderV3{
		compression: compression,
		compressor:  compressor,
		txn:         txn,
		dict:        newDictionaryBuilder(txn),

		maxPointsPerPayload: maxPointsPerPayload,

		payloadHeaderSizeBound: payloadHeaderSizeBound,
		columnHeaderSizeBound:  columnHeaderSizeBound,

		pipelineConfig:  pipelineConfig,
		pipelineContext: pipelineContext,

		scratchBuf: make([]byte, max(payloadHeaderSize, columnHeaderSize)),
	}, nil
}

func (pb *payloadsBuilderV3) startPayload() error {
	return nil
}

func (pb *payloadsBuilderV3) finishPayload() error {
	// Build the final protobuf payload by concatenating several independently compressed streams:
	// field headers and column data. gzip and zstd decompressors will handle such concatenated
	// streams transparently as if it was compressed in one go.

	if pb.pointsThisPayload > 0 {
		err := pb.compressor.Close()
		if err != nil {
			return err
		}

		compressedPayloadSize := pb.payloadHeaderSizeBound
		uncompressedMetricDataSize := 0
		for i := 0; i < numberOfColumns; i++ {
			uncompressedLen := pb.compressor.UncompressedLen(i)
			compressedBytes := pb.compressor.CompressedBytes(i)

			if uncompressedLen == 0 {
				continue
			}

			compressedPayloadSize += pb.columnHeaderSizeBound + len(compressedBytes)
			uncompressedMetricDataSize += protobufFieldHeaderLen(i, uncompressedLen) + uncompressedLen

			tlmColumnSize.Add(float64(len(compressedBytes)), columnNames[i], "compressed")
			tlmColumnSize.Add(float64(uncompressedLen), columnNames[i], "uncompressed")
		}

		payload := make([]byte, 0, compressedPayloadSize)
		payload, err = pb.appendProtobufFieldHeader(payload, payloadFieldMetricData, uncompressedMetricDataSize)
		if err != nil {
			return err
		}

		for i := 0; i < numberOfColumns; i++ {
			uncompressedLen := pb.compressor.UncompressedLen(i)
			compressedBytes := pb.compressor.CompressedBytes(i)

			if uncompressedLen == 0 {
				continue
			}

			payload, err = pb.appendProtobufFieldHeader(payload, i, uncompressedLen)
			if err != nil {
				return err
			}
			payload = append(payload, compressedBytes...)
		}

		pb.pipelineContext.addPayload(transaction.NewBytesPayload(payload, pb.pointsThisPayload))
	}

	pb.reset()

	return nil
}

func (pb *payloadsBuilderV3) appendProtobufFieldHeader(dst []byte, id int, len int) ([]byte, error) {
	n := binary.PutUvarint(pb.scratchBuf[0:], protobufFieldID(id, pbTypeBytes))
	n += binary.PutUvarint(pb.scratchBuf[n:], uint64(len))
	header, err := pb.compression.Compress(pb.scratchBuf[:n])
	if err != nil {
		return nil, err
	}
	return append(dst, header...), nil
}

func (pb *payloadsBuilderV3) reset() {
	pb.pointsThisPayload = 0
	pb.dict.reset()
	pb.deltaNameRef.reset()
	pb.deltaTagsRef.reset()
	pb.deltaResourcesRef.reset()
	pb.deltaInterval.reset()
	pb.deltaTimestamp.reset()
	pb.deltaSourceTypeNameRef.reset()
	pb.deltaOriginRef.reset()
	pb.compressor.Reset()
}

func (pb *payloadsBuilderV3) renderResources(serie *metrics.Serie) {
	pb.resourcesBuf = pb.resourcesBuf[0:0]

	if serie.Host != "" {
		pb.resourcesBuf = append(pb.resourcesBuf, metrics.Resource{
			Type: "host",
			Name: serie.Host,
		})
	}

	if serie.Device != "" {
		pb.resourcesBuf = append(pb.resourcesBuf, metrics.Resource{
			Type: "device",
			Name: serie.Device,
		})
	}

	pb.resourcesBuf = append(pb.resourcesBuf, serie.Resources...)
}

func (pb *payloadsBuilderV3) writeSerie(serie *metrics.Serie) error {
	if !pb.pipelineConfig.Filter.Filter(serie) {
		return nil
	}

	if len(serie.Points) == 0 {
		return nil
	}

	if len(serie.Points) > pb.maxPointsPerPayload {
		tlmItemTooBig.Inc()
		return nil
	}

	if len(serie.Points)+pb.pointsThisPayload > pb.maxPointsPerPayload {
		tlmSplitReason.Inc("max_points")
		err := pb.finishPayload()
		if err != nil {
			return err
		}
	}

	serie.PopulateDeviceField()
	serie.PopulateResources()

	for {
		pb.writeSerieToTxn(serie)

		err := pb.compressor.AddItem(pb.txn)
		switch err {
		case stream.ErrPayloadFull:
			tlmSplitReason.Inc("payload_full")
			err = pb.finishPayload()
			if err != nil {
				return err
			}
			continue
		case stream.ErrItemTooBig:
			tlmItemTooBig.Inc()
			tlmSplitReason.Inc("item_too_big")
			err = pb.finishPayload()
			if err != nil {
				return err
			}
			return nil
		case nil:
			pb.pointsThisPayload += len(serie.Points)
			return nil
		default:
			return err
		}
	}
}

func (pb *payloadsBuilderV3) writeSerieToTxn(serie *metrics.Serie) {
	pb.txn.Reset()
	pb.txn.Sint64(columnNameRef, pb.deltaNameRef.encode(pb.dict.internName(serie.Name)))
	pb.txn.Sint64(columnTagsRef, pb.deltaTagsRef.encode(pb.dict.internTags(serie.Tags)))
	pb.txn.Sint64(columnInterval, pb.deltaInterval.encode(serie.Interval))

	pb.renderResources(serie)
	pb.txn.Sint64(columnResourcesRef,
		pb.deltaResourcesRef.encode(pb.dict.internResources(pb.resourcesBuf)))

	pb.txn.Sint64(columnSourceTypeNameRef,
		pb.deltaSourceTypeNameRef.encode(pb.dict.internSourceTypeName(serie.SourceTypeName)))

	pb.txn.Sint64(columnOriginRef, pb.deltaOriginRef.encode(
		pb.dict.internOriginInfo(originInfo{
			product:  metricSourceToOriginProduct(serie.Source),
			category: metricSourceToOriginCategory(serie.Source),
			service:  metricSourceToOriginService(serie.Source),
		})))

	pb.txn.Int64(columnNumPoints, int64(len(serie.Points)))

	valueType := valueZero
	for _, pnt := range serie.Points {
		pointType := pointValueType(pnt.Value)
		if pointType > valueType {
			valueType = pointType
		}
	}

	typeValue := valueType | metricType(serie.MType)
	if serie.NoIndex {
		typeValue |= flagNoIndex
	}

	pb.txn.Int64(columnType, typeValue)

	for _, pnt := range serie.Points {
		pb.txn.Sint64(columnTimestamp, pb.deltaTimestamp.encode(int64(pnt.Ts)))
		switch valueType {
		case valueZero:
			pb.stats.valuesZero++
		case valueSint64:
			pb.stats.valuesSint64++
			pb.txn.Sint64(columnValueSint64, int64(pnt.Value))
		case valueFloat32:
			pb.stats.valuesFloat32++
			pb.txn.Float32(columnValueFloat32, float32(pnt.Value))
		case valueFloat64:
			pb.stats.valuesFloat64++
			pb.txn.Float64(columnValueFloat64, pnt.Value)
		}
	}
}

type deltaEncoder struct {
	prev int64
}

func (de *deltaEncoder) encode(val int64) int64 {
	delta := val - de.prev
	de.prev = val
	return delta
}

func (de *deltaEncoder) reset() {
	de.prev = 0
}

func deltaEncode(v []int64) {
	if len(v) < 2 {
		return
	}
	for i := len(v) - 1; i > 0; i-- {
		v[i] -= v[i-1]
	}
}

type internable interface {
	comparable
	appendTo(txn *stream.ColumnTransaction, dataColumnID int)
}

type interner[T internable] struct {
	txn          *stream.ColumnTransaction
	dataColumnID int

	lastID int64
	index  map[T]int64
}

func newInterner[T internable](txn *stream.ColumnTransaction, dataColumnID int) interner[T] {
	return interner[T]{
		txn:          txn,
		dataColumnID: dataColumnID,
		index:        map[T]int64{},
	}
}

func (i *interner[T]) reset() {
	i.lastID = 0
	i.index = map[T]int64{}
}

func (i *interner[T]) intern(v T) int64 {
	if id, ok := i.index[v]; ok {
		return id
	}
	i.lastID++
	i.index[v] = i.lastID
	v.appendTo(i.txn, i.dataColumnID)
	return i.lastID
}

type istr string

func (v istr) appendTo(txn *stream.ColumnTransaction, dataColumnID int) {
	txn.Int64(dataColumnID, int64(len(v)))
	txn.Write(dataColumnID, []byte(v))
}

type originInfo struct {
	product  int32
	category int32
	service  int32
}

func (info originInfo) appendTo(txn *stream.ColumnTransaction, dataColumnID int) {
	txn.Int64(dataColumnID, int64(info.product))
	txn.Int64(dataColumnID, int64(info.category))
	txn.Int64(dataColumnID, int64(info.service))
}

type dictionaryBuilder struct {
	txn *stream.ColumnTransaction

	namesInterner          interner[istr]
	tagsInterner           interner[istr]
	resourceInterner       interner[istr]
	sourceTypeNameInterner interner[istr]

	originInfoInterner interner[originInfo]

	tagsLastID int64
	tagsIndex  map[tagsKey]int64
	tagsBuffer []int64

	tagsStringBuf []string

	resourcesLastID int64
	resourcesIndex  map[any]int64

	stats struct {
		tagsSplit   uint64
		tagsUnsplit uint64
	}
}

type tagsKey = uint64

func newDictionaryBuilder(txn *stream.ColumnTransaction) *dictionaryBuilder {
	return &dictionaryBuilder{
		txn: txn,

		namesInterner:    newInterner[istr](txn, columnDictNameStr),
		tagsInterner:     newInterner[istr](txn, columnDictTagsStr),
		resourceInterner: newInterner[istr](txn, columnDictResourceStr),

		sourceTypeNameInterner: newInterner[istr](txn, columnDictSourceTypeName),

		originInfoInterner: newInterner[originInfo](txn, columnDictOrigin),

		tagsIndex:      make(map[tagsKey]int64),
		resourcesIndex: make(map[any]int64),
	}
}

func (db *dictionaryBuilder) reset() {
	db.namesInterner.reset()
	db.tagsInterner.reset()
	db.resourceInterner.reset()
	db.sourceTypeNameInterner.reset()
	db.originInfoInterner.reset()
	db.tagsLastID = 0
	db.tagsIndex = map[tagsKey]int64{}
	db.resourcesLastID = 0
	db.resourcesIndex = map[any]int64{}
}

func (db *dictionaryBuilder) internName(name string) int64 {
	if name == "" {
		return 0
	}
	return db.namesInterner.intern(istr(name))
}

func (db *dictionaryBuilder) appendTagsSlice(tags []string) {
	for _, s := range tags {
		db.tagsBuffer = append(db.tagsBuffer, db.tagsInterner.intern(istr(s)))
	}
}

func (db *dictionaryBuilder) internTags1(prefixID int64, tags []string) int64 {
	defer func() { db.tagsStringBuf = db.tagsStringBuf[:0] }()
	db.tagsStringBuf = append(db.tagsStringBuf, tags...)
	slices.Sort(db.tagsStringBuf)

	var hash1, hash2 uint64 = uint64(prefixID), 0
	for _, s := range db.tagsStringBuf {
		hash1, hash2 = murmur3.SeedStringSum128(hash1, hash2, s)
	}

	key := hash1
	if id, ok := db.tagsIndex[key]; ok {
		return id
	}

	db.tagsBuffer = db.tagsBuffer[0:0]
	if prefixID > 0 {
		db.tagsBuffer = append(db.tagsBuffer, -prefixID)
	}
	db.appendTagsSlice(db.tagsStringBuf)
	slices.Sort(db.tagsBuffer)
	deltaEncode(db.tagsBuffer)

	db.tagsLastID++
	db.tagsIndex[key] = db.tagsLastID

	db.txn.Sint64(columnDictTagsets, int64(len(db.tagsBuffer)))
	for _, idx := range db.tagsBuffer {
		db.txn.Sint64(columnDictTagsets, idx)
	}

	return db.tagsLastID
}

func (db *dictionaryBuilder) internTags(tags tagset.CompositeTags) int64 {
	t1, t2 := tags.UnsafeGet()

	if len(t1) == 0 && len(t2) == 0 {
		return 0
	} else if len(t1) == 0 {
		return db.internTags1(0, t2)
	} else if len(t2) == 0 {
		return db.internTags1(0, t1)
	}

	db.stats.tagsSplit++
	prefixID := db.internTags1(0, t1)
	return db.internTags1(prefixID, t2)
}

func (db *dictionaryBuilder) internResources(res []metrics.Resource) int64 {
	if len(res) == 0 {
		return 0
	}

	var hash1, hash2 uint64
	for _, r := range res {
		hash1, hash2 = murmur3.SeedStringSum128(hash1, hash2, r.Type)
		hash1, hash2 = murmur3.SeedStringSum128(hash1, hash2, r.Name)
	}

	key := hash1
	if id, ok := db.resourcesIndex[key]; ok {
		return id
	}

	db.resourcesLastID++
	db.resourcesIndex[key] = db.resourcesLastID

	db.txn.Int64(columnDictResourcesLen, int64(len(res)))

	typeDelta := deltaEncoder{}
	nameDelta := deltaEncoder{}

	for _, res := range res {
		db.txn.Sint64(columnDictResourceType,
			typeDelta.encode(db.resourceInterner.intern(istr(res.Type))))
		db.txn.Sint64(columnDictResourceName,
			nameDelta.encode(db.resourceInterner.intern(istr(res.Name))))
	}

	return db.resourcesLastID
}

func (db *dictionaryBuilder) internOriginInfo(info originInfo) int64 {
	return db.originInfoInterner.intern(info)
}

func (db *dictionaryBuilder) internSourceTypeName(stn string) int64 {
	if stn == "" {
		return 0
	}
	return db.sourceTypeNameInterner.intern(istr(stn))
}

func pointValueType(v float64) int64 {
	if v == 0 {
		return valueZero
	}

	// Integers in this range encode to 7 byte varints or less
	const maxInt = 1<<48 - 1
	const minInt = -1 << 48

	i := int64(v)
	if i >= minInt && i <= maxInt && float64(i) == v {
		return valueSint64
	}

	if float64(float32(v)) == v {
		return valueFloat32
	}

	return valueFloat64
}

func metricType(ty metrics.APIMetricType) int64 {
	switch ty {
	case metrics.APICountType:
		return metricCount
	case metrics.APIGaugeType:
		return metricGauge
	case metrics.APIRateType:
		return metricRate
	}
	panic("unknown APIMetricType")
}

func varintLen(v int) int {
	if v == 0 {
		return 1
	}

	n, rem := bits.Div(0, uint(bits.Len(uint(v))), 7)
	if rem > 0 {
		n++
	}
	return int(n)
}

func protobufFieldHeaderLen(id int, len int) int {
	return varintLen(id<<3) + varintLen(len)
}

type protobufType uint64

const (
	pbTypeBytes protobufType = 2
)

func protobufFieldID(field int, ty protobufType) uint64 {
	return uint64(field)<<3 | uint64(ty)
}
