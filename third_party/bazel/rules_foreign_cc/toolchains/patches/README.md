# Foreign CC toolchain patches

## [cmake-c++11.patch](./cmake-c++11.patch)

See <https://discourse.cmake.org/t/cmake-error-at-cmakelists-txt-117-message-the-c-compiler-does-not-support-c-11-e-g-std-unique-ptr/3774/8>

## [make-reproducible-bootstrap.patch](./make-reproducible-bootstrap.patch)

This patch avoids reliance on host installed tools for bootstrapping make.

## [pkgconfig-builtin-glib-int-conversion.patch](./pkgconfig-builtin-glib-int-conversion.patch)

This patch fixes explicit integer conversion which causes errors in `clang >= 15` and `gcc >= 14`

## [pkgconfig-detectenv.patch](./pkgconfig-detectenv.patch)

This patch is required as bazel does not provide the VCINSTALLDIR or WINDOWSSDKDIR vars

## [pkgconfig-makefile-vc.patch](./pkgconfig-makefile-vc.patch)

This patch is required as rules_foreign_cc runs in MSYS2 on Windows and MSYS2's "mkdir" is used
