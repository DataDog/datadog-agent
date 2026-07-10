use shlib_core::generate_ffi;

mod backend;
mod check;
use check::check;

mod constants;
mod payload;
mod proto;
mod scanner;
mod version;
use version::VERSION;

generate_ffi!(check, VERSION);
