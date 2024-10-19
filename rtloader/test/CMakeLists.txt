cmake_minimum_required(VERSION 3.12)

set(PKGS
	"../../test/init/..."
	"../../test/rtloader/..."
	"../../test/aggregator/..."
	"../../test/datadog_agent/..."
	"../../test/util/..."
	"../../test/uutil/..."
	"../../test/common/..."
	"../../test/tagger/..."
	"../../test/kubeutil/..."
	"../../test/containers/..."
)

if (WIN32)
    set(LIBS_PATH \"${PROJECT_BINARY_DIR}/rtloader/\;${PROJECT_BINARY_DIR}/three/\")
else()
    set(LIBS_PATH "${PROJECT_BINARY_DIR}/rtloader/:${PROJECT_BINARY_DIR}/three/")
endif()

set (CGO_CFLAGS \"-I${CMAKE_SOURCE_DIR}/include -I${CMAKE_SOURCE_DIR}/common -Wno-deprecated-declarations\")

if (WIN32)
    set (CGO_LDFLAGS -L${PROJECT_BINARY_DIR}/rtloader -ldatadog-agent-rtloader -lstdc++ -static)
else()
    set (CGO_LDFLAGS -L${PROJECT_BINARY_DIR}/rtloader -ldatadog-agent-rtloader -ldl)
endif()

set (CGO_LDFLAGS \"-L${PROJECT_BINARY_DIR}/three/ ${CGO_LDFLAGS}\")
add_custom_command(
    OUTPUT testPy3
    COMMAND ${CMAKE_COMMAND} -E env CGO_CFLAGS=${CGO_CFLAGS} CGO_LDFLAGS=${CGO_LDFLAGS} DYLD_LIBRARY_PATH=${LIBS_PATH} LD_LIBRARY_PATH=${LIBS_PATH} go test -mod=mod -tags "three" -count=1 -p=1 ${PKGS}
)
list(APPEND TARGETS "testPy3")

add_custom_target(run DEPENDS ${TARGETS})
