package com.datadoghq.e2e.logwriter;

import org.apache.logging.log4j.LogManager;
import org.apache.logging.log4j.Logger;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.scheduling.annotation.Scheduled;
import org.springframework.stereotype.Service;

import javax.annotation.PostConstruct;
import java.io.IOException;
import java.nio.file.Files;
import java.nio.file.Path;
import java.nio.file.Paths;
import java.nio.file.StandardOpenOption;
import java.time.Instant;
import java.time.temporal.ChronoUnit;
import java.time.ZoneOffset;
import java.time.format.DateTimeFormatter;
import java.util.UUID;
import java.util.Arrays;
import java.util.concurrent.atomic.AtomicLong;

/**
 * Produces production-shaped log files while accelerating Log4j2 time-based
 * rotation from one hour to one minute.
 */
@Service
public class LogWriterService {

    private static final Logger dataLog = LogManager.getLogger(LogWriterService.class);
    private static final Logger statusLog = LogManager.getLogger("logwriter.status");
    private static final DateTimeFormatter PERIOD_FORMAT =
            DateTimeFormatter.ofPattern("yyyyMMdd'T'HHmm'Z'").withZone(ZoneOffset.UTC);
    private static final String PAYLOAD =
            "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx" +
            "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx";

    private final AtomicLong sequence = new AtomicLong(0);
    private final String runId = readRunId();
    private final String host = System.getenv().getOrDefault("HOSTNAME", "unknown");

    @Value("${logwriter.target-bytes:83000}")
    private long targetBytes;

    @Value("${logwriter.target-bytes-sequence:}")
    private String targetBytesSequence;

    @Value("${logwriter.head-pause-ms:5000}")
    private long headPauseMillis;

    @Value("${logwriter.max-records-per-period:2000}")
    private int maxRecordsPerPeriod;

    @Value("${logwriter.initial-fill-runway-ms:10000}")
    private long initialFillRunwayMillis;

    private Path logPath;
    private String completedPeriod = "";
    private long[] parsedTargetBytes;
    private int completedPeriods;

    @PostConstruct
    public void init() {
        String logDir = System.getenv().getOrDefault("LOGWRITER_LOG_DIR", "/mnt/azure-files");
        logPath = Paths.get(logDir, "app.log");
        parsedTargetBytes = parseTargetBytesSequence(targetBytesSequence, targetBytes);
        statusLog.info(
                "writer_ready run_id={} path={} rotation=log4j2-minute target_bytes={} target_bytes_sequence={} head_pause_ms={}",
                runId,
                logPath,
                targetBytes,
                Arrays.toString(parsedTargetBytes),
                headPauseMillis);
    }

    /**
     * On the first scheduler tick in each UTC minute, emit one header record,
     * leave a five-second short-file window, then fill the active file to the
     * production-sized target. The first event in a new minute is what causes
     * Log4j2 to rename the prior active file and create its replacement.
     */
    @Scheduled(
            fixedDelayString = "${logwriter.interval-ms:250}",
            initialDelayString = "${logwriter.initial-delay-ms:1000}")
    public synchronized void writeCurrentPeriod() throws IOException, InterruptedException {
        String period = currentPeriod();
        if (period.equals(completedPeriod)) {
            return;
        }
        Instant now = Instant.now();
        if (sequence.get() == 0
                && shouldDeferInitialPeriod(now, headPauseMillis, initialFillRunwayMillis)) {
            return;
        }

        long periodTargetBytes = parsedTargetBytes[completedPeriods % parsedTargetBytes.length];
        writeRecord(period, "head", 1, periodTargetBytes);
        Thread.sleep(headPauseMillis);

        int records = 1;
        while (Files.size(logPath) < periodTargetBytes) {
            if (!period.equals(currentPeriod())) {
                statusLog.warn(
                        "period_changed_before_target run_id={} period={} records={} bytes={}",
                        runId,
                        period,
                        records,
                        Files.size(logPath));
                return;
            }
            if (records >= maxRecordsPerPeriod) {
                throw new IllegalStateException(
                        "active file did not reach target size before max records: " + logPath);
            }

            long remaining = periodTargetBytes - Files.size(logPath);
            if (remaining <= 512) {
                appendPadding(remaining);
                break;
            }

            records++;
            writeRecord(period, "fill", records, periodTargetBytes);
        }

        completedPeriod = period;
        completedPeriods++;
        statusLog.info(
                "period_complete run_id={} period={} target_bytes={} records={} bytes={} first_sequence={} last_sequence={}",
                runId,
                period,
                periodTargetBytes,
                records,
                Files.size(logPath),
                sequence.get() - records + 1,
                sequence.get());
    }

    private void writeRecord(String period, String phase, int recordInPeriod, long periodTargetBytes) {
        long currentSequence = sequence.incrementAndGet();
        String message = String.format(
                "run_id=%s period=%s sequence=%d record=%d phase=%s target_bytes=%d host=%s payload=%s",
                runId,
                period,
                currentSequence,
                recordInPeriod,
                phase,
                periodTargetBytes,
                host,
                PAYLOAD);

        if (currentSequence % 20 == 0) {
            dataLog.error(message);
        } else if (currentSequence % 10 == 0) {
            dataLog.warn(message);
        } else {
            dataLog.info(message);
        }
    }

    private void appendPadding(long bytes) throws IOException {
        if (bytes <= 0) {
            return;
        }
        if (bytes > 512) {
            throw new IllegalArgumentException("padding must not exceed 512 bytes");
        }
        byte[] padding = new byte[(int) bytes];
        Arrays.fill(padding, (byte) 'p');
        // The scheduled method is synchronized and Log4j uses immediate flush,
        // so this exact-size padding cannot race another write in this process.
        Files.write(logPath, padding, StandardOpenOption.APPEND);
    }

    static long[] parseTargetBytesSequence(String configured, long fallback) {
        if (configured == null || configured.trim().isEmpty()) {
            if (fallback <= 0) {
                throw new IllegalArgumentException("target bytes must be positive");
            }
            return new long[] {fallback};
        }

        String[] values = configured.split(",");
        long[] targets = new long[values.length];
        for (int index = 0; index < values.length; index++) {
            long value;
            try {
                value = Long.parseLong(values[index].trim());
            } catch (NumberFormatException error) {
                throw new IllegalArgumentException("invalid target byte sequence: " + configured, error);
            }
            if (value < 511 || value > 1024 * 1024) {
                throw new IllegalArgumentException("target byte sequence values must be between 511 and 1048576");
            }
            targets[index] = value;
        }
        return targets;
    }

    private static String currentPeriod() {
        return PERIOD_FORMAT.format(Instant.now());
    }

    static boolean shouldDeferInitialPeriod(
            Instant now, long headPauseMillis, long fillRunwayMillis) {
        Instant nextMinute = now.truncatedTo(ChronoUnit.MINUTES).plus(1, ChronoUnit.MINUTES);
        long remainingMillis = ChronoUnit.MILLIS.between(now, nextMinute);
        return remainingMillis <= headPauseMillis + fillRunwayMillis;
    }

    private static String readRunId() {
        String configured = System.getenv("LOGWRITER_RUN_ID");
        if (configured == null || configured.isEmpty()) {
            return UUID.randomUUID().toString();
        }
        if (!configured.matches("[A-Za-z0-9][A-Za-z0-9_.-]{0,62}")) {
            throw new IllegalArgumentException(
                    "LOGWRITER_RUN_ID must match [A-Za-z0-9][A-Za-z0-9_.-]{0,62}");
        }
        return configured;
    }
}
