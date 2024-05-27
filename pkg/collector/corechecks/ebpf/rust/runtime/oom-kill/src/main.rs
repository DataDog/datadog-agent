#![no_std]
#![no_main]
#![allow(non_snake_case)]
#![allow(non_upper_case_globals)]

mod cgroup;
mod utils;
mod stats;
#[allow(dead_code, non_camel_case_types)]
mod vmlinux;

use aya_ebpf::bpf_printk;
use aya_ebpf::macros::kprobe;
use aya_ebpf::programs::ProbeContext;

use cgroup::get_current_task;
use utils::get_pid;
use stats::OomStats;

#[kprobe(function = "oom_kill_process")]
pub fn kprobe__oom_kill_process(_ctx: ProbeContext) -> i64 {
    unsafe {bpf_printk!(b"Hello from Rust");}
    try_oom_kill_process().ok();

    0
}

fn try_oom_kill_process() -> Result<(), i64> {
    let pid: u32 = get_pid()?;
    let stats = &mut OomStats::from_map(&pid)?;

    stats.pid = pid;

    let task = get_current_task();

    Ok(())
}

#[panic_handler]
fn panic(_info: &core::panic::PanicInfo) -> ! {
    unsafe { core::hint::unreachable_unchecked() }
}
