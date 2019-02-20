cmake_minimum_required(VERSION 3.9)

project(datadog-agent-six VERSION 0.1.0 DESCRIPTION "CPython backend for the Datadog Agent")
include(GNUInstallDirs)

if(WIN32)
foreach(flag_var
        CMAKE_CXX_FLAGS CMAKE_CXX_FLAGS_DEBUG CMAKE_CXX_FLAGS_RELEASE
        CMAKE_CXX_FLAGS_MINSIZEREL CMAKE_CXX_FLAGS_RELWITHDEBINFO)
   if(${flag_var} MATCHES "/MD")
      string(REGEX REPLACE "/MD" "/MT" ${flag_var} "${${flag_var}}")
   endif(${flag_var} MATCHES "/MD")
endforeach(flag_var)
endif()

if(WIN32)
add_library(datadog-agent-six
    api.cpp
    six.cpp
)
else()
add_library(datadog-agent-six SHARED
    api.cpp
    six.cpp
)
endif()

set_target_properties(datadog-agent-six PROPERTIES
    VERSION ${PROJECT_VERSION}
    SOVERSION 1
    PUBLIC_HEADER include/datadog_agent_six.h
)
configure_file(datadog-agent-six.pc.in datadog-agent-six.pc @ONLY)
target_include_directories(datadog-agent-six PRIVATE ./include)
target_include_directories(datadog-agent-six PUBLIC
    ${CMAKE_SOURCE_DIR}/include
)
find_library( LIBdl dl )
#install(TARGETS datadog-agent-six
#    LIBRARY DESTINATION ${CMAKE_INSTALL_LIBDIR}
#    PUBLIC_HEADER DESTINATION ${CMAKE_INSTALL_INCLUDEDIR}
#)
