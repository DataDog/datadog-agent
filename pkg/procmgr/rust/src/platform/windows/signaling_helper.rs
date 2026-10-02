// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Isolated delivery of Windows console control signals.
//!
//! Generating CTRL_BREAK requires attachment to the workload's console. Console
//! attachment, the control-handler table, and standard-handle slots are
//! process-global: attaching the daemon to a workload's console would interfere
//! with concurrent logging and spawning. Detaching also invalidates console
//! handles, including duplicates, so restoring the daemon's standard slots or
//! serializing spawn and signal operations cannot preserve its startup objects.
//!
//! Instead, the daemon launches its own executable on demand in a detached,
//! internal helper mode. Only that helper attaches to the target console,
//! installs its break-ignore handler after attachment resets the handler table,
//! and generates CTRL_BREAK for the selected process group. The helper entry
//! runs without logging, SCM initialization, or an async runtime. This mechanism
//! is specific to Windows console controls; it does not implement Unix signaling
//! or other daemon shutdown responsibilities.
//!
//! A restricted inherited handle identifies the original target without reopening
//! a potentially reused PID. A dedicated report pipe survives console attachment
//! replacing the helper's standard slots. Its report distinguishes successful
//! event generation from target exit or signaling failure; generation alone is
//! not evidence that the workload has stopped.
//!
//! The daemon retains ownership of the workload job and process resources. It
//! remains responsible for main-process exit monitoring, the existing shutdown
//! timeouts, job closure, and forceful termination. The helper is assigned to the
//! target workload's job at creation, so job-wide termination and kill-on-close
//! contain it too. Creation is synchronous, like ordinary workload creation;
//! observing the report is asynchronous and does not wait for helper exit.

use std::ffi::OsString;
use std::io::Write;
use std::os::windows::io::FromRawHandle;
use std::time::{Duration, Instant};

use windows_sys::Win32::Foundation::{GetHandleInformation, GetLastError, HANDLE, INVALID_HANDLE_VALUE};
use windows_sys::Win32::System::Console::{
    AttachConsole, CTRL_BREAK_EVENT, GenerateConsoleCtrlEvent, SetConsoleCtrlHandler,
};
use windows_sys::Win32::System::Threading::GetProcessId;

use super::process::{ProcessWaitOutcome, wait_for_process_exit_ms};
use crate::handle::OwnedProcessHandle;

const MODE: &str = "--internal-console-signal";
const MAGIC: u32 = 0x44445347;
const VERSION: u32 = 1;
pub(crate) const REPORT_SIZE: usize = 24;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[repr(u32)]
pub(crate) enum Outcome { Generated = 1, AlreadyExited = 2, Failed = 3 }

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[repr(u32)]
pub(crate) enum Stage {
    None = 0, Validation = 1, ExitCheck = 2, Attachment = 3,
    HandlerInstallation = 4, Generation = 5,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) struct Report {
    pub(crate) outcome: Outcome,
    pub(crate) stage: Stage,
    pub(crate) error: u32,
    pub(crate) target_pid: u32,
}

impl Report {
    fn packet(self) -> [u8; REPORT_SIZE] {
        let fields = [MAGIC, VERSION, self.outcome as u32, self.stage as u32, self.error, self.target_pid];
        let mut bytes = [0; REPORT_SIZE];
        for (slot, value) in bytes.chunks_exact_mut(4).zip(fields) {
            slot.copy_from_slice(&value.to_le_bytes());
        }
        bytes
    }

