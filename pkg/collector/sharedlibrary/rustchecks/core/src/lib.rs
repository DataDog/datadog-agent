// modules used by checks
mod agent_check;
pub use agent_check::AgentCheck;

mod aggregator;
pub use aggregator::{Aggregator, Event, LogLevel, MetricType, ServiceCheckStatus};

mod config;
pub use config::Config;

// FFI using the C-ABI
mod ffi;

mod cstring;
pub use cstring::to_cstring;
pub use cstring::to_rust_string;

// Shared test harness (like integrations-core's `AggregatorStub`). Unreachable
// from the `Run`/`Version` FFI entrypoints, so the linker strips it from release cdylibs.
pub mod stubs;
pub use stubs::{
    AggregatorStub, RecordedEvent, RecordedEventPlatformEvent, RecordedHistogramBucket,
    RecordedLog, RecordedMetric, RecordedServiceCheck,
};
