cmake_minimum_required(VERSION 3.15)

project(hello)

# Set C++ standard
set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_CXX_EXTENSIONS OFF)

# Detect platform and architecture
if(APPLE)
    if(CMAKE_SYSTEM_PROCESSOR MATCHES "arm64|aarch64")
        set(ONNXRUNTIME_ARCH "arm64")
        set(ONNXRUNTIME_OS "osx")
    else()
        set(ONNXRUNTIME_ARCH "x64")
        set(ONNXRUNTIME_OS "osx")
    endif()
elseif(UNIX)
    if(CMAKE_SYSTEM_PROCESSOR MATCHES "arm64|aarch64")
        set(ONNXRUNTIME_ARCH "aarch64")
        set(ONNXRUNTIME_OS "linux")
    else()
        set(ONNXRUNTIME_ARCH "x64")
        set(ONNXRUNTIME_OS "linux")
    endif()
elseif(WIN32)
    if(CMAKE_SYSTEM_PROCESSOR MATCHES "x86")
        set(ONNXRUNTIME_ARCH "x86")
    else()
        set(ONNXRUNTIME_ARCH "x64")
    endif()
    set(ONNXRUNTIME_OS "win")
else()
    message(FATAL_ERROR "Unsupported platform")
endif()

# ONNX Runtime version
set(ONNXRUNTIME_VERSION "1.23.2")

# Determine file extension
if(WIN32)
    set(ONNXRUNTIME_EXT "zip")
else()
    set(ONNXRUNTIME_EXT "tgz")
endif()

# Construct download URL
set(ONNXRUNTIME_URL "https://github.com/microsoft/onnxruntime/releases/download/v${ONNXRUNTIME_VERSION}/onnxruntime-${ONNXRUNTIME_OS}-${ONNXRUNTIME_ARCH}-${ONNXRUNTIME_VERSION}.${ONNXRUNTIME_EXT}")

# Set build directory for downloads
set(ONNXRUNTIME_BUILD_DIR "${CMAKE_BINARY_DIR}/onnxruntime")
set(ONNXRUNTIME_SOURCE_DIR "${ONNXRUNTIME_BUILD_DIR}/onnxruntime-${ONNXRUNTIME_OS}-${ONNXRUNTIME_ARCH}-${ONNXRUNTIME_VERSION}")

# Download and extract ONNX Runtime using ExternalProject
include(ExternalProject)

# ExternalProject_Add will automatically extract archives when using URL
ExternalProject_Add(
    onnxruntime_download
    URL ${ONNXRUNTIME_URL}
    DOWNLOAD_DIR ${ONNXRUNTIME_BUILD_DIR}
    SOURCE_DIR ${ONNXRUNTIME_SOURCE_DIR}
    CONFIGURE_COMMAND ""
    BUILD_COMMAND ""
    INSTALL_COMMAND ""
    UPDATE_COMMAND ""
    PATCH_COMMAND ""
    DOWNLOAD_NO_EXTRACT FALSE
)

# Set paths for ONNX Runtime (after extraction)
set(ONNXRUNTIME_INCLUDE_DIR "${ONNXRUNTIME_SOURCE_DIR}/include")
if(WIN32)
    set(ONNXRUNTIME_LIB_DIR "${ONNXRUNTIME_SOURCE_DIR}/lib")
    set(ONNXRUNTIME_BIN_DIR "${ONNXRUNTIME_SOURCE_DIR}/lib")
else()
    set(ONNXRUNTIME_LIB_DIR "${ONNXRUNTIME_SOURCE_DIR}/lib")
endif()

# Option to control whether to install ONNX Runtime files
option(HELLO_INSTALL_ONNXRUNTIME "Install ONNX Runtime files to CMAKE_INSTALL_PREFIX" OFF)

