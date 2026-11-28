import unittest

from tasks.libs.logs_tokenizer.compare import (
    BYPASS_LABEL,
    Row,
    benchmark_metrics,
    bypassed,
    find_regressions,
    markdown_report,
    parse_benchstat_csv,
)

# Output of `benchstat -format csv base=base.txt head=head.txt` (golang.org/x/perf, the version
# pinned in internal/tools) for three benchmarks: one 8.18% slower, one 44.82% faster, one unchanged.
BENCHSTAT_CSV = """goos: linux
goarch: amd64
pkg: github.com/DataDog/datadog-agent/pkg/logs/tokenizer
cpu: Intel(R) Xeon(R) Platinum 8259CL CPU @ 2.50GHz
,base,,head,,,
,sec/op,CI,sec/op,CI,vs base,P
Consumer/aad_log_signature/applog,1.4969500000000001e-06,1%,1.6194500000000002e-06,1%,+8.18%,p=0.000 n=10
Consumer/aad_log_signature/short,4.295e-08,1%,2.37e-08,1%,-44.82%,p=0.000 n=10
Consumer/logs_auto_multiline/short,1.8000000000000002e-08,1%,1.8000000000000002e-08,1%,~,p=0.871 n=10
geomean,1.0498993236805745e-07,,8.840214406530306e-08,,-15.80%,

,base,,head,,,
,B/op,CI,B/op,CI,vs base,P
Consumer/aad_log_signature/applog,512,0%,1024,0%,+100.00%,p=0.000 n=10
Consumer/aad_log_signature/short,16,0%,0,0%,-100.00%,p=0.000 n=10
Consumer/logs_auto_multiline/short,0,0%,0,0%,~,p=1.000 n=10
geomean,,,,,?,

,base,,head,,,
,allocs/op,CI,allocs/op,CI,vs base,P
Consumer/aad_log_signature/applog,7,0%,1,0%,-85.71%,p=0.000 n=10
Consumer/aad_log_signature/short,2,0%,0,0%,-100.00%,p=0.000 n=10
Consumer/logs_auto_multiline/short,0,0%,1,0%,?,p=0.000 n=10
geomean,,,,,?,
"""


class TestParseBenchstatCsv(unittest.TestCase):
    def test_rows_per_unit(self):
        rows = parse_benchstat_csv(BENCHSTAT_CSV)
        self.assertEqual([row.unit for row in rows], ["sec/op"] * 4 + ["B/op"] * 4 + ["allocs/op"] * 4)
        self.assertEqual(
            rows[0],
            Row(
                "sec/op", "Consumer/aad_log_signature/applog", 1.4969500000000001e-06, 1.6194500000000002e-06, "+8.18%"
            ),
        )
        self.assertEqual(rows[2].change, "~")

    def test_empty_cells_are_none(self):
        geomean = parse_benchstat_csv(BENCHSTAT_CSV)[-1]
        self.assertEqual(geomean, Row("allocs/op", "geomean", None, None, "?"))

    def test_no_tables(self):
        self.assertEqual(parse_benchstat_csv("goos: linux\ngoarch: amd64\n"), [])


