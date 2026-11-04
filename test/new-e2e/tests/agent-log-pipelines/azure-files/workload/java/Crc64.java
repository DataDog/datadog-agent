package com.datadoghq.e2e.logwriter;

import java.io.BufferedInputStream;
import java.io.ByteArrayOutputStream;
import java.io.FileInputStream;
import java.io.IOException;
import java.io.InputStream;
import java.nio.file.Path;
import java.nio.file.Paths;

/** Computes the same CRC64-ISO value as Go's hash/crc64.Checksum. */
public final class Crc64 {

    private static final long POLYNOMIAL = 0xD800000000000000L;
    private static final long[] TABLE = buildTable();

    private Crc64() {}

    public static void main(String[] args) throws IOException {
        if (args.length != 2 || !("bytes".equals(args[0]) || "line".equals(args[0]))) {
            throw new IllegalArgumentException("usage: Crc64 <bytes|line> <path>");
        }
        Path path = Paths.get(args[1]);
        byte[] content = "bytes".equals(args[0]) ? firstBytes(path, 2048) : firstLine(path);
        System.out.printf("0x%x%n", checksum(content));
    }

    static long checksum(byte[] content) {
        long crc = ~0L;
        for (byte value : content) {
            crc = TABLE[((int) crc ^ value) & 0xff] ^ (crc >>> 8);
        }
        return ~crc;
    }

    private static byte[] firstBytes(Path path, int count) throws IOException {
        byte[] output = new byte[count];
        int offset = 0;
        try (InputStream input = new BufferedInputStream(new FileInputStream(path.toFile()))) {
            while (offset < count) {
                int read = input.read(output, offset, count - offset);
                if (read < 0) {
                    break;
                }
                offset += read;
            }
        }
        if (offset == output.length) {
            return output;
        }
        byte[] shortOutput = new byte[offset];
        System.arraycopy(output, 0, shortOutput, 0, offset);
        return shortOutput;
    }

    private static byte[] firstLine(Path path) throws IOException {
        ByteArrayOutputStream output = new ByteArrayOutputStream();
        try (InputStream input = new BufferedInputStream(new FileInputStream(path.toFile()))) {
            int value;
            while ((value = input.read()) >= 0 && value != '\n') {
                output.write(value);
            }
        }
        byte[] line = output.toByteArray();
        if (line.length > 0 && line[line.length - 1] == '\r') {
            byte[] withoutCarriageReturn = new byte[line.length - 1];
            System.arraycopy(line, 0, withoutCarriageReturn, 0, withoutCarriageReturn.length);
            return withoutCarriageReturn;
        }
        return line;
    }

    private static long[] buildTable() {
        long[] table = new long[256];
        for (int index = 0; index < table.length; index++) {
            long crc = index;
            for (int bit = 0; bit < 8; bit++) {
                crc = (crc & 1L) != 0 ? (crc >>> 1) ^ POLYNOMIAL : crc >>> 1;
            }
            table[index] = crc;
        }
        return table;
    }
}
