cmake_minimum_required(VERSION 3.18.1)

project(cmake_with_target_example)

set(LIB_SRC lib.cpp lib.h)

add_library(working_a STATIC ${LIB_SRC})
target_compile_definitions(working_a PRIVATE TRIGGER_ERROR=0)

add_library(working_b STATIC ${LIB_SRC})
target_compile_definitions(working_b PRIVATE TRIGGER_ERROR=0)

add_library(broken STATIC ${LIB_SRC})
target_compile_definitions(broken PRIVATE TRIGGER_ERROR=1)

install(TARGETS working_a working_b COMPONENT working ARCHIVE DESTINATION lib)
install(TARGETS broken COMPONENT broken ARCHIVE DESTINATION lib)
install(FILES lib.h DESTINATION include)
