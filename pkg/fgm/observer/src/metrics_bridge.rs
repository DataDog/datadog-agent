/// Metrics bridge for intercepting metrics facade emissions
///
/// This module implements a custom metrics::Recorder that captures all
/// gauge/counter emissions from lading and routes them to a callback.
/// This allows us to use lading's observer code as-is while extracting
/// raw metric values for FFI.

use metrics::{Counter, Gauge, Histogram, Key, KeyName, Metadata, Recorder, SharedString, Unit};
use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::Arc;
use std::time::SystemTime;

/// Callback function type for metric emissions
pub type MetricCallback = Box<dyn Fn(&str, f64, Vec<(String, String)>, i64) + Send + Sync>;

/// Wrapper for Counter that calls callback on increment
struct CallbackCounter {
    key: Key,
    callback: Arc<MetricCallback>,
    value: AtomicU64,
}

impl CallbackCounter {
    fn new(key: Key, callback: Arc<MetricCallback>) -> Self {
        Self {
            key,
            callback,
            value: AtomicU64::new(0),
        }
    }

    fn emit(&self, value: u64) {
        let name = self.key.name().to_string();
        let labels: Vec<(String, String)> = self
            .key
            .labels()
            .map(|label| (label.key().to_string(), label.value().to_string()))
            .collect();

        let timestamp = SystemTime::now()
            .duration_since(SystemTime::UNIX_EPOCH)
            .unwrap()
            .as_millis() as i64;

        (self.callback)(&name, value as f64, labels, timestamp);
    }
}

impl metrics::CounterFn for CallbackCounter {
    fn increment(&self, value: u64) {
        let new_value = self.value.fetch_add(value, Ordering::Relaxed) + value;
        self.emit(new_value);
    }

    fn absolute(&self, value: u64) {
        self.value.store(value, Ordering::Relaxed);
        self.emit(value);
    }
}

/// Wrapper for Gauge that calls callback on set
struct CallbackGauge {
    key: Key,
    callback: Arc<MetricCallback>,
}

impl CallbackGauge {
    fn new(key: Key, callback: Arc<MetricCallback>) -> Self {
        Self { key, callback }
    }

    fn emit(&self, value: f64) {
        let name = self.key.name().to_string();
        let labels: Vec<(String, String)> = self
            .key
            .labels()
            .map(|label| (label.key().to_string(), label.value().to_string()))
            .collect();

        let timestamp = SystemTime::now()
            .duration_since(SystemTime::UNIX_EPOCH)
            .unwrap()
            .as_millis() as i64;

        (self.callback)(&name, value, labels, timestamp);
    }
}

impl metrics::GaugeFn for CallbackGauge {
    fn increment(&self, value: f64) {
        self.emit(value);
    }

    fn decrement(&self, value: f64) {
        self.emit(-value);
    }

    fn set(&self, value: f64) {
        self.emit(value);
    }
}

/// Wrapper for Histogram that calls callback on record
struct CallbackHistogram {
    key: Key,
    callback: Arc<MetricCallback>,
}

impl CallbackHistogram {
    fn new(key: Key, callback: Arc<MetricCallback>) -> Self {
        Self { key, callback }
    }

    fn emit(&self, value: f64) {
        let name = self.key.name().to_string();
        let labels: Vec<(String, String)> = self
            .key
            .labels()
            .map(|label| (label.key().to_string(), label.value().to_string()))
            .collect();

        let timestamp = SystemTime::now()
            .duration_since(SystemTime::UNIX_EPOCH)
            .unwrap()
            .as_millis() as i64;

        (self.callback)(&name, value, labels, timestamp);
    }
}

impl metrics::HistogramFn for CallbackHistogram {
    fn record(&self, value: f64) {
        self.emit(value);
    }
}

/// Custom recorder that captures metrics and routes to callback
pub struct CallbackRecorder {
    callback: Arc<MetricCallback>,
}

impl CallbackRecorder {
    pub fn new(callback: MetricCallback) -> Self {
        Self {
            callback: Arc::new(callback),
        }
    }
}

impl Recorder for CallbackRecorder {
    fn describe_counter(&self, _key: KeyName, _unit: Option<Unit>, _description: SharedString) {
        // No-op: we don't need metadata for our use case
    }

    fn describe_gauge(&self, _key: KeyName, _unit: Option<Unit>, _description: SharedString) {
        // No-op: we don't need metadata for our use case
    }

    fn describe_histogram(&self, _key: KeyName, _unit: Option<Unit>, _description: SharedString) {
        // No-op: we don't need metadata for our use case
    }

    fn register_counter(&self, key: &Key, _metadata: &Metadata<'_>) -> Counter {
        let counter = CallbackCounter::new(key.clone(), self.callback.clone());
        Counter::from_arc(Arc::new(counter))
    }

    fn register_gauge(&self, key: &Key, _metadata: &Metadata<'_>) -> Gauge {
        let gauge = CallbackGauge::new(key.clone(), self.callback.clone());
        Gauge::from_arc(Arc::new(gauge))
    }

    fn register_histogram(&self, key: &Key, _metadata: &Metadata<'_>) -> Histogram {
        let histogram = CallbackHistogram::new(key.clone(), self.callback.clone());
        Histogram::from_arc(Arc::new(histogram))
    }
}
