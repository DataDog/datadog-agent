## Use compiler debugging flags per default
set(CMAKE_C_FLAGS "${CMAKE_C_FLAGS} ${CMAKE_C_FLAGS_DEBUG}")

add_executable(demo main.c)
target_link_libraries(demo LINK_PUBLIC datadog-agent-six dl)
