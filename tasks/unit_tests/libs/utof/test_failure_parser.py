"""Tests for tasks.libs.testing.utof.go_parser.failure_parser.

Covers the review feedback from PR #47621:
- Panic stacktrace frame regex must match tab-indented file:line after strip() fix
- Testify assertion block parsing (single and multiple assertions)
- Testify continuation lines for multiline expected/actual values
- EventuallyWithT wrapper filtering
- Standard Go test error fallback
- Custom failure extractors
"""

from __future__ import annotations

from datetime import datetime

from tasks.libs.testing.result_json import ActionType, ResultJsonLine
from tasks.libs.testing.utof.go_parser.failure_parser import (
    _drop_eventually_wrappers,
    _extract_failure_info,
    _extract_message_from_raw_output,
    _extract_stacktrace_from_raw_output,
    _parse_assertion_blocks,
    _short_location,
)


def _line(action: ActionType, output: str | None = None, pkg: str = "example.com/pkg") -> ResultJsonLine:
    """Helper to create a ResultJsonLine."""
    return ResultJsonLine(
        time=datetime(2026, 1, 1),
        action=action,
        package=pkg,
        test="TestExample",
        output=output,
    )


# ---------------------------------------------------------------------------
# _parse_assertion_blocks
# ---------------------------------------------------------------------------


class TestParseAssertionBlocks:
    def test_single_assertion(self):
        lines = [
            "\tError Trace:\t/home/user/project/foo_test.go:42",
            "\tError:      \tExpected nil, but got: &errors.errorString{s:\"oops\"}",
        ]
        blocks = _parse_assertion_blocks(lines)
        assert len(blocks) == 1
        assert blocks[0].trace == "/home/user/project/foo_test.go:42"
        assert blocks[0].error_lines == ['Expected nil, but got: &errors.errorString{s:"oops"}']

    def test_multiple_assertions(self):
        lines = [
            "\tError Trace:\t/path/a_test.go:10",
            "\tError:      \tNot equal",
            "\tError Trace:\t/path/b_test.go:20",
            "\tError:      \tShould be true",
        ]
        blocks = _parse_assertion_blocks(lines)
        assert len(blocks) == 2
        assert blocks[0].trace == "/path/a_test.go:10"
        assert blocks[0].error_lines == ["Not equal"]
        assert blocks[1].trace == "/path/b_test.go:20"
        assert blocks[1].error_lines == ["Should be true"]

    def test_continuation_lines_multiline_expected_actual(self):
        """Review comment: Does the continuation regex work for multiline expected/actual?"""
        lines = [
            "\tError Trace:\t/path/foo_test.go:42",
            "\tError:      \tNot equal:",
            "\t            \texpected: 5",
            "\t            \tactual  : 3",
        ]
        blocks = _parse_assertion_blocks(lines)
        assert len(blocks) == 1
        assert blocks[0].error_lines == ["Not equal:", "expected: 5", "actual  : 3"]

    def test_messages_line(self):
        lines = [
            "\tError Trace:\t/path/foo_test.go:42",
            "\tError:      \tExpected nil",
            "\tMessages:   \t'diskspd.exe' process not found in payloads",
        ]
        blocks = _parse_assertion_blocks(lines)
        assert len(blocks) == 1
        assert "'diskspd.exe' process not found in payloads" in blocks[0].error_lines

    def test_messages_truncated_when_long(self):
        long_msg = "x" * 200
        lines = [
            "\tError Trace:\t/path/foo_test.go:42",
            "\tError:      \tExpected nil",
            f"\tMessages:   \t{long_msg}",
        ]
        blocks = _parse_assertion_blocks(lines)
        assert len(blocks) == 1
        msg_line = blocks[0].error_lines[-1]
        assert len(msg_line) <= 121  # 120 + "…"
        assert msg_line.endswith("…")

    def test_no_assertion_blocks(self):
        lines = [
            "    some_test.go:42: expected X, got Y",
            "random output",
        ]
        blocks = _parse_assertion_blocks(lines)
        assert blocks == []

    def test_continuation_stops_on_non_matching_line(self):
        """Continuation capture stops when line doesn't match the indentation pattern."""
        lines = [
            "\tError Trace:\t/path/foo_test.go:42",
            "\tError:      \tNot equal:",
            "\t            \texpected: 5",
            "\tTest:       \tTestFoo",  # Not a continuation
        ]
        blocks = _parse_assertion_blocks(lines)
        assert len(blocks) == 1
        assert blocks[0].error_lines == ["Not equal:", "expected: 5"]