    pub(crate) fn decode(bytes: [u8; REPORT_SIZE]) -> anyhow::Result<Self> {
        let fields: Vec<u32> = bytes.chunks_exact(4)
            .map(|b| u32::from_le_bytes(b.try_into().unwrap())).collect();
        anyhow::ensure!(fields[0] == MAGIC && fields[1] == VERSION, "invalid helper report header");
        let outcome = match fields[2] {
            1 => Outcome::Generated, 2 => Outcome::AlreadyExited, 3 => Outcome::Failed,
            _ => anyhow::bail!("invalid helper outcome"),
        };
        let stage = match fields[3] {
            0 => Stage::None, 1 => Stage::Validation, 2 => Stage::ExitCheck,
            3 => Stage::Attachment, 4 => Stage::HandlerInstallation, 5 => Stage::Generation,
            _ => anyhow::bail!("invalid helper failure stage"),
        };
        anyhow::ensure!(
            (outcome == Outcome::Failed && stage != Stage::None)
                || (outcome != Outcome::Failed && stage == Stage::None && fields[4] == 0 && fields[5] != 0),
            "inconsistent helper report"
        );
        Ok(Self { outcome, stage, error: fields[4], target_pid: fields[5] })
    }
}

/// The only public helper API: returns an exit code if the internal mode was
/// requested, including malformed requests. Never falls through to the daemon.
pub fn dispatch_internal_console_signal() -> Option<i32> {
    dispatch(std::env::args_os().skip(1).collect())
}

fn dispatch(args: Vec<OsString>) -> Option<i32> {
    if args.first().is_none_or(|arg| arg != MODE) { return None; }
    Some(run_helper(&args[1..]).unwrap_or(2))
}

fn parse_handle(arg: &OsString) -> Option<HANDLE> {
    let value = arg.to_str()?.parse::<usize>().ok()?;
    let handle = value as HANDLE;
    if handle.is_null() || handle == INVALID_HANDLE_VALUE { return None; }
    let mut flags = 0;
    (unsafe { GetHandleInformation(handle, &mut flags) } != 0).then_some(handle)
}

fn run_helper(args: &[OsString]) -> Option<i32> {
    if args.len() != 2 { return None; }
    let report_handle = parse_handle(&args[1])?;
    // A dedicated pipe survives attachment even when Windows replaces all
    // three standard slots. Do not emit any diagnostics through those slots.
    let mut writer = unsafe { std::fs::File::from_raw_handle(report_handle) };
    let report = match parse_handle(&args[0]) {
        Some(target) if target != report_handle => {
            let target = unsafe { OwnedProcessHandle::from_raw(target) };
            signal(target.get())
        }
        _ => Report { outcome: Outcome::Failed, stage: Stage::Validation, error: 6, target_pid: 0 },
    };
    if writer.write_all(&report.packet()).is_err() { return Some(3); }
    if report.outcome == Outcome::Generated {
        // Publish generation before settling; death/hang afterward cannot undo it.
        std::thread::sleep(Duration::from_millis(200));
    }
    Some(if report.outcome == Outcome::Failed { 1 } else { 0 })
}

fn failed(stage: Stage, pid: u32, error: u32) -> Report {
    Report { outcome: Outcome::Failed, stage, error, target_pid: pid }
}

fn check_exit(target: HANDLE, pid: u32) -> Result<(), Report> {
    match wait_for_process_exit_ms(target, 0) {
        Ok(ProcessWaitOutcome::TimedOut) => Ok(()),
        Ok(ProcessWaitOutcome::Exited(_)) => Err(Report {
            outcome: Outcome::AlreadyExited, stage: Stage::None, error: 0, target_pid: pid,
        }),
        Err(_) => Err(failed(Stage::ExitCheck, pid, unsafe { GetLastError() })),
    }
}

unsafe extern "system" fn ignore_break(event: u32) -> i32 {
    if event == CTRL_BREAK_EVENT { 1 } else { 0 }
}

