//! Optional jemalloc heap profiling support.
//!
//! When `MALLOC_CONF=prof:true,...` is set, this module can trigger heap dumps
//! at strategic points (after merge passes, on shutdown, etc.).
//!
//! Usage:
//!   - Set env `MALLOC_CONF=prof:true,lg_prof_sample:19` to enable profiling.
//!   - Call `dump_heap_profile("label")` to write a `.heap` file.
//!   - Analyze with `jeprof --svg /path/to/binary /path/to/file.heap > out.svg`

use std::ffi::CString;
use std::path::Path;
use std::sync::atomic::{AtomicBool, Ordering};

use tikv_jemalloc_ctl::raw;

/// Whether jemalloc profiling is active (checked once at startup).
static PROF_ACTIVE: AtomicBool = AtomicBool::new(false);

/// Check if jemalloc profiling is enabled and cache the result.
pub fn init() {
    // Log jemalloc config env vars for diagnosis.
    // tikv-jemallocator uses prefixed symbols (_rjem_), so the env var is _RJEM_MALLOC_CONF.
    for var in ["_RJEM_MALLOC_CONF", "MALLOC_CONF"] {
        if let Ok(val) = std::env::var(var) {
            tracing::info!(%var, %val, "jemalloc config from env");
        }
    }

    let opt_prof = unsafe { raw::read::<bool>(b"opt.prof\0") };
    tracing::info!(?opt_prof, "jemalloc opt.prof (compiled-in profiling)");

    let active = match unsafe { raw::read::<bool>(b"prof.active\0") } {
        Ok(v) => v,
        Err(e) => {
            tracing::warn!("prof.active read failed: {e}");
            false
        }
    };
    PROF_ACTIVE.store(active, Ordering::Relaxed);
    if active {
        tracing::info!("jemalloc heap profiling is active");
    } else {
        tracing::info!("jemalloc heap profiling is NOT active");
    }
}

/// Dump a jemalloc heap profile to `{output_dir}/{label}.heap`.
/// No-op if profiling is not enabled.
pub fn dump_heap_profile(output_dir: &Path, label: &str) {
    if !PROF_ACTIVE.load(Ordering::Relaxed) {
        return;
    }

    let path = output_dir.join(format!("{label}.heap"));
    let path_str = match path.to_str() {
        Some(s) => s,
        None => {
            tracing::warn!("heap profile path is not valid UTF-8");
            return;
        }
    };

    // jemalloc expects a null-terminated C string for the dump path.
    let c_path = match CString::new(path_str) {
        Ok(c) => c,
        Err(_) => {
            tracing::warn!("heap profile path contains null byte");
            return;
        }
    };

    match unsafe { raw::write(b"prof.dump\0", c_path.as_ptr()) } {
        Ok(()) => {
            tracing::info!(path = %path.display(), "dumped heap profile");
        }
        Err(e) => {
            tracing::warn!(path = %path.display(), "failed to dump heap profile: {e}");
        }
    }
}
