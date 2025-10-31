pub mod os;
pub mod fqdn;
pub mod file;

pub use os::get_hostname as os_hostname;
pub use fqdn::get_hostname as fqdn_hostname;
pub use file::get_hostname as file_hostname;
