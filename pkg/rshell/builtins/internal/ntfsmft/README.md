# NTFS MFT integration tests

On Windows, raw-MFT integration tests lazily create one disposable, 2 GiB
expandable NTFS VHD on `R:`. They run against that clean volume, then detach
and delete it at package teardown. This requires an Administrator token and an
unused `R:` drive letter. If VHD provisioning is unavailable, only those raw
integration tests are skipped; parser and other unit tests still run.

To run against an already-mounted real NTFS drive instead, set
`RSHELL_NTFSDU_TEST_ROOT` to a directory on that drive. The tests create and
remove only their own temporary subdirectories; they do not attach, detach, or
delete the caller's drive.

```powershell
$env:RSHELL_NTFSDU_TEST_ROOT = 'R:\'
go test -count=1 -v ./builtins/internal/ntfsmft
```

The default VHD is clean and small, so every full-MFT scan is fast and its
contents are controlled by the test. A real drive validates the same raw-device
path against existing host data, but its MFT may be much larger and its scan
time varies with the drive's file population.