fn signal(target: HANDLE) -> Report {
    let pid = unsafe { GetProcessId(target) };
    if pid == 0 { return failed(Stage::Validation, 0, unsafe { GetLastError() }); }
    if let Err(report) = check_exit(target, pid) { return report; }
    let deadline = Instant::now() + Duration::from_millis(500);
    loop {
        if let Err(report) = check_exit(target, pid) { return report; }
        if unsafe { AttachConsole(pid) } != 0 { break; }
        let error = unsafe { GetLastError() };
        if Instant::now() >= deadline { return failed(Stage::Attachment, pid, error); }
        std::thread::sleep(Duration::from_millis(10));
    }
    // AttachConsole resets the handler table, so install only after attachment.
    if unsafe { SetConsoleCtrlHandler(Some(ignore_break), 1) } == 0 {
        return failed(Stage::HandlerInstallation, pid, unsafe { GetLastError() });
    }
    if let Err(report) = check_exit(target, pid) { return report; }
    if unsafe { GenerateConsoleCtrlEvent(CTRL_BREAK_EVENT, pid) } == 0 {
        return failed(Stage::Generation, pid, unsafe { GetLastError() });
    }
    Report { outcome: Outcome::Generated, stage: Stage::None, error: 0, target_pid: pid }
}

use std::io::Read;
use crate::handle::RetainedProcessHandle;
use super::JobObject;

#[derive(Default)]
struct LaunchOptions {
    #[cfg(test)]
    executable: Option<std::path::PathBuf>,
    #[cfg(test)]
    hang_without_report: bool,
}

pub(crate) struct SignalingHelper {
    backend: HelperBackend,
}

enum HelperBackend {
    Process(HelperProcess),
    #[cfg(test)]
    Injected(Option<Report>),
}

/// Initiate Windows graceful signaling using the original process identity.
/// Create the helper synchronously in the workload job. The waiting half of the
/// stop observes delivery separately; this call does not wait for a report or exit.
pub(crate) fn send_graceful_stop(
    target: &RetainedProcessHandle,
    job: &JobObject,
) -> anyhow::Result<SignalingHelper> {
    launch_helper(target, job, LaunchOptions::default())
}

/// Resolve delivery in the daemon's waiting phase. This bounds only the helper
/// operation; the caller retains its existing workload grace and force-kill waits.
pub(crate) async fn wait_for_graceful_stop(
    helper: &mut SignalingHelper,
    name: &str,
    timeout: Duration,
) -> bool {
    let deadline = tokio::time::Instant::now() + timeout.min(Duration::from_secs(2));
    loop {
        let observation = helper.observe();
        if let Some(exit) = observation.exit
            && !matches!(exit, Ok(0))
        {
            log::warn!("[{name}] abnormal signaling helper exit: {exit:?}");
        }
        if let Some(result) = observation.report {
            return match result {
                Ok(report) if report.outcome == Outcome::Generated => {
                    log::info!("[{name}] CTRL_BREAK event generated for original target {}", report.target_pid);
                    true
                }
                Ok(report) if report.outcome == Outcome::AlreadyExited => {
                    log::info!("[{name}] original target already exited before signaling");
                    false
                }
                Ok(report) => {
                    log::warn!("[{name}] signaling failed at {:?}: Win32 {}", report.stage, report.error);
                    false
                }
                Err(error) => {
                    log::warn!("[{name}] signaling failure or timeout: {error}");
                    false
                }
            };
        }
        let now = tokio::time::Instant::now();
        if now >= deadline {
            log::warn!("[{name}] signaling helper report timed out");
            return false;
        }
        tokio::time::sleep_until((now + Duration::from_millis(10)).min(deadline)).await;
    }
}

struct HelperProcess {
    process: OwnedProcessHandle,
    reader: std::fs::File,
    report: Option<Result<Report, String>>,
}

pub(crate) struct HelperObservation {
    pub(crate) report: Option<Result<Report, String>>,
    pub(crate) exit: Option<Result<u32, String>>,
}

impl SignalingHelper {
    #[cfg(test)]
    pub(crate) fn test_unreported() -> Self {
        Self { backend: HelperBackend::Injected(None) }
    }

