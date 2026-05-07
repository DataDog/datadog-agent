I need to build a tool that reads a .tar file (which might be compressed) and emits a file containing the md5 checksum of each file within that tar file.
Let's first work on the requirements and then start implementation.
There are both functional requirements and environment requirements
Functional:
- must take the path to the input tar file as a command line arg
  - if no input tar is provided, then use stdin as the file
- must take the path to the output file as a command line arg
- sample of the output
md5 sum and the path
```
e3c6a486a70a471110731b1708d232cc  opt/datadog-installer/LICENSE
f9a6f2aa44430e18abbc7363751e3f7c  opt/datadog-installer/LICENSES/THIRD-PARTY-0BSD
3b83ef96387f14655fc854ddc3c6bd57  opt/datadog-installer/LICENSES/THIRD-PARTY-Apache-2.0
11d3feb7137319430849e84dbc75ac27  opt/datadog-installer/LICENSES/THIRD-PARTY-BSD-2-Clause
```
- paths should be relative, with no preceding "./"
- directories and symlinks in the tar file should be ignored.
- We must support different compression algorithms that could be provided
  - we do not have to decode the compression from the binary itself, we can use the file name as a hint
  - required for first implementation:  XZ compression, if the file ends in .xz,  gzip compression if the file ends in .gz or .tgz.

Non functional requirements
- The code should be in Rust.
- The code should live in the file tools/tar_checksums/generate_md5sum.rs
- This repository uses bazel as a build system. Copy the patterns used to build other rust code and use that
- Build and testing the code should be done with bazel commands instead of cargo.
- If we are already using a decompression library somewhere else, prefer that.
  - If not, the choice of decompress library is a sub-design problem.