class TestFindRegressions(unittest.TestCase):
    def test_allocating_fails_and_time_does_not(self):
        # The allocation regression goes from 0 to 1, so benchstat prints "?" instead of a percentage:
        # it must still be caught. The +8.18% sec/op row is significant, but time never fails the run.
        self.assertEqual(
            find_regressions(parse_benchstat_csv(BENCHSTAT_CSV), 5),
            [
                "Consumer/aad_log_signature/applog B/op: 512 -> 1024",
                "Consumer/logs_auto_multiline/short allocs/op: 0 -> 1",
            ],
        )

    def test_time_never_fails(self):
        rows = [
            Row("sec/op", "Consumer/a", 1e-06, 2e-06, "+100.00%"),
            Row("sec/op", "geomean", 1e-06, 2e-06, "+100.00%"),
        ]
        self.assertEqual(find_regressions(rows, 5), [])

    def test_bigger_allocations_fail(self):
        # Same allocation count, but each one is bigger: only B/op shows it.
        rows = [
            Row("allocs/op", "Consumer/a", 1, 1, "~"),
            Row("B/op", "Consumer/a", 50, 5000, "+9900.00%"),
            Row("B/op", "Consumer/b", 0, 16, "?"),
        ]
        self.assertEqual(find_regressions(rows, 5), ["Consumer/a B/op: 50 -> 5000", "Consumer/b B/op: 0 -> 16"])

    def test_small_byte_growth_and_geomean_pass(self):
        rows = [
            Row("B/op", "Consumer/a", 1000, 1040, "+4.00%"),
            Row("B/op", "Consumer/b", 512, 0, "-100.00%"),
            Row("B/op", "geomean", 100, 1000, "+900.00%"),
        ]
        self.assertEqual(find_regressions(rows, 5), [])

    def test_fractional_bytes_need_significance(self):
        # Real CI row from an A/A run (same code on both sides): a buffer that grows on some iterations
        # makes B/op fractional, and the medians moved 2.5 -> 3 (+20%). benchstat says it's noise.
        rows = parse_benchstat_csv(
            ",base,,head,,,\n"
            ",B/op,CI,B/op,CI,vs base,P\n"
            "Consumer/logs_adaptive_sampler/dense,2.5,20%,3,33%,~,p=0.350 n=10\n"
            "Consumer/aad_log_signature/applog,512,0%,1024,0%,+100.00%,p=0.000 n=10\n"
            "\n"
            ",base,,head,,,\n"
            ",allocs/op,CI,allocs/op,CI,vs base,P\n"
            "Consumer/logs_adaptive_sampler/dense,0,0%,1,0%,~,p=0.474 n=10\n"
        )
        self.assertEqual(find_regressions(rows, 5), ["Consumer/aad_log_signature/applog B/op: 512 -> 1024"])

    def test_faster_and_fewer_allocations_pass(self):
        rows = [
            Row("sec/op", "Consumer/a", 1e-06, 8e-07, "-20.00%"),
            Row("allocs/op", "Consumer/a", 7, 1, "-85.71%"),
        ]
        self.assertEqual(find_regressions(rows, 5), [])


class TestMarkdownReport(unittest.TestCase):
    def setUp(self):
        self.rows = parse_benchstat_csv(BENCHSTAT_CSV)

    def test_regression(self):
        report = markdown_report(
            "Benchmarks of `pkg/logs/tokenizer`",
            self.rows,
            find_regressions(self.rows, 5),
            "4a84f5c051b0d",
            5,
            "the benchstat tables",
        )
        self.assertIn("### Benchmarks of `pkg/logs/tokenizer`", report)
        self.assertIn("**Regression** against `4a84f5c051b`", report)
        self.assertIn("- `Consumer/aad_log_signature/applog B/op: 512 -> 1024`", report)
        self.assertIn(f"add the `{BYPASS_LABEL}` label", report)
        # Significant allocation and byte changes only, with readable values; "~" rows and every time
        # row stay in the benchstat block.
        self.assertIn("| Consumer/logs_auto_multiline/short | allocs/op | 0 | 1 | new allocations |", report)
        self.assertIn("| Consumer/aad_log_signature/applog | B/op | 512 | 1024 | +100.00% |", report)
        self.assertNotIn("| Consumer/logs_auto_multiline/short | B/op", report)
        self.assertNotIn("| sec/op |", report)
        self.assertNotIn("| geomean |", report)
        self.assertIn("Time is not judged", report)
        self.assertIn("```\nthe benchstat tables\n```", report)

    def test_no_regression(self):
        report = markdown_report("Benchmarks", self.rows, [], "4a84f5c051b0d", 5, "tables")
        self.assertIn("**No regression**", report)
        self.assertNotIn(BYPASS_LABEL, report)

    def test_pr_59467_on_a_noisy_runner(self):
        # The CI run that reported every consumer ~20% faster now shows only the allocation and byte
        # changes TokensToString really made, and passes.
        rows = parse_benchstat_csv(NOISY_CSV)
        self.assertEqual(find_regressions(rows, 5), [])
        report = markdown_report("Benchmarks", rows, [], "4a84f5c051b0d", 5, "tables")
        self.assertIn("| Consumer/aad_log_signature/applog | allocs/op | 7 | 1 | -85.71% |", report)
        self.assertIn("| Consumer/aad_log_signature/applog | B/op | 1016 | 512 | -49.61% |", report)
        self.assertNotIn("-12.88%", report)
        self.assertNotIn("-18.27%", report)