    #[cfg(test)]
    pub(crate) fn test_report(outcome: Outcome) -> Self {
        Self { backend: HelperBackend::Injected(Some(Report {
            outcome, stage: if outcome == Outcome::Failed { Stage::Attachment } else { Stage::None },
            error: if outcome == Outcome::Failed { 5 } else { 0 }, target_pid: 42,
        })) }
    }

    fn observe(&mut self) -> HelperObservation {
        match &mut self.backend {
            #[cfg(test)]
            HelperBackend::Injected(report) => HelperObservation {
                report: report.map(Ok), exit: None,
            },
            HelperBackend::Process(helper) => {
                let exit = match wait_for_process_exit_ms(helper.process.get(), 0) {
                    Ok(ProcessWaitOutcome::TimedOut) => None,
                    Ok(ProcessWaitOutcome::Exited(code)) => Some(Ok(code)),
                    Err(error) => Some(Err(error.to_string())),
                };
                if helper.report.is_none() {
                    helper.report = helper.read_report(exit.is_some());
                }
                HelperObservation { report: helper.report.clone(), exit }
            }
        }
    }
}

impl HelperProcess {
    fn read_report(&mut self, exited: bool) -> Option<Result<Report, String>> {
        use std::os::windows::io::AsRawHandle;
        use windows_sys::Win32::System::Pipes::PeekNamedPipe;
        let mut available = 0;
        let ok = unsafe {
            PeekNamedPipe(self.reader.as_raw_handle(), std::ptr::null_mut(), 0,
                std::ptr::null_mut(), &mut available, std::ptr::null_mut())
        };
        if ok == 0 { return Some(Err(format!("helper report unavailable: {}", std::io::Error::last_os_error()))); }
        if available < REPORT_SIZE as u32 {
            return exited.then(|| Err("helper exited without a complete report".into()));
        }
        // The sole writer writes one 24-byte packet. With all bytes available,
        // this read cannot block waiting for startup, attachment, or settling.
        let mut bytes = [0; REPORT_SIZE];
        Some(self.reader.read_exact(&mut bytes).map_err(|e| e.to_string())
            .and_then(|_| Report::decode(bytes).map_err(|e| e.to_string())))
    }
}

