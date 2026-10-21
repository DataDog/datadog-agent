// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

import java.util.ArrayList;
import java.util.List;

// GcLoop allocates continuously, so that young collections happen on their
// own, and requests a full collection every second. Run it with source launch:
//
//   java -XX:+UseSerialGC -Xmx64m GcLoop.java
public class GcLoop {
    public static void main(String[] args) throws InterruptedException {
        System.out.println("GcLoop started");
        System.out.flush();

        List<byte[]> retained = new ArrayList<>();
        long iteration = 0;
        while (true) {
            // about 1 MiB of garbage per iteration, a few objects survive
            for (int i = 0; i < 1024; i++) {
                byte[] block = new byte[1024];
                block[0] = (byte) i;
                if (i % 128 == 0) {
                    retained.add(block);
                }
            }
            if (retained.size() > 4096) {
                retained.clear();
            }

            iteration++;
            if (iteration % 100 == 0) {
                System.gc();
            }
            Thread.sleep(10);
        }
    }
}
