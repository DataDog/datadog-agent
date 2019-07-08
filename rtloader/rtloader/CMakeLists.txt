cmake_minimum_required(VERSION 3.9)

project(datadog-agent-rtloader VERSION 0.1.0 DESCRIPTION "CPython backend for the Datadog Agent")
include(GNUInstallDirs)

if(WIN32)
# explicitly set the compiler flags to use the static C runtime (/MT(d) instead of the DLL
# c runtime (/MD(d) so that we don't have to worry about redistributing the CRT).
foreach(flag_var
        CMAKE_CXX_FLAGS CMAKE_CXX_FLAGS_DEBUG CMAKE_CXX_FLAGS_RELEASE
        CMAKE_CXX_FLAGS_MINSIZEREL CMAKE_CXX_FLAGS_RELWITHDEBINFO)
   if(${flag_var} MATCHES "/MD")
      string(REGEX REPLACE "/MD" "/MT" ${flag_var} "${${flag_var}}")
   endif(${flag_var} MATCHES "/MD")
endforeach(flag_var)
endif()

if(WIN32)
add_library(datadog-agent-rtloader
    api.cpp
    rtloader.cpp
    ../common/memory.c
)
else()
add_library(datadog-agent-rtloader SHARED
    api.cpp
    rtloader.cpp
    ../common/memory.c
)
endif()

set_target_properties(datadog-agent-rtloader PROPERTIES
    VERSION ${PROJECT_VERSION}
    SOVERSION 1
    PUBLIC_HEADER "../include/datadog_agent_rtloader.h;../include/rtloader_types.h;../common/memory.h"
)
configure_file(datadog-agent-rtloader.pc.in datadog-agent-rtloader.pc @ONLY)
target_include_directories(datadog-agent-rtloader PRIVATE ./include)
target_include_directories(datadog-agent-rtloader PUBLIC
    ${CMAKE_SOURCE_DIR}/include
    ${CMAKE_SOURCE_DIR}/common
)

if(NOT WIN32)
find_library( LIBdl dl )
endif()

install(TARGETS datadog-agent-rtloader
    RUNTIME DESTINATION bin
    LIBRARY DESTINATION ${CMAKE_INSTALL_LIBDIR}
    PUBLIC_HEADER DESTINATION include
    ARCHIVE DESTINATION ${CMAKE_INSTALL_LIBDIR}
)
