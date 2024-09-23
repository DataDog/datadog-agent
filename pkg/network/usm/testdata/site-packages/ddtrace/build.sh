#!/bin/sh

clang -shared -fpic fakessl.c -target aarch64-linux-gnu -nostdlib -fuse-ld=lld -Wl,-s -o libssl.so.arm64
clang -shared -fpic fakessl.c -target x86_64 -nostdlib -fuse-ld=lld -Wl,-s -o libssl.so.amd64
