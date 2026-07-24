use core::generate_ffi;

mod backend;
mod check;
use check::check;

mod config;
mod payload;
mod scanning;
mod version;
use version::VERSION;

generate_ffi!(check, VERSION);
