cmake_minimum_required(VERSION 3.9)
find_package (Python2 COMPONENTS Interpreter Development)

project(datadog-agent-two VERSION 0.1.0 DESCRIPTION "CPython backend for the Datadog Agent")
include(GNUInstallDirs)
configure_file(constants.h.in constants.h)
include_directories(${CMAKE_CURRENT_BINARY_DIR})
add_library(datadog-agent-two SHARED
    two.cpp
)
target_include_directories(datadog-agent-two PRIVATE .)
target_include_directories(datadog-agent-two PUBLIC
    ${CMAKE_SOURCE_DIR}/include
    ${Python2_INCLUDE_DIRS}
)
target_link_libraries(datadog-agent-two ${Python2_LIBRARIES} datadog-agent-six)
target_compile_options(datadog-agent-two PRIVATE "-Wno-deprecated-register")
install(TARGETS datadog-agent-two
    LIBRARY DESTINATION ${CMAKE_INSTALL_LIBDIR}
)
