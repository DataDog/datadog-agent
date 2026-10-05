package com.datadoghq.e2e.logwriter;

import static org.junit.jupiter.api.Assertions.assertEquals;

import java.nio.charset.StandardCharsets;
import org.junit.jupiter.api.Test;

class Crc64Test {

    @Test
    void matchesGoCrc64IsoCheckVector() {
        assertEquals(
                0xb90956c775a41001L,
                Crc64.checksum("123456789".getBytes(StandardCharsets.US_ASCII)));
    }
}
