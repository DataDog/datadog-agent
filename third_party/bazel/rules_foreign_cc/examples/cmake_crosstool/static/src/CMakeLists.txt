# taken from https://github.com/Akagi201/learning-cmake/tree/master/hello-world-lib
# for test only

cmake_minimum_required(VERSION 2.8.4)

set(LIBHELLO_SRC hello.c)

add_library(hello_static STATIC ${LIBHELLO_SRC})

set_target_properties(hello_static PROPERTIES OUTPUT_NAME "hello")

set(LIBRARY_OUTPUT_PATH ${PROJECT_BINARY_DIR}/lib)

install(TARGETS hello_static ARCHIVE DESTINATION lib)

install(FILES hello.h DESTINATION include/version123)
