syntax = "proto3";

message Payload {
  Metadata metadata = 1;
  oneof content { // nesting is limited to a small number
    Payload payload = 2;
    MetricData metricData = 3;
  }
}

message Metadata {
  repeated string tags = 1;
  repeated string resources = 2; // even number of elements, [Type, Name] pairs
}

message MetricData {
  // Dictionaries
  bytes dictNameStr = 1;                // varint length + value
  bytes dictTagsStr = 2;                // varint length + value
  repeated sint64 dictTagsets = 3;      // length,
                                        // delta encoded set of indexes itno dictTagsStr
  bytes dictResourceStr = 4;            // varint length + value
  repeated int64 dictResourceLen = 5;   // number  of elements in Type and Name arrays
  repeated sint64 dictResourceType = 6; // delta encoded set of indexes into dictResourceStr
  repeated sint64 dictResourceName = 7; // delta encoded set of indexes into dictResourceStr

  bytes dictSourceTypeName = 8;         // varint length + value
  repeated int32 dictOriginInfo = 9;    // (product, category, service) tuples

  // One entry per time series
  repeated uint64 types = 10;           // type = metricType | valueType | metricFlags
  repeated sint64 names = 11;           // index into dictNameStr, entire array is delta encoded
  repeated sint64 tags  = 12;           // index into dictTagsets, entire array is delta encoded
  repeated sint64 resources = 13;       // index into dictResourceLen, entire array is delta encoded
  repeated uint64 intervals = 14;
  repeated uint64 numPoints = 15;

  // each metric has numPoints values in this section
  repeated sint64 timestamps = 16;      // entire array delta encoded
  repeated sint64 valsInt64 = 17;       // or
  repeated float valsFloat32 = 18;      // or
  repeated double valsFloat64 = 19;     // based on valueType
  repeated uint64 sketchNumBins = 20;
  repeated sint32 sketchBinKeys = 21;   // per-metric sequence is delta encoded
  repeated uint32 sketchBinCnts = 22;
  // sketch summary Sum, Min, Max are encoded as three consecuive elements in one of vals using valueType
  // sketch summary Cnt is always encoded in valInt64
  // sketch summary Avg is reconstructed as Sum/Cnt in the intake
  repeated sint64 sourceTypeName = 23;  // index into dictSourceTypeName, entire array is delta encoded
  repeated sint64 originInfo = 24;      // index into dictOriginInfo, entire array is delta encoded
}

enum metricType {
  unused = 0;
  count = 1;  // matches current metrics intake types
  rate = 2;
  gauge = 3;
  sketch = 4; // new
}

enum valueType {
  zero    = 0x00;  // value is zero, not stored explicitly
  float64 = 0x10;  // value is stored in valsFloat64
  float32 = 0x20;  // value is stored in valsFloat32
  int8    = 0x30;  // value is stored in valsInt8
}

enum metircFlags {
  flagNone    = 0;
  flagNoIndex = 0x100;
}
