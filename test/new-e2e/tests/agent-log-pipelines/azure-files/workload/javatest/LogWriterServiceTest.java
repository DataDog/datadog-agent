package com.datadoghq.e2e.logwriter;

import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertTrue;
import static org.junit.jupiter.api.Assertions.assertArrayEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;

import java.time.Instant;
import org.junit.jupiter.api.Test;

class LogWriterServiceTest {

    @Test
    void defersFirstPeriodWhenFiveSecondHeadPauseLeavesTooLittleFillTime() {
        assertTrue(
                LogWriterService.shouldDeferInitialPeriod(
                        Instant.parse("2026-08-13T12:00:56Z"), 5_000, 10_000));
    }

    @Test
    void startsFirstPeriodWhenThereIsEnoughRunway() {
        assertFalse(
                LogWriterService.shouldDeferInitialPeriod(
                        Instant.parse("2026-08-13T12:00:10Z"), 5_000, 10_000));
    }

    @Test
    void parsesBurstTargetByteSequence() {
        assertArrayEquals(
                new long[] {511, 2048, 4096, 1048576},
                LogWriterService.parseTargetBytesSequence("511,2048,4096,1048576", 83000));
        assertArrayEquals(new long[] {83000}, LogWriterService.parseTargetBytesSequence("", 83000));
        assertThrows(
                IllegalArgumentException.class,
                () -> LogWriterService.parseTargetBytesSequence("510", 83000));
    }
}
