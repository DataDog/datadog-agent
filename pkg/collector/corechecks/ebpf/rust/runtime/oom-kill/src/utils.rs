use aya_ebpf::helpers::bpf_get_current_pid_tgid;

#[inline(always)]
pub fn get_pid() -> Result<u32, i64> {
    Ok((bpf_get_current_pid_tgid() >> 32).try_into().unwrap_or(0))
}


