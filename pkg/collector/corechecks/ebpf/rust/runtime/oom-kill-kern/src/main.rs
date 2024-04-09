#![no_std]
#![no_main]
#![allow(non_snake_case)]
#![allow(non_upper_case_globals)]

use aya_ebpf::{
    macros::{kprobe, map},
    maps::HashMap,
    programs::ProbeContext,
};

#[map]
static oom_stats: HashMap<u32, u32> = HashMap::<u32, u32>::with_max_entries(10240, 0);

#[kprobe(function = "oom_kill_process")]
pub fn kprobe__oom_kill_process(_ctx: ProbeContext) -> i64 {
    0
}

#[panic_handler]
fn panic(_info: &core::panic::PanicInfo) -> ! {
    unsafe { core::hint::unreachable_unchecked() }
}
