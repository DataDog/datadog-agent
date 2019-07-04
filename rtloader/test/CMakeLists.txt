cmake_minimum_required(VERSION 3.12)

set(PKGS
	"./init/..."
	"./rtloader/..."
	"./aggregator/..."
	"./datadog_agent/..."
	"./util/..."
	"./uutil/..."
	"./common/..."
	"./tagger/..."
    "./kubeutil/..."
    "./containers/..."
)

set(LIBS_PATH "${CMAKE_SOURCE_DIR}/rtloader/:${CMAKE_SOURCE_DIR}/two/:${CMAKE_SOURCE_DIR}/three/")

if (NOT DISABLE_PYTHON2)
    add_custom_command(
        OUTPUT testPy2
        COMMAND ${CMAKE_COMMAND} -E env DYLD_LIBRARY_PATH=${LIBS_PATH} LD_LIBRARY_PATH=${LIBS_PATH} go test -tags "two" -count=1 -p=1 ${PKGS}
    )
    list(APPEND TARGETS "testPy2")
endif()

if (NOT DISABLE_PYTHON3)
    add_custom_command(
        OUTPUT testPy3
        COMMAND ${CMAKE_COMMAND} -E env DYLD_LIBRARY_PATH=${LIBS_PATH} LD_LIBRARY_PATH=${LIBS_PATH} go test -tags "three" -count=1 -p=1 ${PKGS}
    )
    list(APPEND TARGETS "testPy3")
endif()

add_custom_target(run DEPENDS ${TARGETS})
