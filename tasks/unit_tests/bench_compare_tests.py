import unittest

from tasks.libs.bench.compare import Row, find_regressions, parse_benchstat_csv

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
    def test_slower_and_allocating(self):
        # The allocation regression goes from 0 to 1, so benchstat prints "?" instead of a percentage:
        # it must still be caught.
        self.assertEqual(
            find_regressions(parse_benchstat_csv(BENCHSTAT_CSV), 5),
            [
                "Consumer/aad_log_signature/applog sec/op: +8.18%",
                "Consumer/logs_auto_multiline/short allocs/op: 0 -> 1",
            ],
        )

    def test_slowdown_within_threshold(self):
        self.assertEqual(
            find_regressions(parse_benchstat_csv(BENCHSTAT_CSV), 10),
            ["Consumer/logs_auto_multiline/short allocs/op: 0 -> 1"],
        )

    def test_bytes_per_op_does_not_fail(self):
        rows = [row for row in parse_benchstat_csv(BENCHSTAT_CSV) if row.unit == "B/op"]
        self.assertEqual(find_regressions(rows, 5), [])

    def test_geomean_slowdown_fails_without_a_significant_benchmark(self):
        rows = [
            Row("sec/op", "Consumer/a", 1e-06, 1.06e-06, "~"),
            Row("sec/op", "Consumer/b", 1e-06, 1.06e-06, "~"),
            Row("sec/op", "geomean", 1e-06, 1.06e-06, "+6.00%"),
        ]
        self.assertEqual(find_regressions(rows, 5), ["geomean sec/op: +6.00%"])

    def test_faster_and_fewer_allocations_pass(self):
        rows = [
            Row("sec/op", "Consumer/a", 1e-06, 8e-07, "-20.00%"),
            Row("allocs/op", "Consumer/a", 7, 1, "-85.71%"),
        ]
        self.assertEqual(find_regressions(rows, 5), [])


if __name__ == "__main__":
    unittest.main()
