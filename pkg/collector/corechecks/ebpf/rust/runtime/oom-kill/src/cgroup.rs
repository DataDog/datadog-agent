use aya_ebpf::helpers::bpf_get_current_task;

use crate::vmlinux::task_struct;

pub fn get_current_task() -> &'static task_struct {
    unsafe { &*(bpf_get_current_task() as *const task_struct) }
}