# ---------------------------------------------------------------------------
# _short_location
# ---------------------------------------------------------------------------


class TestShortLocation:
    def test_absolute_path(self):
        assert _short_location("/home/user/project/foo_test.go:42") == "foo_test.go:42"

    def test_relative_path(self):
        assert _short_location("foo_test.go:42") == "foo_test.go:42"

    def test_empty(self):
        assert _short_location("") == ""


# ---------------------------------------------------------------------------
# _drop_eventually_wrappers
# ---------------------------------------------------------------------------


class TestDropEventuallyWrappers:
    def test_removes_wrapper_when_real_blocks_exist(self):
        from tasks.libs.testing.utof.go_parser.failure_parser import _AssertionBlock

        wrapper = _AssertionBlock(trace="wrapper.go:1", error_lines=["Condition never satisfied"])
        real = _AssertionBlock(trace="foo_test.go:42", error_lines=["Not equal"])
        result = _drop_eventually_wrappers([wrapper, real])
        assert len(result) == 1
        assert result[0] is real

    def test_keeps_wrapper_when_only_block(self):
        from tasks.libs.testing.utof.go_parser.failure_parser import _AssertionBlock

        wrapper = _AssertionBlock(trace="wrapper.go:1", error_lines=["Condition never satisfied"])
        result = _drop_eventually_wrappers([wrapper])
        assert len(result) == 1
        assert result[0] is wrapper


# ---------------------------------------------------------------------------
# _extract_message_from_raw_output
# ---------------------------------------------------------------------------


class TestExtractMessageFromRawOutput:
    def test_single_testify_assertion(self):
        lines = [
            "\tError Trace:\t/path/foo_test.go:42",
            "\tError:      \tExpected nil, but got: error",
        ]
        msg = _extract_message_from_raw_output(lines)
        assert "foo_test.go:42" in msg
        assert "Expected nil, but got: error" in msg

    def test_multiple_testify_assertions(self):
        lines = [
            "\tError Trace:\t/path/a_test.go:10",
            "\tError:      \tNot equal",
            "\tError Trace:\t/path/b_test.go:20",
            "\tError:      \tShould be true",
        ]
        msg = _extract_message_from_raw_output(lines)
        assert "2 assertions failed" in msg
        assert "[1]" in msg
        assert "[2]" in msg

    def test_go_test_error_fallback(self):
        lines = [
            "    some_test.go:42: expected 5, got 3",
        ]
        msg = _extract_message_from_raw_output(lines)
        assert "some_test.go:42" in msg
        assert "expected 5, got 3" in msg

    def test_empty_output(self):
        assert _extract_message_from_raw_output([]) == ""

    def test_no_matching_patterns(self):
        assert _extract_message_from_raw_output(["random noise", "more noise"]) == ""


# ---------------------------------------------------------------------------
# _extract_stacktrace_from_raw_output
# ---------------------------------------------------------------------------


class TestExtractStacktraceFromRawOutput:
    def test_testify_traces(self):
        lines = [
            "\tError Trace:\t/path/a_test.go:10",
            "\tError:      \tNot equal",
            "\tError Trace:\t/path/b_test.go:20",
            "\tError:      \tShould be true",
        ]
        st = _extract_stacktrace_from_raw_output(lines)
        assert "/path/a_test.go:10" in st
        assert "/path/b_test.go:20" in st

    def test_go_test_fallback(self):
        lines = ["    some_test.go:42: expected 5, got 3"]
        st = _extract_stacktrace_from_raw_output(lines)
        assert "some_test.go:42" in st

    def test_empty(self):
        assert _extract_stacktrace_from_raw_output([]) == ""


# ---------------------------------------------------------------------------
# _extract_failure_info — the main integration function
# ---------------------------------------------------------------------------


