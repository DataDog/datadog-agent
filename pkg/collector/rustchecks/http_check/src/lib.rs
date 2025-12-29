pub mod check;
pub mod sink;
pub mod version;

pub type GenericError = Box<dyn std::error::Error + Send + Sync + 'static>;
pub type Result<T> = std::result::Result<T, GenericError>;
