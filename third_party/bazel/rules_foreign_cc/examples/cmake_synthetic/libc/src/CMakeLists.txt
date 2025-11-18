cmake_minimum_required(VERSION 2.8.4)

set(libc_SRC libc.cpp libc.h)

add_library(libc_static STATIC ${libc_SRC})
set_target_properties(libc_static PROPERTIES OUTPUT_NAME "libc")
IF (WIN32)
  set_target_properties(libc_static PROPERTIES ARCHIVE_OUTPUT_NAME "libc")
ELSE()
  set_target_properties(libc_static PROPERTIES ARCHIVE_OUTPUT_NAME "c")
ENDIF()

#find_package(LIBA NAMES alib liba libliba liba++)
#find_package(LIBB NAMES blib libb liblibb)

# LIBA_INCLUDE_DIRS is not set, so giving the path relative to liba_config.cmake
# would be happy to improve that
target_link_libraries(libc_static PUBLIC ${LIBA_DIR}/lib/liblib_a_bazel.a)
target_include_directories(libc_static PUBLIC ${LIBA_DIR}/include)

target_link_libraries(libc_static PUBLIC ${LIBB_DIR}/lib/liblib_b_bazel.a)
target_include_directories(libc_static PUBLIC ${LIBB_DIR}/include)

set(LIBRARY_OUTPUT_PATH ${PROJECT_BINARY_DIR}/lib)

install(TARGETS libc_static ARCHIVE DESTINATION lib)

install(FILES libc.h DESTINATION include)