fn launch_helper(
    target: &RetainedProcessHandle, job: &JobObject, options: LaunchOptions,
) -> anyhow::Result<SignalingHelper> {
    use std::os::windows::io::AsRawHandle;
    use windows_sys::Win32::Foundation::{HANDLE_FLAG_INHERIT, SetHandleInformation};
    use windows_sys::Win32::Security::SECURITY_ATTRIBUTES;
    use windows_sys::Win32::System::Pipes::CreatePipe;
    use windows_sys::Win32::System::Threading::{
        CREATE_NEW_PROCESS_GROUP, DETACHED_PROCESS, EXTENDED_STARTUPINFO_PRESENT,
        CreateProcessW, PROCESS_INFORMATION,
    };
    use super::spawn::StartupInfoEx;

    #[cfg(test)]
    let executable = options.executable.unwrap_or_else(|| crate::test_helpers::graceful_sleeper_exe().into());
    #[cfg(not(test))]
    let executable = { let _ = options; std::env::current_exe()? };
    anyhow::ensure!(executable.is_absolute(), "helper executable must be absolute");
    use anyhow::Context;
    let restricted = target.duplicate_for_helper().context("duplicate helper target")?;
    let mut reader = std::ptr::null_mut();
    let mut writer = std::ptr::null_mut();
    let attributes = SECURITY_ATTRIBUTES {
        nLength: std::mem::size_of::<SECURITY_ATTRIBUTES>() as u32,
        lpSecurityDescriptor: std::ptr::null_mut(), bInheritHandle: 1,
    };
    if unsafe { CreatePipe(&mut reader, &mut writer, &attributes, 0) } == 0 {
        return Err(std::io::Error::last_os_error()).context("create report pipe");
    }
    let reader = unsafe { std::fs::File::from_raw_handle(reader) };
    let writer = unsafe { std::fs::File::from_raw_handle(writer) };
    if unsafe { SetHandleInformation(reader.as_raw_handle(), HANDLE_FLAG_INHERIT, 0) } == 0 {
        return Err(std::io::Error::last_os_error()).context("disable report reader inheritance");
    }
    let mut startup = StartupInfoEx::with_handles_and_job(
        vec![restricted.get(), writer.as_raw_handle()], job.raw_handle(),
    )?;
    let mut command = super::wide::null_terminated(&{
        #[cfg(test)]
        if options.hang_without_report {
            format!("\"{}\" --internal-console-hang", executable.display())
        } else {
            format!("\"{}\" {MODE} {} {}", executable.display(), restricted.get() as usize, writer.as_raw_handle() as usize)
        }
        #[cfg(not(test))]
        format!("\"{}\" {MODE} {} {}", executable.display(), restricted.get() as usize, writer.as_raw_handle() as usize)
    });
    let application: Vec<u16> = {
        use std::os::windows::ffi::OsStrExt;
        executable.as_os_str().encode_wide().chain(Some(0)).collect()
    };
    let mut info: PROCESS_INFORMATION = unsafe { std::mem::zeroed() };
    let ok = unsafe {
        CreateProcessW(application.as_ptr(), command.as_mut_ptr(), std::ptr::null(), std::ptr::null(),
            1, DETACHED_PROCESS | CREATE_NEW_PROCESS_GROUP | EXTENDED_STARTUPINFO_PRESENT,
            std::ptr::null(), std::ptr::null(), startup.startup_info(), &mut info)
    };
    if ok == 0 { return Err(std::io::Error::last_os_error()).context("CreateProcessW(helper)"); }
    let process = unsafe { OwnedProcessHandle::from_raw(info.hProcess) };
    drop(unsafe { OwnedProcessHandle::from_raw(info.hThread) });
    drop(writer);
    drop(restricted);
    Ok(SignalingHelper { backend: HelperBackend::Process(HelperProcess { process, reader, report: None }) })
}

#[cfg(test)]
mod tests {
    use super::*;

    fn exited_target() -> RetainedProcessHandle {
        use std::os::windows::io::AsRawHandle;
        let mut child = std::process::Command::new("cmd.exe").args(["/C", "exit 0"]).spawn().unwrap();
        let handle = crate::handle::ProcessHandle::from_borrowed(child.id(), child.as_raw_handle()).unwrap();
        let retained = handle.retain_for_shutdown().unwrap();
        child.wait().unwrap();
        retained
    }

    #[tokio::test]
    async fn helper_is_created_in_job_without_waiting_for_delivery() {
        use windows_sys::Win32::System::JobObjects::IsProcessInJob;
        let job = JobObject::new().unwrap();
        // The explicit hang mode intentionally never reports.
        let mut helper = launch_helper(&exited_target(), &job, LaunchOptions {
            hang_without_report: true, ..Default::default()
        }).unwrap();
        let HelperBackend::Process(process) = &helper.backend else { panic!("expected native helper"); };
        let mut member = 0;
        assert_ne!(unsafe { IsProcessInJob(process.process.get(), job.raw_handle(), &mut member) }, 0);
        assert_ne!(member, 0);
        assert!(helper.observe().report.is_none(), "creation must not wait for delivery");
        job.terminate().unwrap();
        assert!(job.wait_for_drain(tokio::time::Instant::now() + Duration::from_secs(5)).await.unwrap());
    }

    #[test]
    fn launch_failure_is_returned_synchronously() {
        let job = JobObject::new().unwrap();
        assert!(launch_helper(&exited_target(), &job, LaunchOptions {
            executable: Some(std::env::temp_dir().join("nonexistent-console-helper.exe")),
            ..Default::default()
        }).is_err());
    }

