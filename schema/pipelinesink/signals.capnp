# Cap'n Proto schema for the pipeline sink signal types.
# File ID: 0xdeadbeefcafe0001
@0xdeadbeefcafe0001;

using Go = import "/go.capnp";
$Go.package("signals");
$Go.import("github.com/DataDog/datadog-agent/schema/pipelinesink");

struct SignalEnvelope {
  union {
    metricBatch @0 :MetricBatch;
    logBatch    @1 :LogBatch;
    # @2 reserved for TraceBatch
    # @3 reserved for ProfileBatch
    # @4 reserved for TraceStatBatch
  }
}

struct MetricBatch {
  samples @0 :List(MetricSample);
}

struct MetricSample {
  name        @0 :Text;
  value       @1 :Float64;
  tags        @2 :List(Text);
  timestampNs @3 :Int64;
  sampleRate  @4 :Float64;
  source      @5 :Text;
}

struct LogBatch {
  entries @0 :List(LogEntry);
}

struct LogEntry {
  content     @0 :Data;
  status      @1 :Text;
  tags        @2 :List(Text);
  hostname    @3 :Text;
  timestampNs @4 :Int64;
  source      @5 :Text;
}
