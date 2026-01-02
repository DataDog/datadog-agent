use crate::Result;

pub mod console;
pub use self::console::Console;

pub mod shlib;
pub use self::shlib::SharedLibrary;

// TODO can we take Metric/ServiceCheck/... as ref to avoid alloc?
pub trait Sink {
    fn submit_metric(&self, metric: metric::Metric, flush_first: bool) -> Result<()>;
    fn submit_service_check(&self, service_check: service_check::ServiceCheck) -> Result<()>;
    fn submit_event(&self, check_id: &str, event: event::Event) -> Result<()>;
    fn submit_histogram(&self, histogram: histogram::Histrogram, flush_first: bool) -> Result<()>;
    fn submit_event_platform_event(&self, event: event_platform_event::Event) -> Result<()>;

    // FIXME Accept any kind of string
    fn log(&self, level: log::Level, message: String);
}

pub mod log {
    #[derive(Debug, Copy, Clone, PartialEq)]
    pub enum Level {
        Critical = 50,
        Error = 40,
        Warning = 30,
        Info = 20,
        Debug = 10,
        Trace = 7,
    }
}

pub mod metric {
    use std::collections::HashMap;

    #[derive(Debug, Copy, Clone, PartialEq)]
    pub enum Type {
        Gauge = 0,
        Rate,
        Count,
        MonotonicCount,
        Counter,
        Histrogram,
        Historate,
    }

    // rtloader/include/rtloader_types.h
    #[derive(Debug, Clone)]
    pub struct Metric {
        pub id: String,
        pub metric_type: Type,
        pub name: String,
        pub value: f64,
        pub tags: HashMap<String, String>,
        pub hostname: String,
    }
}

pub mod service_check {
    use std::collections::HashMap;

    // integrations-core/datadog_checks_base/datadog_checks/base/types.py
    #[derive(Debug, Copy, Clone, PartialEq)]
    pub enum Status {
        Ok = 0,
        Warning,
        Critical,
        Unknown,
    }

    #[derive(Debug, Clone)]
    pub struct ServiceCheck {
        pub id: String,
        pub name: String,
        pub status: Status,
        pub tags: HashMap<String, String>,
        pub hostname: String,
        pub message: String,
    }
}

pub mod event {
    use std::collections::HashMap;

    #[derive(Debug, Clone)]
    pub struct Event {
        pub title: String,
        pub text: String,
        pub timestamp: u64,
        pub priority: String,
        pub hostname: String,
        pub tags: HashMap<String, String>,
        pub alert_type: String,
        pub aggregation_key: String,
        pub source_type_name: String,
        pub event_type: String,
    }
}

pub mod histogram {
    use std::collections::HashMap;

    #[derive(Debug, Clone)]
    pub struct Histrogram {
        pub id: String,
        pub metric_name: String,
        pub value: i64,
        pub lower_bound: f32,
        pub upper_bound: f32,
        pub monotonic: i32,
        pub hostname: String,
        pub tags: HashMap<String, String>,
    }
}

pub mod event_platform_event {
    #[derive(Debug, Clone)]
    pub struct Event {
        pub id: String,
        pub event: String,
        pub event_type: String,
    }
}
