use shlib_core::generate_ffi;

mod backend;
mod check;
use check::check;

mod config;
mod constants;
mod proto;
mod result;
mod scanning;
mod version;
use version::VERSION;

generate_ffi!(check, VERSION);
