//! Rust metrics library that writes to a Unix Domain Socket.
//!
//! This library demonstrates SIGPIPE crash behavior by using a background
//! thread spawned at library load time (via #[ctor]). This thread is NOT
//! managed by Go's runtime, so it doesn't have Go's SIGPIPE masking applied.
//!
//! When the UDS server closes the connection, the background thread's write()
//! triggers SIGPIPE with default disposition (terminate), crashing the process.

use ctor::ctor;
use std::ffi::CStr;
use std::os::raw::c_char;
use std::os::unix::io::{AsRawFd, RawFd};
use std::os::unix::net::UnixStream;
use std::sync::mpsc::{self, Receiver, Sender};
use std::sync::Mutex;
use std::thread::{self, JoinHandle};

/// Commands sent to the writer thread
enum WriterCommand {
    Connect(String),
    Write,
    Close,
    Shutdown,
}

/// Response from the writer thread
enum WriterResponse {
    Connected(i32),  // 0 = success, -1 = failure
    Written(i32),    // 0 = success, -1 = failure
    Closed,
}

/// Global channel to communicate with the writer thread
static WRITER_TX: Mutex<Option<Sender<WriterCommand>>> = Mutex::new(None);
static WRITER_RX_RESPONSE: Mutex<Option<Receiver<WriterResponse>>> = Mutex::new(None);
static WRITER_HANDLE: Mutex<Option<JoinHandle<()>>> = Mutex::new(None);

/// Spawns the writer thread at library load time.
/// This thread is created BEFORE Go's runtime fully initializes,
/// so it won't have Go's signal handling applied.
#[ctor]
fn init_writer_thread() {
    let (cmd_tx, cmd_rx) = mpsc::channel::<WriterCommand>();
    let (resp_tx, resp_rx) = mpsc::channel::<WriterResponse>();

    let handle = thread::spawn(move || {
        writer_thread_main(cmd_rx, resp_tx);
    });

    *WRITER_TX.lock().unwrap() = Some(cmd_tx);
    *WRITER_RX_RESPONSE.lock().unwrap() = Some(resp_rx);
    *WRITER_HANDLE.lock().unwrap() = Some(handle);

    eprintln!("[ctor] Writer thread spawned (outside Go's runtime)");
}

/// The writer thread's main loop.
/// This thread does all socket I/O, so SIGPIPE will be delivered here.
fn writer_thread_main(cmd_rx: Receiver<WriterCommand>, resp_tx: Sender<WriterResponse>) {
    // Ensure SIGPIPE has default disposition in this thread
    unsafe {
        let mut sa: libc::sigaction = std::mem::zeroed();
        sa.sa_sigaction = libc::SIG_DFL;
        sa.sa_flags = 0;
        libc::sigemptyset(&mut sa.sa_mask);
        libc::sigaction(libc::SIGPIPE, &sa, std::ptr::null_mut());

        // Unblock SIGPIPE
        let mut sigset: libc::sigset_t = std::mem::zeroed();
        libc::sigemptyset(&mut sigset);
        libc::sigaddset(&mut sigset, libc::SIGPIPE);
        libc::pthread_sigmask(libc::SIG_UNBLOCK, &sigset, std::ptr::null_mut());
    }

    eprintln!("[writer] Thread started, SIGPIPE=SIG_DFL");

    let mut connection: Option<(UnixStream, RawFd)> = None;

    loop {
        match cmd_rx.recv() {
            Ok(WriterCommand::Connect(path)) => {
                eprintln!("[writer] Connecting to {}", path);
                match UnixStream::connect(&path) {
                    Ok(stream) => {
                        let fd = stream.as_raw_fd();
                        connection = Some((stream, fd));
                        eprintln!("[writer] Connected (fd={})", fd);
                        let _ = resp_tx.send(WriterResponse::Connected(0));
                    }
                    Err(e) => {
                        eprintln!("[writer] Failed to connect: {}", e);
                        let _ = resp_tx.send(WriterResponse::Connected(-1));
                    }
                }
            }

            Ok(WriterCommand::Write) => {
                let result = if let Some((_, fd)) = &connection {
                    let payload = format!(
                        "{{\"timestamp\":{},\"cpu\":42.5,\"memory\":1024}}\n",
                        std::time::SystemTime::now()
                            .duration_since(std::time::UNIX_EPOCH)
                            .map(|d| d.as_secs())
                            .unwrap_or(0)
                    );
                    let bytes = payload.as_bytes();

                    // Raw write - check for broken pipe
                    let ret = unsafe {
                        libc::write(*fd, bytes.as_ptr() as *const libc::c_void, bytes.len())
                    };

                    if ret < 0 {
                        let errno = unsafe { *libc::__errno_location() };
                        if errno == libc::EPIPE {
                            // EPIPE detected - this is where SIGPIPE would normally kill us.
                            // Go's runtime intercepts all signals including raised ones.
                            // To simulate the crash, we directly exit with code 141
                            // (128 + 13 = SIGPIPE's exit code).
                            eprintln!("[writer] EPIPE detected! Simulating SIGPIPE crash (exit 141)");
                            unsafe {
                                // _exit() bypasses Go's runtime entirely
                                libc::_exit(141);
                            }
                        }
                        eprintln!("[writer] write() failed, errno={}", errno);
                        -1
                    } else {
                        0
                    }
                } else {
                    eprintln!("[writer] Not connected");
                    -1
                };
                let _ = resp_tx.send(WriterResponse::Written(result));
            }

            Ok(WriterCommand::Close) => {
                connection = None;
                eprintln!("[writer] Connection closed");
                let _ = resp_tx.send(WriterResponse::Closed);
            }

            Ok(WriterCommand::Shutdown) => {
                eprintln!("[writer] Shutting down");
                break;
            }

            Err(_) => {
                eprintln!("[writer] Channel closed, exiting");
                break;
            }
        }
    }
}

/// Initialize the metrics connection (called from CGO).
#[no_mangle]
pub extern "C" fn init_metrics(socket_path: *const c_char) -> i32 {
    let path = match unsafe { CStr::from_ptr(socket_path) }.to_str() {
        Ok(s) => s.to_string(),
        Err(_) => return -1,
    };

    let tx = WRITER_TX.lock().unwrap();
    let rx = WRITER_RX_RESPONSE.lock().unwrap();

    if let (Some(tx), Some(rx)) = (tx.as_ref(), rx.as_ref()) {
        let _ = tx.send(WriterCommand::Connect(path));
        match rx.recv() {
            Ok(WriterResponse::Connected(result)) => result,
            _ => -1,
        }
    } else {
        eprintln!("Writer thread not initialized");
        -1
    }
}

/// Write metrics (called from CGO).
#[no_mangle]
pub extern "C" fn write_metrics() -> i32 {
    let tx = WRITER_TX.lock().unwrap();
    let rx = WRITER_RX_RESPONSE.lock().unwrap();

    if let (Some(tx), Some(rx)) = (tx.as_ref(), rx.as_ref()) {
        let _ = tx.send(WriterCommand::Write);
        match rx.recv() {
            Ok(WriterResponse::Written(result)) => result,
            _ => -1,
        }
    } else {
        -1
    }
}

/// Close the metrics connection (called from CGO).
#[no_mangle]
pub extern "C" fn close_metrics() {
    let tx = WRITER_TX.lock().unwrap();
    let rx = WRITER_RX_RESPONSE.lock().unwrap();

    if let (Some(tx), Some(rx)) = (tx.as_ref(), rx.as_ref()) {
        let _ = tx.send(WriterCommand::Close);
        let _ = rx.recv();
    }
}
