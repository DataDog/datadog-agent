use core::generate_ffi;

mod check;
use check::check;

mod config;
mod payload;
mod version;
use version::VERSION;

generate_ffi!(check, VERSION);
