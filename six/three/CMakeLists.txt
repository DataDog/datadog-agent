cmake_minimum_required(VERSION 3.9)
find_package (Python3 COMPONENTS Interpreter Development)

project(datadog-agent-three VERSION 0.1.0 DESCRIPTION "CPython backend for the Datadog Agent")

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

include(GNUInstallDirs)
configure_file(constants.h.in constants.h)
include_directories(${CMAKE_CURRENT_BINARY_DIR})
add_library(datadog-agent-three SHARED
    three.cpp
    ../common/sixstrings.c
    ../common/aggregator.c
    ../common/datadog_agent.c
)

add_compile_definitions(DATADOG_AGENT_THREE)
target_include_directories(datadog-agent-three PRIVATE .)
target_include_directories(datadog-agent-three PUBLIC
    ${CMAKE_SOURCE_DIR}/include
    ${CMAKE_SOURCE_DIR}/common
    ${Python3_INCLUDE_DIRS}
)
target_link_libraries(datadog-agent-three ${Python3_LIBRARIES} datadog-agent-six)
if(WIN32)
  install(TARGETS datadog-agent-three
      RUNTIME DESTINATION ${CMAKE_INSTALL_LIBDIR}
  )
else()
  install(TARGETS datadog-agent-three
      LIBRARY DESTINATION ${CMAKE_INSTALL_LIBDIR}
  )
endif()
