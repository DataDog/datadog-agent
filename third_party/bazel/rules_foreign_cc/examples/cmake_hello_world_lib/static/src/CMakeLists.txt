# taken from https://github.com/Akagi201/learning-cmake/tree/master/hello-world-lib
# for test only

cmake_minimum_required(VERSION 3.5.0)

set(LIBHELLO_SRC hello.c)

add_library(hello_static STATIC ${LIBHELLO_SRC})

if(CMAKE_HOST_SYSTEM_NAME STREQUAL "Windows")
    set_target_properties(hello_static PROPERTIES OUTPUT_NAME "libhello")
else()
    set_target_properties(hello_static PROPERTIES OUTPUT_NAME "hello")
endif()

set(LIBRARY_OUTPUT_PATH ${PROJECT_BINARY_DIR}/lib)

install(TARGETS hello_static ARCHIVE DESTINATION lib)

install(FILES hello.h DESTINATION include/version123)
