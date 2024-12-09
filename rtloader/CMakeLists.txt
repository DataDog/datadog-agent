cmake_minimum_required(VERSION 3.12)

## Project name
project(RtLoader)

## Require C++11 with no extensions
set(CMAKE_CXX_STANDARD 11)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_CXX_EXTENSIONS OFF)

# Favor finding Python by location rather than by version
if (POLICY CMP0094)
    cmake_policy(SET CMP0094 NEW)
endif()

if(WIN32)
    set(CMAKE_RUNTIME_OUTPUT_DIRECTORY ${CMAKE_SOURCE_DIR}/bin )
    SET(CMAKE_CXX_FLAGS  "${CMAKE_CXX_FLAGS} -mwin32")
endif()

if(ARCH_I386)
    set(CMAKE_CXX_FLAGS "${CMAKE_CXX_FLAGS} -m32")
    set(CMAKE_C_FLAGS "${CMAKE_C_FLAGS} -m32")
endif()

## Config options
option(BUILD_DEMO "Build the demo app" ON)

## Add Build Targets
add_subdirectory(three)
add_subdirectory(rtloader)
add_subdirectory(test)
if (BUILD_DEMO)
    add_subdirectory(demo)
endif()

## Dev tools
include(cmake/clang-format.cmake)
