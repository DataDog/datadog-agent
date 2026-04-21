use criterion::{criterion_group, criterion_main, Criterion, Throughput};
use std::time::Instant;
use tempfile::TempDir;
use tokio::io::{AsyncReadExt, AsyncWriteExt};
use tokio::net::{UnixListener, UnixStream};
use tokio::runtime::Runtime;

/// Spawn a Unix socket server that discards all received bytes.
/// Returns the `TempDir` (keep alive to preserve the socket path) and the socket path.
fn start_sink_server(rt: &Runtime) -> (TempDir, std::path::PathBuf) {
    let tmp = tempfile::tempdir().unwrap();
    let sock_path = tmp.path().join("bench_transport.sock");

    let listener = rt.block_on(async { UnixListener::bind(&sock_path).unwrap() });

    rt.spawn(async move {
        loop {
            if let Ok((mut stream, _)) = listener.accept().await {
                tokio::spawn(async move {
                    let mut buf = vec![0u8; 65536];
                    loop {
                        match stream.read(&mut buf).await {
                            Ok(0) | Err(_) => break,
                            Ok(_) => {}
                        }
                    }
                });
            }
        }
    });

    (tmp, sock_path)
}

fn bench_transport(c: &mut Criterion) {
    let rt = Runtime::new().unwrap();
    let (_tmp, sock_path) = start_sink_server(&rt);

    // Frame sizes representative of real pipeline traffic:
    //   64B  — small metric batch
    //   4KB  — medium batch (~50 metrics)
    //   64KB — large log batch
    let frame_cases: &[(&str, usize)] = &[("64B", 64), ("4KB", 4 * 1024), ("64KB", 64 * 1024)];

    let mut group = c.benchmark_group("unix_socket_write");

    for &(name, frame_size) in frame_cases {
        let payload = vec![0xABu8; frame_size];
        group.throughput(Throughput::Bytes(frame_size as u64));

        group.bench_function(name, |b| {
            let payload = payload.clone();
            let sock_path = sock_path.clone();
            // iter_custom: create one connection per measurement window, then
            // perform `iters` writes. Connection setup time is excluded from the
            // measurement.
            b.iter_custom(|iters| {
                let payload = payload.clone();
                let sock_path = sock_path.clone();
                rt.block_on(async move {
                    let mut stream = UnixStream::connect(&sock_path).await.unwrap();
                    let start = Instant::now();
                    for _ in 0..iters {
                        stream.write_all(&payload).await.unwrap();
                    }
                    let elapsed = start.elapsed();
                    let _ = stream.shutdown().await;
                    elapsed
                })
            });
        });
    }

    group.finish();
}

criterion_group!(benches, bench_transport);
criterion_main!(benches);