    #[tokio::test]
    async fn signal_sender_accepts_retained_identity_after_original_exit() {
        use windows_sys::Win32::System::Threading::GetProcessId;
        let target = exited_target();
        let pid = unsafe { GetProcessId(target.raw()) };
        assert_ne!(pid, 0);
        let job = JobObject::new().unwrap();
        let _helper = send_graceful_stop(&target, &job).expect("helper created");
        assert_eq!(unsafe { GetProcessId(target.raw()) }, pid);
        job.terminate().unwrap();
        assert!(job.wait_for_drain(tokio::time::Instant::now() + Duration::from_secs(5)).await.unwrap());
    }

    #[tokio::test]
    async fn wait_stage_resolves_delivery_without_a_workload_deadline_policy() {
        for outcome in [Outcome::Generated, Outcome::AlreadyExited, Outcome::Failed] {
            let mut helper = SignalingHelper::test_report(outcome);
            assert_eq!(wait_for_graceful_stop(&mut helper, "delivery-test", Duration::from_secs(90)).await,
                outcome == Outcome::Generated);
        }
        let mut unreported = SignalingHelper::test_unreported();
        assert!(!wait_for_graceful_stop(&mut unreported, "delivery-test", Duration::ZERO).await);
    }

    #[tokio::test]
    async fn report_timeout_leaves_created_helper_termination_to_job_close() {
        let job = JobObject::new().unwrap();
        let mut helper = launch_helper(&exited_target(), &job, LaunchOptions {
            hang_without_report: true, ..Default::default()
        }).unwrap();
        assert!(!wait_for_graceful_stop(&mut helper, "hung-helper", Duration::ZERO).await);
        let HelperBackend::Process(process) = &helper.backend else { panic!("expected native helper"); };
        assert!(matches!(wait_for_process_exit_ms(process.process.get(), 0).unwrap(), ProcessWaitOutcome::TimedOut),
            "report timeout must not terminate an existing helper separately");
        drop(job);
        let deadline = tokio::time::Instant::now() + Duration::from_secs(5);
        while matches!(wait_for_process_exit_ms(process.process.get(), 0).unwrap(), ProcessWaitOutcome::TimedOut) {
            assert!(tokio::time::Instant::now() < deadline, "job close did not terminate helper");
            tokio::time::sleep(Duration::from_millis(10)).await;
        }
    }

    #[test]
    fn fixture_rejects_unknown_and_malformed_modes() {
        let executable = crate::test_helpers::graceful_sleeper_exe();
        for args in [
            vec!["--unknown"],
            vec!["--internal-console-signal"],
            vec!["--internal-console-signal", "bad", "bad"],
            vec!["--internal-console-hang", "unexpected"],
        ] {
            let status = std::process::Command::new(&executable).args(args).status().unwrap();
            assert_eq!(status.code(), Some(2));
        }
    }

    #[test]
    fn malformed_internal_mode_never_starts_daemon() {
        assert_eq!(dispatch(vec![MODE.into()]), Some(2));
        assert_eq!(dispatch(vec![MODE.into(), "-1".into(), "0".into()]), Some(2));
        assert_eq!(dispatch(vec![MODE.into(), "18446744073709551616".into(), "0".into()]), Some(2));
        assert_eq!(dispatch(vec![]), None);
    }

    #[test]
    fn report_contract_rejects_malformed_generation() {
        let report = Report { outcome: Outcome::Generated, stage: Stage::None, error: 0, target_pid: 42 };
        assert_eq!(Report::decode(report.packet()).unwrap(), report);
        for index in [0, 4, 8, 12, 16, 20] {
            let mut bytes = report.packet();
            bytes[index..index + 4].copy_from_slice(&99u32.to_le_bytes());
            if index != 20 { assert!(Report::decode(bytes).is_err()); }
        }
        assert!(Report::decode([0; REPORT_SIZE]).is_err());
    }
}
