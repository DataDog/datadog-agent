pub mod simple;
pub mod java;
pub mod python;
pub mod nodejs;
pub mod php;
pub mod ruby;
pub mod dotnet;

use crate::context::DetectionContext;
use crate::error::UsmResult;
use crate::metadata::ServiceMetadata;

/// Trait for detecting service metadata from process information
pub trait Detector: Send + Sync {
    /// Attempt to detect service metadata from the remaining command line arguments
    /// Returns Ok(Some(metadata)) if detection succeeded, Ok(None) if detection failed,
    /// or Err if an error occurred during detection
    fn detect(&self, ctx: &mut DetectionContext, remaining_args: &[String]) -> UsmResult<Option<ServiceMetadata>>;
    
    /// Get the name of this detector for debugging/logging
    fn name(&self) -> &'static str;
}

/// Factory function type for creating detectors
pub type DetectorFactory = fn(ctx: &DetectionContext) -> Box<dyn Detector>;

pub use simple::SimpleDetector;
pub use java::JavaDetector;
pub use python::PythonDetector;
pub use nodejs::NodejsDetector;
pub use php::PhpDetector;
pub use ruby::RubyDetector;
pub use dotnet::DotnetDetector;