# Only copy files to install directory if installation is enabled
if(HELLO_INSTALL_ONNXRUNTIME)
    # Create a custom target to copy files to install directory
    # This ensures the files are available for CGO linking
    set(ONNXRUNTIME_INSTALL_INCLUDE_DIR "${CMAKE_INSTALL_PREFIX}/include")
    set(ONNXRUNTIME_INSTALL_LIB_DIR "${CMAKE_INSTALL_PREFIX}/lib")
    if(WIN32)
        set(ONNXRUNTIME_INSTALL_BIN_DIR "${CMAKE_INSTALL_PREFIX}/bin")
    endif()

    # Copy headers
    add_custom_command(
        TARGET onnxruntime_download POST_BUILD
        COMMAND ${CMAKE_COMMAND} -E make_directory ${ONNXRUNTIME_INSTALL_INCLUDE_DIR}
        COMMAND ${CMAKE_COMMAND} -E copy_directory ${ONNXRUNTIME_INCLUDE_DIR} ${ONNXRUNTIME_INSTALL_INCLUDE_DIR}
        COMMENT "Copying ONNX Runtime headers to ${ONNXRUNTIME_INSTALL_INCLUDE_DIR}"
    )

    # Copy libraries
    if(WIN32)
        add_custom_command(
            TARGET onnxruntime_download POST_BUILD
            COMMAND ${CMAKE_COMMAND} -E make_directory ${ONNXRUNTIME_INSTALL_BIN_DIR}
            COMMAND ${CMAKE_COMMAND} -E make_directory ${ONNXRUNTIME_INSTALL_LIB_DIR}
            COMMAND ${CMAKE_COMMAND} -E copy ${ONNXRUNTIME_LIB_DIR}/onnxruntime.dll ${ONNXRUNTIME_INSTALL_BIN_DIR}/
            COMMAND ${CMAKE_COMMAND} -E copy ${ONNXRUNTIME_LIB_DIR}/onnxruntime.lib ${ONNXRUNTIME_INSTALL_LIB_DIR}/
            COMMENT "Copying ONNX Runtime libraries to ${ONNXRUNTIME_INSTALL_BIN_DIR} and ${ONNXRUNTIME_INSTALL_LIB_DIR}"
        )
    elseif(APPLE)
        add_custom_command(
            TARGET onnxruntime_download POST_BUILD
            COMMAND ${CMAKE_COMMAND} -E make_directory ${ONNXRUNTIME_INSTALL_LIB_DIR}
            COMMAND ${CMAKE_COMMAND} -E copy ${ONNXRUNTIME_LIB_DIR}/libonnxruntime.dylib* ${ONNXRUNTIME_INSTALL_LIB_DIR}/
            COMMAND ${CMAKE_COMMAND} -E copy ${ONNXRUNTIME_LIB_DIR}/libonnxruntime.*.dylib ${ONNXRUNTIME_INSTALL_LIB_DIR}/
            COMMENT "Copying ONNX Runtime libraries to ${ONNXRUNTIME_INSTALL_LIB_DIR}"
        )
    else()
        add_custom_command(
            TARGET onnxruntime_download POST_BUILD
            COMMAND ${CMAKE_COMMAND} -E make_directory ${ONNXRUNTIME_INSTALL_LIB_DIR}
            COMMAND ${CMAKE_COMMAND} -E copy ${ONNXRUNTIME_LIB_DIR}/libonnxruntime.so* ${ONNXRUNTIME_INSTALL_LIB_DIR}/
            COMMENT "Copying ONNX Runtime libraries to ${ONNXRUNTIME_INSTALL_LIB_DIR}"
        )
    endif()
else()
    message(STATUS "No install prefix set - ONNX Runtime will remain in build directory: ${ONNXRUNTIME_SOURCE_DIR}")
endif()

# Note: hello.cpp is compiled directly by CGO via hello.go
# This CMake file only ensures ONNX Runtime is downloaded and installed
# The hello library target is optional - it can be used for testing
# but CGO will compile hello.cpp directly when building the Go package

# Optional: Build hello as a static library for testing purposes
# Uncomment if you want to build a standalone library
# add_library(hello STATIC
#     hello.cpp
# )
#
# target_include_directories(hello PUBLIC
#     ${CMAKE_CURRENT_SOURCE_DIR}
#     ${ONNXRUNTIME_INCLUDE_DIR}
# )
#
# target_link_directories(hello PRIVATE
#     ${ONNXRUNTIME_LIB_DIR}
# )
#
# if(WIN32)
#     target_link_libraries(hello PRIVATE onnxruntime.lib)
# else()
#     target_link_libraries(hello PRIVATE onnxruntime)
# endif()
#
# add_dependencies(hello onnxruntime_download)

# Install hello.h header (used by CGO) only if installation is enabled
if(HELLO_INSTALL_ONNXRUNTIME)
    install(FILES hello.h
        DESTINATION include
    )
endif()