# Real run: the CI job of PR #59467 on a shared runner, trimmed to a few rows. That PR only changes
# TokensToString, which logs_auto_multiline and logs_adaptive_sampler never call, so their true change is ~0%,
# yet benchstat reported -12% to -28% on them. Every ± band is 31% to 86%.
NOISY_CSV = """goos: linux
goarch: amd64
,base,,head,,,
,sec/op,CI,sec/op,CI,vs base,P
Consumer/logs_auto_multiline/short,4.071e-08,60%,4.195e-08,45%,~,p=0.579 n=10
Consumer/logs_auto_multiline/applog,2.52e-07,44%,2.195e-07,61%,-12.88%,p=0.043 n=10
Consumer/aad_log_signature/applog,3.776e-06,35%,2.617e-06,60%,-30.70%,p=0.003 n=10
geomean,8.136e-07,,6.649e-07,,-18.27%,

,base,,head,,,
,B/op,CI,B/op,CI,vs base,P
Consumer/logs_auto_multiline/short,0,0%,0,0%,~,p=1.000 n=10
Consumer/aad_log_signature/applog,1016,0%,512,0%,-49.61%,p=0.000 n=10
geomean,,,,,-24.65%,

,base,,head,,,
,allocs/op,CI,allocs/op,CI,vs base,P
Consumer/logs_auto_multiline/short,0,0%,0,0%,~,p=1.000 n=10
Consumer/aad_log_signature/applog,7,0%,1,0%,-85.71%,p=0.000 n=10
geomean,,,,,-42.10%,
"""


class TestBenchmarkMetrics(unittest.TestCase):
    def test_head_side_per_benchmark(self):
        metrics = benchmark_metrics(parse_benchstat_csv(BENCHSTAT_CSV))
        by_key = {(name, benchmark): (value, unit) for name, value, benchmark, unit in metrics}
        value, unit = by_key[("datadog.agent.benchmark.time_per_op", "Consumer/aad_log_signature/applog")]
        self.assertAlmostEqual(value, 1619.45)
        self.assertEqual(unit, "nanosecond")
        value, unit = by_key[("datadog.agent.benchmark.time_change", "Consumer/aad_log_signature/applog")]
        self.assertAlmostEqual(value, 8.1833, places=3)
        self.assertEqual(unit, "percent")
        # Sent even when not significant, so noise shows over time.
        self.assertIn(("datadog.agent.benchmark.time_change", "Consumer/logs_auto_multiline/short"), by_key)
        self.assertEqual(
            by_key[("datadog.agent.benchmark.allocs_per_op", "Consumer/logs_auto_multiline/short")], (1, None)
        )
        self.assertEqual(
            by_key[("datadog.agent.benchmark.bytes_per_op", "Consumer/aad_log_signature/applog")], (1024, "byte")
        )
        # The geomean has a time but no allocation or byte count.
        self.assertIn(("datadog.agent.benchmark.time_per_op", "geomean"), by_key)
        self.assertNotIn(("datadog.agent.benchmark.allocs_per_op", "geomean"), by_key)
        self.assertNotIn(("datadog.agent.benchmark.bytes_per_op", "geomean"), by_key)
        self.assertEqual(len(metrics), 4 * 2 + 3 + 3)


class TestBypassed(unittest.TestCase):
    def test_label_accepts_a_regression(self):
        self.assertTrue(bypassed({"verdict": "regression"}, ["team/agent-log-pipelines", BYPASS_LABEL]))

    def test_without_the_label(self):
        self.assertFalse(bypassed({"verdict": "regression"}, ["team/agent-log-pipelines"]))


if __name__ == "__main__":
    unittest.main()