class TestExtractFailureInfo:
    def test_no_failure(self):
        lines = [
            _line(ActionType.OUTPUT, "=== RUN   TestExample\n"),
            _line(ActionType.PASS),
        ]
        assert _extract_failure_info(lines) is None

    def test_assertion_failure(self):
        lines = [
            _line(ActionType.OUTPUT, "--- FAIL: TestExample (0.01s)\n"),
            _line(ActionType.OUTPUT, "    some_test.go:42: expected 5, got 3\n"),
            _line(ActionType.FAIL),
        ]
        f = _extract_failure_info(lines)
        assert f is not None
        assert f.type == "assertion"
        assert "expected 5, got 3" in f.message

    def test_build_failure(self):
        lines = [
            _line(ActionType.BUILD_FAIL, "compilation error\n"),
        ]
        f = _extract_failure_info(lines)
        assert f is not None
        assert f.type == "build"

    def test_panic_basic(self):
        lines = [
            _line(ActionType.OUTPUT, "panic: runtime error: index out of range\n"),
            _line(ActionType.OUTPUT, "goroutine 1 [running]:\n"),
            _line(ActionType.OUTPUT, "main.foo()\n"),
            _line(ActionType.OUTPUT, "\t/home/user/project/main.go:42 +0x1a\n"),
            _line(ActionType.FAIL),
        ]
        f = _extract_failure_info(lines)
        assert f is not None
        assert f.type == "panic"
        assert "runtime error: index out of range" in f.message

    def test_panic_stacktrace_captures_tab_indented_file_lines(self):
        """Regression test for PR #47621 review: tab-indented file:line frames
        must be captured in the panic stacktrace.

        Before the fix, `stripped = output.strip()` removed leading tabs, then
        `re.match(r'\\t.*:\\d+', stripped)` could never match since the tab was gone.
        The fix matches against the un-stripped output.
        """
        lines = [
            _line(ActionType.OUTPUT, "panic: nil pointer dereference\n"),
            _line(ActionType.OUTPUT, "goroutine 1 [running]:\n"),
            _line(ActionType.OUTPUT, "main.doStuff(0x0)\n"),
            _line(ActionType.OUTPUT, "\t/home/user/project/main.go:42 +0x1a\n"),
            _line(ActionType.OUTPUT, "main.main()\n"),
            _line(ActionType.OUTPUT, "\t/home/user/project/main.go:10 +0x2b\n"),
            _line(ActionType.FAIL),
        ]
        f = _extract_failure_info(lines)
        assert f is not None
        assert f.type == "panic"
        # The tab-indented file:line frames must be in the stacktrace
        assert "/home/user/project/main.go:42" in f.stacktrace
        assert "/home/user/project/main.go:10" in f.stacktrace

    def test_panic_stacktrace_includes_goroutine_header(self):
        lines = [
            _line(ActionType.OUTPUT, "panic: something bad\n"),
            _line(ActionType.OUTPUT, "goroutine 1 [running]:\n"),
            _line(ActionType.OUTPUT, "pkg.Func()\n"),
            _line(ActionType.OUTPUT, "\t/path/file.go:99 +0xff\n"),
            _line(ActionType.FAIL),
        ]
        f = _extract_failure_info(lines)
        assert f is not None
        assert "goroutine 1 [running]:" in f.stacktrace

    def test_panic_stacktrace_includes_created_by(self):
        lines = [
            _line(ActionType.OUTPUT, "panic: oh no\n"),
            _line(ActionType.OUTPUT, "goroutine 42 [running]:\n"),
            _line(ActionType.OUTPUT, "pkg.handler()\n"),
            _line(ActionType.OUTPUT, "\t/path/handler.go:10 +0x1a\n"),
            _line(ActionType.OUTPUT, "created by pkg.Start in goroutine 1\n"),
            _line(ActionType.FAIL),
        ]
        f = _extract_failure_info(lines)
        assert f is not None
        assert "created by pkg.Start" in f.stacktrace

    def test_panic_stacktrace_stops_on_non_trace_line(self):
        """After the stacktrace ends, normal output lines should not be included."""
        lines = [
            _line(ActionType.OUTPUT, "panic: bad\n"),
            _line(ActionType.OUTPUT, "goroutine 1 [running]:\n"),
            _line(ActionType.OUTPUT, "pkg.Func()\n"),
            _line(ActionType.OUTPUT, "\t/path/file.go:99 +0xff\n"),
            _line(ActionType.OUTPUT, "FAIL\texample.com/pkg\t0.001s\n"),
            _line(ActionType.FAIL),
        ]
        f = _extract_failure_info(lines)
        assert f is not None
        # The FAIL line should NOT be in the stacktrace
        assert "FAIL\texample.com/pkg" not in f.stacktrace
        # But the real frames should be
        assert "/path/file.go:99" in f.stacktrace

    def test_testify_assertion_via_extract_failure_info(self):
        lines = [
            _line(ActionType.OUTPUT, "\tError Trace:\t/path/foo_test.go:42\n"),
            _line(ActionType.OUTPUT, "\tError:      \tExpected nil, but got: error\n"),
            _line(ActionType.OUTPUT, "--- FAIL: TestExample (0.01s)\n"),
            _line(ActionType.FAIL),
        ]
        f = _extract_failure_info(lines)
        assert f is not None
        assert f.type == "assertion"
        assert "Expected nil, but got: error" in f.message
        assert f.direct is True

    def test_custom_extractor(self):
        def my_extractor(raw_lines):
            for line in raw_lines:
                if "CUSTOM_ERROR" in line:
                    return ("custom", "custom error detected")
            return None

        lines = [
            _line(ActionType.OUTPUT, "CUSTOM_ERROR: something went wrong\n"),
            _line(ActionType.OUTPUT, "--- FAIL: TestExample (0.01s)\n"),
            _line(ActionType.FAIL),
        ]
        f = _extract_failure_info(lines, custom_extractors=[my_extractor])
        assert f is not None
        assert f.type == "custom"
        assert f.message == "custom error detected"
        assert f.direct is True

    def test_custom_extractor_skipped_when_no_match(self):
        def noop_extractor(raw_lines):
            return None

        lines = [
            _line(ActionType.OUTPUT, "    some_test.go:42: expected 5, got 3\n"),
            _line(ActionType.OUTPUT, "--- FAIL: TestExample (0.01s)\n"),
            _line(ActionType.FAIL),
        ]
        f = _extract_failure_info(lines, custom_extractors=[noop_extractor])
        assert f is not None
        assert f.type == "assertion"
        assert "expected 5, got 3" in f.message

    def test_fail_without_output_still_detected(self):
        """A FAIL action with no output lines should still produce a failure."""
        lines = [
            _line(ActionType.FAIL),
        ]
        f = _extract_failure_info(lines)
        assert f is not None
        assert f.type == "assertion"
        assert f.message == ""

    def test_multiline_testify_values(self):
        """Review comment: verify multiline expected/actual values are captured
        through the full _extract_failure_info pipeline."""
        lines = [
            _line(ActionType.OUTPUT, "\tError Trace:\t/path/foo_test.go:42\n"),
            _line(ActionType.OUTPUT, "\tError:      \tNot equal:\n"),
            _line(ActionType.OUTPUT, "\t            \texpected: map[string]int{\"a\":1, \"b\":2}\n"),
            _line(ActionType.OUTPUT, "\t            \tactual  : map[string]int{\"a\":1}\n"),
            _line(ActionType.OUTPUT, "--- FAIL: TestExample (0.01s)\n"),
            _line(ActionType.FAIL),
        ]
        f = _extract_failure_info(lines)
        assert f is not None
        assert 'expected: map[string]int{"a":1, "b":2}' in f.message
        assert 'actual  : map[string]int{"a":1}' in f.message

    def test_eventually_wrapper_filtered_in_full_pipeline(self):
        """EventuallyWithT wrapper blocks should be stripped when real assertions exist."""
        lines = [
            _line(ActionType.OUTPUT, "\tError Trace:\t/path/wrapper.go:10\n"),
            _line(ActionType.OUTPUT, "\tError:      \tCondition never satisfied\n"),
            _line(ActionType.OUTPUT, "\tError Trace:\t/path/real_test.go:42\n"),
            _line(ActionType.OUTPUT, "\tError:      \tExpected true, got false\n"),
            _line(ActionType.OUTPUT, "--- FAIL: TestExample (0.01s)\n"),
            _line(ActionType.FAIL),
        ]
        f = _extract_failure_info(lines)
        assert f is not None
        assert "Condition never satisfied" not in f.message
        assert "Expected true, got false" in f.message
