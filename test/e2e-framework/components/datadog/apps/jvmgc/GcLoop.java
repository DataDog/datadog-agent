// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// GcLoop keeps the garbage collector busy at a low CPU cost, so that the
// HotSpot GC and memory pool USDT probes fire regularly:
//   - about 10 MiB/s of short-lived arrays trigger a young collection every few
//     seconds;
//   - a slowly rotating set of retained arrays survives young collections, is
//     promoted, and makes the old generation grow until the collector runs
//     concurrent cycles and mixed collections;
//   - an explicit System.gc() every 30 seconds triggers a full collection.
//
// It is run with the source launcher (`java GcLoop.java`), so it must stay a
// single file whose first class holds main.
public class GcLoop {
    // Arrays of at most half a G1 region (1 MiB for heaps below 4 GiB) are
    // allocated in eden; bigger ones would be humongous and go to the old
    // generation directly.
    private static final int GARBAGE_SIZE = 64 * 1024;
    private static final int GARBAGE_PER_TICK = 16;
    private static final int RETAINED_SIZE = 256 * 1024;
    private static final int RETAINED_SLOTS = 128;
    private static final long TICK_MILLIS = 100;
    private static final int TICKS_PER_RETAINED_REPLACEMENT = 5;
    private static final int TICKS_PER_FULL_GC = 300;

    // Written so that the JIT cannot prove the allocations useless.
    private static volatile long sink;

    public static void main(String[] args) throws InterruptedException {
        byte[][] retained = new byte[RETAINED_SLOTS][];
        for (long tick = 1; ; tick++) {
            for (int i = 0; i < GARBAGE_PER_TICK; i++) {
                byte[] garbage = new byte[GARBAGE_SIZE];
                garbage[(int) (tick % GARBAGE_SIZE)] = (byte) i;
                sink += garbage[(int) ((tick + i) % GARBAGE_SIZE)];
            }
            if (tick % TICKS_PER_RETAINED_REPLACEMENT == 0) {
                int slot = (int) ((tick / TICKS_PER_RETAINED_REPLACEMENT) % RETAINED_SLOTS);
                retained[slot] = new byte[RETAINED_SIZE];
            }
            if (tick % TICKS_PER_FULL_GC == 0) {
                System.gc();
            }
            Thread.sleep(TICK_MILLIS);
        }
    }
}
