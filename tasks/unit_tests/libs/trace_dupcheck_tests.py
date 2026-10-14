import unittest

from tasks.libs.trace_dupcheck import compare_reports, render_diff, render_without_baseline

BASELINE = "0123456789abcdef0123456789abcdef01234567"


def issue(subject, count, partners, title=None, created_by=("v04",)):
    return {
        "subject": subject,
        "title": title or f"{subject} repeats {next(iter(partners))}",
        "count": count,
        "created_by": list(created_by),
        "partners": partners,
    }


def report(*issues):
    return {
        "seed": 1,
        "occurrences": sum(i["count"] for i in issues),
        "issues": list(issues),
        "text": "Trace payload duplicates: ...",
    }


ENV = issue("span.attr[env]", 10, {"span.attr[deployment.environment]": 10})
DM = issue("chunk.tags[_dd.p.dm]", 5, {"span.attr[_dd.p.dm]": 5})


class TestCompareReports(unittest.TestCase):
    def test_same_reports(self):
        self.assertEqual(compare_reports(report(ENV, DM), report(ENV, DM)), [])

    def test_new_and_gone(self):
        sql = issue("span.attr[sql.query]", 3, {"span.resource": 3})
        changes = compare_reports(report(ENV, DM), report(ENV, sql))
        self.assertEqual(
            [(c["kind"], (c["head"] or c["base"])["subject"]) for c in changes],
            [("new", "span.attr[sql.query]"), ("gone", "chunk.tags[_dd.p.dm]")],
        )

    def test_count_change(self):
        changes = compare_reports(report(ENV), report(issue("span.attr[env]", 12, ENV["partners"])))
        self.assertEqual(len(changes), 1)
        self.assertEqual(changes[0]["kind"], "changed")
        self.assertEqual(changes[0]["new_partners"], [])

    def test_new_partner_is_the_same_issue(self):
        # The title changes with a second partner, the subject doesn't.
        head = issue(
            "span.attr[env]",
            10,
            {"span.attr[deployment.environment]": 8, "span.attr[service.env]": 2},
            title="span.attr[env] repeats another location",
        )
        changes = compare_reports(report(ENV), report(head))
        self.assertEqual(len(changes), 1)
        self.assertEqual(changes[0]["kind"], "changed")
        self.assertEqual(changes[0]["new_partners"], ["span.attr[service.env]"])
        self.assertEqual(changes[0]["removed_partners"], [])

    def test_partner_count_change(self):
        # One finding can repeat several partners: a partner count can grow
        # while the issue count and the partner names stay the same.
        base = issue("span.attr[env]", 10, {"span.attr[a]": 10, "span.attr[b]": 1})
        head = issue("span.attr[env]", 10, {"span.attr[a]": 10, "span.attr[b]": 10})
        changes = compare_reports(report(base), report(head))
        self.assertEqual(len(changes), 1)
        self.assertEqual(changes[0]["kind"], "changed")
        self.assertEqual(changes[0]["changed_partners"], [("span.attr[b]", 1, 10)])


class TestRenderDiff(unittest.TestCase):
    def test_no_change_is_empty(self):
        self.assertEqual(render_diff(report(ENV), report(ENV), BASELINE, checker_changed=False), "")

    def test_changes(self):
        sql = issue("span.attr[sql.query]", 3, {"span.resource": 3}, created_by=("v04", "v1"))
        body = render_diff(
            report(ENV, DM),
            report(issue("span.attr[env]", 12, ENV["partners"]), sql),
            BASELINE,
            checker_changed=False,
        )
        self.assertIn("[01234567](https://github.com/DataDog/datadog-agent/commit/" + BASELINE + ")", body)
        self.assertIn("**2 issues, 15 occurrences**", body)
        self.assertIn("| New | `span.attr[sql.query] repeats span.resource` | 3 | created by: v04, v1 |", body)
        self.assertIn(
            "| Changed | `span.attr[env] repeats span.attr[deployment.environment]` | 10 → 12 (+2) |  |", body
        )
        self.assertIn("| Gone | `chunk.tags[_dd.p.dm] repeats span.attr[_dd.p.dm]` | 5 → 0 |  |", body)
        self.assertIn("Trace payload duplicates: ...", body)
        self.assertNotIn("changes `pkg/trace/testutil/dupcheck`", body)

    def test_partner_count_change(self):
        base = issue(
            "span.attr[env]",
            10,
            {"span.attr[a]": 10, "span.attr[b]": 1},
            title="span.attr[env] repeats another location",
        )
        head = issue("span.attr[env]", 10, {"span.attr[a]": 10, "span.attr[b]": 1_500}, title=base["title"])
        body = render_diff(report(base), report(head), BASELINE, checker_changed=False)
        self.assertIn(
            "| Changed | `span.attr[env] repeats another location` | 10 | partner counts: `span.attr[b]` 1 → 1,500 |",
            body,
        )

    def test_partner_counts_are_capped(self):
        partners = {f"span.attr[p{i}]": i + 1 for i in range(5)}
        base = issue("span.attr[env]", 10, partners, title="span.attr[env] repeats another location")
        head = issue("span.attr[env]", 20, {k: v * 2 for k, v in partners.items()}, title=base["title"])
        body = render_diff(report(base), report(head), BASELINE, checker_changed=False)
        self.assertIn(
            "partner counts: `span.attr[p4]` 5 → 10, `span.attr[p3]` 4 → 8, `span.attr[p2]` 3 → 6, +2 more |", body
        )

    def test_checker_changed(self):
        body = render_diff(report(ENV), report(ENV, DM), BASELINE, checker_changed=True)
        self.assertIn("**2 issues (+1), 15 occurrences (+5)**", body)
        self.assertIn("changes `pkg/trace/testutil/dupcheck`", body)


class TestRenderWithoutBaseline(unittest.TestCase):
    def test_report(self):
        body = render_without_baseline(report(ENV), BASELINE)
        self.assertIn("has no report to compare with", body)
        self.assertIn("**1 issue, 10 occurrences**", body)
        self.assertIn("Trace payload duplicates: ...", body)
