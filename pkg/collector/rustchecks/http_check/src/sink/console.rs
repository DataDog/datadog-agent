use crate::sink::{Sink, event, event_platform_event, histogram, log, metric, service_check};
use crate::Result;

pub struct Console {}

impl Sink for Console {
    fn submit_metric(&self, metric: metric::Metric, _flush_first: bool) -> Result<()> {
        println!("submit_metric: {:#?}", metric);
        Ok(())
    }

    fn submit_service_check(&self, service_check: service_check::ServiceCheck) -> Result<()> {
        println!("submit_service_check: {:#?}", service_check);
        Ok(())
    }

    fn submit_event(&self, _check_id: &str, event: event::Event) -> Result<()> {
        println!("submit_event: {:#?}", event);
        Ok(())
    }

    fn submit_histogram(&self, histogram: histogram::Histrogram, _flush_first: bool) -> Result<()> {
        println!("submit_histogram: {:#?}", histogram);
        Ok(())
    }

    fn submit_event_platform_event(&self, event: event_platform_event::Event) -> Result<()> {
        println!("submit_event_platform_event: {:#?}", event);
        Ok(())
    }

    fn log(&self, level: log::Level, message: String) {
        println!("[{level:?}] {message}")
    }
}
