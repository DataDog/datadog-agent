// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

use criterion::{criterion_group, criterion_main, BenchmarkId, Criterion};
use dd_preprocessor_tokenizer::profile;
use dd_preprocessor_tokenizer::Tokenizer;

// Production log corpus matching Go's loadBenchCorpus in tokenizer_load_benchmark_test.go
static CORPUS: &[&[u8]] = &[
    b"2024-01-15T10:30:45.123Z INFO [service-name] Request processed successfully user_id=12345 duration_ms=42 path=/api/v1/users",
    b"2024-01-15 10:30:46,789 INFO c.e.s.UserService - Authentication successful session=abc123def ip=192.168.1.100",
    b"Jan 15 10:30:47 web-01 nginx: 192.168.1.100 - - [15/Jan/2024:10:30:47 -0800] \"GET /api/v1/health HTTP/1.1\" 200 15",
    br#"{"timestamp":"2024-01-15T10:30:48.000Z","level":"INFO","service":"payment","message":"Transaction completed","amount":99.99,"currency":"USD"}"#,
    b"2024-01-15 10:30:49.111 [pool-1-thread-15] DEBUG c.e.s.CacheManager - Cache hit key=user:5678 ttl=300 store=redis",
    b"2024-01-15 10:30:50.222 WARN [service-name] Connection pool low available=2 max=50 wait_time_ms=150",
    b"Mon Jan 15 10:30:51 PST 2024 | audit | user=admin action=delete resource=record id=9999 result=ok",
    b"2024-01-15T10:30:52.333Z DEBUG [grpc-server] method=GetUser status=OK latency=5ms peer=10.0.0.1:52341",
    b"2024-01-15T10:30:53.444Z ERROR [service-name] exception=NullPointerException user_id=12345 request_id=req-abc-123",
    b"at com.example.Service.handleRequest(Service.java:123) at com.example.Controller.process(Controller.java:456)",
    b"cpu=45.67 memory=2048 disk=512000 network_tx=1234567890 network_rx=9876543210 connections=100 threads=32",
    b"2024-01-15 10:30:54 health status=ok latency_p99=12ms connections=128 workers=16 queue_depth=0",
    br#"2024-01-15T10:30:55.555Z stdout F {"log":"Starting container","stream":"stdout","time":"2024-01-15T10:30:55.555Z"}"#,
    b"/var/log/containers/web-deployment-abc123_default_web-abc123.log 2024-01-15T10:30:56Z INFO ready",
    br#"192.168.1.100 - user123 [15/Jan/2024:10:30:57 -0800] "GET /api/v1/users/profile?id=12345&filter=active HTTP/1.1" 200 4567 "https://example.com" "Mozilla/5.0""#,
    br#"10.0.0.1 - - [15/Jan/2024:10:30:58 +0000] "POST /api/v2/events HTTP/2.0" 202 0 "-" "datadog-agent/7.50.0""#,
];

fn bench_tokenizer_by_window(c: &mut Criterion) {
    let mut group = c.benchmark_group("tokenizer_window");

    for &(name, max_eval) in &[("labeler_60B", 60), ("sampler_2048B", 2048), ("unlimited", 0)] {
        group.bench_function(BenchmarkId::new("rust", name), |b| {
            let tokenizer = Tokenizer::new(max_eval);
            let mut i = 0;
            b.iter(|| {
                let input = CORPUS[i % CORPUS.len()];
                i += 1;
                tokenizer.tokenize(input)
            });
        });
    }
    group.finish();
}

fn bench_tokenizer_by_input(c: &mut Criterion) {
    let mut group = c.benchmark_group("tokenizer_input");

    let cases: &[(&str, &[u8])] = &[
        ("short_6B", b"abc123"),
        (
            "medium_78B",
            b"2024-01-15T10:30:45.123Z INFO [service-name] Request processed successfully",
        ),
        (
            "long_500B",
            b"2024-01-15T10:30:45.123Z ERROR [payment-service] Transaction failed: timeout after 30000ms waiting for upstream response from payment-gateway.internal.example.com:8443 | correlation_id=abc-123-def-456 | customer_id=98765 | amount=1234.56 USD | retry_count=3 | last_error=connection_reset_by_peer | trace_id=0af7651916cd43dd8448eb211c80319c | span_id=b7ad6b7169203331 | flags=01 | service.version=2.14.3 | deployment.environment=production-us-east-1",
        ),
    ];

    for &(name, input) in cases {
        group.bench_function(BenchmarkId::new("rust", name), |b| {
            let tokenizer = Tokenizer::new(0);
            b.iter(|| tokenizer.tokenize(input));
        });
    }
    group.finish();
}

fn bench_tokenizer_keyword_heavy(c: &mut Criterion) {
    let mut group = c.benchmark_group("tokenizer_keywords");

    let months_days = b"JAN FEB MAR APR MAY JUN JUL AUG SEP OCT NOV DEC MON TUE WED THU FRI SAT SUN AM PM";
    let timezones = b"UTC GMT EST EDT CST CDT MST MDT PST PDT JST KST IST MSK CET BST HST HDT NST NDT CEST NZST NZDT";
    let severity = b"FATAL ERROR PANIC ALERT SEVERE WARN WARNING CRIT CRITICAL EMERG EMERGENCY EXCEPTION CRASH CRASHED FAILED FAILURE DEADLOCK TIMEOUT";

    group.bench_function("months_days", |b| {
        let tokenizer = Tokenizer::new(0);
        b.iter(|| tokenizer.tokenize(months_days));
    });
    group.bench_function("timezones", |b| {
        let tokenizer = Tokenizer::new(0);
        b.iter(|| tokenizer.tokenize(timezones));
    });
    group.bench_function("severity", |b| {
        let tokenizer = Tokenizer::new(0);
        b.iter(|| tokenizer.tokenize(severity));
    });
    group.finish();
}

fn bench_profiling_variants(c: &mut Criterion) {
    let mut group = c.benchmark_group("profiling");

    // Compare: AC (current) vs LUT-only vs Switch vs AC-bulk on corpus at 60B
    for &(name, max_eval) in &[("60B", 60usize), ("2048B", 2048)] {
        group.bench_function(BenchmarkId::new("ac_per_emit", name), |b| {
            let tokenizer = Tokenizer::new(max_eval);
            let mut i = 0;
            b.iter(|| {
                let input = &CORPUS[i % CORPUS.len()][..CORPUS[i % CORPUS.len()].len().min(max_eval)];
                i += 1;
                tokenizer.tokenize(input)
            });
        });

        group.bench_function(BenchmarkId::new("lut_only", name), |b| {
            let mut i = 0;
            b.iter(|| {
                let input = &CORPUS[i % CORPUS.len()][..CORPUS[i % CORPUS.len()].len().min(max_eval)];
                i += 1;
                profile::tokenize_lut_only(input)
            });
        });

        group.bench_function(BenchmarkId::new("switch_cascade", name), |b| {
            let mut i = 0;
            b.iter(|| {
                let input = &CORPUS[i % CORPUS.len()][..CORPUS[i % CORPUS.len()].len().min(max_eval)];
                i += 1;
                profile::tokenize_switch(input)
            });
        });

        // ac_bulk omitted: find_overlapping_iter is incompatible with LeftmostLongest
    }
    group.finish();
}

criterion_group!(
    benches,
    bench_tokenizer_by_window,
    bench_tokenizer_by_input,
    bench_tokenizer_keyword_heavy,
    bench_profiling_variants,
);
criterion_main!(benches);
