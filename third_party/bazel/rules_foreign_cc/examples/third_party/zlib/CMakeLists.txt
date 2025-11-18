cmake_minimum_required(VERSION 3.15)

project(zlib-example)

find_package(ZLIB REQUIRED)

set(SRCS zlib-example.cpp)

add_executable(${PROJECT_NAME} ${SRCS})

target_link_libraries(${PROJECT_NAME} ZLIB::ZLIB)

install(TARGETS ${PROJECT_NAME} DESTINATION bin)
