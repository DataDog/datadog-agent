`simple-1.0.1-1.i386.rpm` is the unsigned test fixture from
[sassoftware/go-rpmutils v0.4.0](https://github.com/sassoftware/go-rpmutils/tree/v0.4.0/testdata),
licensed under Apache-2.0. Tests sign it with ephemeral keys generated at runtime;
no trusted production keys or private keys are stored here.

`sha256.rpm` comes from the same upstream release's `testdata/nfpm/test-1.0.0.x86_64.rpm`.
It has a SHA-256 payload digest in its general header.
