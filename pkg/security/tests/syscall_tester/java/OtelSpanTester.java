/*
 * Unless explicitly stated otherwise all files in this repository are licensed
 * under the Apache License Version 2.0.
 * This product includes software developed at Datadog (https://www.datadoghq.com/).
 * Copyright 2016-present Datadog, Inc.
 */

import datadog.trace.api.GlobalTracer;
import datadog.trace.api.Trace;

import java.io.FileWriter;
import java.nio.file.Files;
import java.nio.file.Path;
import java.nio.file.Paths;

/**
 * OtelSpanTester drives the CWS OTel Thread-Local Context (OTEP #4947) reader
 * end-to-end with a real dd-trace-java agent.
 *
 * When launched under {@code -javaagent:dd-java-agent.jar} with profiling and
 * CWS enabled, the bundled java-profiler (ddprof) publishes the active span's
 * trace/span IDs into the thread-local {@code otel_thread_ctx_v1} record, and
 * the agent creates the {@code datadog-tracer-info-} memfd that triggers the
 * system-probe reader. See the validated launch recipe in span_test.go.
 *
 * Because a real tracer assigns trace/span IDs (they cannot be forced to fixed
 * constants like the C testers), this program reports the IDs it is running
 * under so the test can match them against the captured span context.
 *
 * A two-phase handshake keeps the JVM warm-up out of the test's event-timeout
 * window:
 *   1. Boot, open a span, spin briefly so the wall profiler registers this
 *      thread (which publishes otel_thread_ctx_v1), then write the reported
 *      IDs to reportFile.
 *   2. Wait for triggerFile, then open readyFile (lets the agent snapshot the
 *      tracer), wait for continueFile, then open testFile (the span-carrying
 *      open the CWS rule matches). The span stays active on this thread for
 *      the whole method, so the record is valid at testFile open time.
 *
 * args: reportFile triggerFile readyFile continueFile testFile
 */
public final class OtelSpanTester {
    private static final long WAIT_TIMEOUT_MS = 60_000L;

    public static void main(String[] args) throws Exception {
        if (args.length != 5) {
            throw new IllegalArgumentException(
                "usage: OtelSpanTester <reportFile> <triggerFile> <readyFile> <continueFile> <testFile>");
        }
        Thread.currentThread().setName("otel-span-tester");
        // Give the ddprof wall profiler time to start before opening the span.
        Thread.sleep(Long.getLong("otel.startup.sleep.ms", 4_000L));
        runInSpan(args[0], args[1], args[2], args[3], args[4]);
    }

    @Trace(operationName = "cws-otel-span-open")
    static void runInSpan(String reportFile, String triggerFile, String readyFile,
                          String continueFile, String testFile) throws Exception {
        // Spend CPU inside the active span so the wall-clock profiler samples
        // and registers this thread; that publishes the otel_thread_ctx_v1 TLS
        // record with the current trace/span context.
        long acc = 0;
        for (long i = 0; i < 200_000_000L; i++) {
            acc += i;
        }
        if (acc == Long.MIN_VALUE) {
            System.out.print(""); // keep the loop from being optimized away
        }

        String traceID = GlobalTracer.get().getTraceId();
        String spanID = GlobalTracer.get().getSpanId();
        Files.write(Paths.get(reportFile), (traceID + " " + spanID + "\n").getBytes());
        System.out.println("otel-span-tester reported traceID=" + traceID + " spanID=" + spanID);
        System.out.flush();

        // Phase 2: fast opens only, inside the test's event-timeout window.
        waitForFile(triggerFile);
        touch(readyFile);
        waitForFile(continueFile);
        touch(testFile);
    }

    private static void touch(String path) throws Exception {
        try (FileWriter writer = new FileWriter(path)) {
            writer.write("x");
        }
    }

    private static void waitForFile(String path) throws Exception {
        Path p = Paths.get(path);
        long deadline = System.currentTimeMillis() + WAIT_TIMEOUT_MS;
        while (System.currentTimeMillis() < deadline) {
            if (Files.exists(p)) {
                return;
            }
            Thread.sleep(20);
        }
        throw new RuntimeException("timed out waiting for " + path);
    }
}
