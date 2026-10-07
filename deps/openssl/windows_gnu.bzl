"""configure_make settings for OpenSSL builds targeting
//bazel/toolchains/hermetic_llvm:windows_x86_64_gnu."""

WINDOWS_GNU = "@llvm//platforms/config:windows_x86_64_gnu"

WINDOWS_GNU_BUILD_DATA = [
    "@@//bazel/toolchains/hermetic_llvm:libssp",
    "@@//bazel/toolchains/hermetic_llvm:llvm_windres.sh",
    "@llvm//tools:clang",
    "@llvm//tools:llvm-windres",
    "@mingw//:mingw-w64-headers/crt/_mingw.h",
    "@mingw//:mingw_w64_headers_crt_directory",
    "@mingw//:mingw_w64_headers_include_directory",
]

WINDOWS_GNU_CONFIGURE_OPTIONS = [
    "-L$(execpath @@//bazel/toolchains/hermetic_llvm:libssp)",
]

WINDOWS_GNU_ENV = {
    "LLVM_WINDRES": "$(execpath @llvm//tools:llvm-windres)",
    "MINGW_CRT_DIR": "$(execpath @mingw//:mingw_w64_headers_crt_directory)",
    "MINGW_GENERATED_CRT_H": "$(execpath @mingw//:mingw-w64-headers/crt/_mingw.h)",
    "MINGW_INCLUDE_DIR": "$(execpath @mingw//:mingw_w64_headers_include_directory)",
    "RC": "$(execpath @@//bazel/toolchains/hermetic_llvm:llvm_windres.sh)",
    "WINDRES": "$(execpath @@//bazel/toolchains/hermetic_llvm:llvm_windres.sh)",
}
