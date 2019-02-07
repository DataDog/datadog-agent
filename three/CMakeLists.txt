cmake_minimum_required(VERSION 3.9)
find_package (Python3 COMPONENTS Interpreter Development)

project(datadog-agent-three VERSION 0.1.0 DESCRIPTION "CPython backend for the Datadog Agent")
include(GNUInstallDirs)
configure_file(constants.h.in constants.h)
include_directories(${CMAKE_CURRENT_BINARY_DIR})
add_library(datadog-agent-three SHARED
    three.cpp
)
target_include_directories(datadog-agent-three PRIVATE .)
target_include_directories(datadog-agent-three PUBLIC
    ${CMAKE_SOURCE_DIR}/include
    ${Python3_INCLUDE_DIRS}
)
target_link_libraries(datadog-agent-three ${Python3_LIBRARIES} datadog-agent-six)
install(TARGETS datadog-agent-three
    LIBRARY DESTINATION ${CMAKE_INSTALL_LIBDIR}
)
