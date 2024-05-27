use core::mem;
use aya_ebpf::{
    bindings::BPF_NOEXIST, macros::map, maps::HashMap
};

/*
 * The `oom_stats` hash map is used to share with the userland program system-probe
 * the statistics per pid
 */
#[map]
static oom_stats: HashMap<u32, OomStats> = HashMap::<u32, OomStats>::with_max_entries(10240, 0);

const TASK_COMM_LEN: usize = 16;

#[repr(C)]
pub struct OomStats {
    pub cgroup_name: [u8; 129],
    pub pid: u32,
    pub tpid: u32,
    pub fcomm: [u8; TASK_COMM_LEN],
    pub tcomm: [u8; TASK_COMM_LEN],
    pub pages: u64,
    pub memcg_oom: u32,
}

impl OomStats {
    #[inline(always)]
    pub fn from_map(pid: &u32) -> Result<&'static mut Self, i64> {
        oom_stats.insert(pid, &OomStats::zeroed(), BPF_NOEXIST.into())?;

        match oom_stats.get_ptr_mut(&pid) {
            Some(ptr) if !ptr.is_null() => Ok(unsafe { &mut *ptr } ),
            Some(_) | None => Err(0),
        }
    }

    #[inline(always)]
    fn zeroed() -> Self {
        unsafe { mem::zeroed() }
    }
}


