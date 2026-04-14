#!/bin/bash
# test_subinterpreters.sh — Build and test RTLoader with sub-interpreter support
#
# Usage:
#   ./test_subinterpreters.sh          # build + run all tests
#   ./test_subinterpreters.sh build    # build only
#   ./test_subinterpreters.sh test     # test only (assumes already built)
#   ./test_subinterpreters.sh subinterp # sub-interpreter tests only (assumes already built)
#   ./test_subinterpreters.sh clean    # remove build dir and Go test cache

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
BUILD_DIR="$SCRIPT_DIR/build"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
NC='\033[0m' # No Color

info()  { echo -e "${GREEN}[INFO]${NC} $*"; }
warn()  { echo -e "${YELLOW}[WARN]${NC} $*"; }
error() { echo -e "${RED}[FAIL]${NC} $*"; }

do_build() {
    info "Cleaning build directory..."
    rm -rf "$BUILD_DIR"
    mkdir -p "$BUILD_DIR"

    info "Running cmake with -DENABLE_SUBINTERPRETERS=ON..."
    cd "$BUILD_DIR"
    cmake -DENABLE_SUBINTERPRETERS=ON .. 2>&1

    info "Building..."
    make -j"$(sysctl -n hw.ncpu 2>/dev/null || nproc 2>/dev/null || echo 4)" 2>&1

    info "Build complete."
}

do_standard_tests() {
    info "Running standard rtloader tests (no sub-interpreter flag)..."
    cd "$REPO_ROOT"
    # dda inv rtloader.test doesn't pass ENABLE_SUBINTERPRETERS,
    # so this tests the normal code path.
    if dda inv rtloader.test 2>&1; then
        info "Standard tests: PASS"
    else
        warn "Standard tests: some failures (check output above)"
        warn "TestGetIntegrationsList and TestCancelCheck are known to fail"
        warn "locally if datadog_checks is pip-installed."
    fi
}

do_subinterp_tests() {
    if [ ! -f "$BUILD_DIR/rtloader/libdatadog-agent-rtloader.dylib" ] && \
       [ ! -f "$BUILD_DIR/rtloader/libdatadog-agent-rtloader.so" ]; then
        error "Build not found. Run '$0 build' first."
        exit 1
    fi

    info "Running sub-interpreter tests..."
    cd "$REPO_ROOT"

    CGO_CFLAGS="-I${REPO_ROOT}/rtloader/include -I${REPO_ROOT}/rtloader/common" \
    CGO_LDFLAGS="-L${BUILD_DIR}/rtloader -L${BUILD_DIR}/three -ldatadog-agent-rtloader -ldatadog-agent-three -ldl" \
    DYLD_LIBRARY_PATH="${BUILD_DIR}/rtloader:${BUILD_DIR}/three" \
    LD_LIBRARY_PATH="${BUILD_DIR}/rtloader:${BUILD_DIR}/three" \
    go test -v -tags "three,python" ./rtloader/test/rtloader/ \
        -run "Subinterpreter|SubInterpreter" -count=1 2>&1

    info "Sub-interpreter tests: PASS"
}

do_clean() {
    info "Removing build directory..."
    rm -rf "$BUILD_DIR"

    info "Clearing Go test cache..."
    go clean -testcache 2>/dev/null || true

    # Also clean any cached .o / .a files from dda inv rtloader builds
    info "Removing dev/lib artifacts..."
    rm -f "$REPO_ROOT/dev/lib/libdatadog-agent-rtloader"* \
          "$REPO_ROOT/dev/lib/libdatadog-agent-three"* 2>/dev/null || true

    info "Clean complete."
}

case "${1:-all}" in
    build)
        do_build
        ;;
    test)
        do_standard_tests
        do_subinterp_tests
        ;;
    subinterp)
        do_subinterp_tests
        ;;
    clean)
        do_clean
        ;;
    all)
        do_build
        do_standard_tests
        do_subinterp_tests
        ;;
    *)
        echo "Usage: $0 [build|test|subinterp|clean|all]"
        exit 1
        ;;
esac
