## Use compiler debugging flags per default
set(CMAKE_C_FLAGS "${CMAKE_C_FLAGS} ${CMAKE_C_FLAGS_DEBUG}")

add_executable(demo main.c)
include_directories(${CMAKE_SOURCE_DIR}/include)
if(NOT WIN32)
    target_link_libraries(demo LINK_PUBLIC datadog-agent-rtloader dl)
else()
    set_target_properties(demo PROPERTIES LINK_FLAGS -static)
    target_link_libraries(demo LINK_PUBLIC datadog-agent-rtloader)
endif()
