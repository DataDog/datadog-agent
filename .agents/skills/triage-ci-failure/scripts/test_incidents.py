#!/usr/bin/env python3
"""Unit tests for incidents.py's pure logic, against captured real fixture shapes.

No `pup` calls here — everything below is JSON shapes captured live against real incidents
(IR-59848, IR-59770, IR-58327) and trimmed to the fields each test cares about. Stdlib only,
so this runs identically via `python3 test_incidents.py` or `python3 -m pytest
test_incidents.py` — no dependency on which one happens to be installed.
"""

from __future__ import annotations

import unittest

from incidents import (
    Incident,
    MatchTier,
    build_search_queries,
    classify_pair,
    describe_status_change,
    parse_iso,
    render_markdown_cell,
    slim_timeline,
)

# Trimmed real shape from `pup --no-agent incidents list` (IR-59848), reused across tests.
IR_59848_TITLE = (
    '[INTERNAL] new-e2e-sbom: [--run &quot;TestSBOMKubeadmCrioSuite&quot;] job is '
    "broken on the main branch of the `datadog-agent` repository"
)


def make_incident(*, title: str = IR_59848_TITLE, created: str = "2026-08-27T13:05:22.790651Z", resolved=None):
    """An Incident built the way production builds them — through `from_api`, so the
    fixture exercises the real field mapping and title unescaping rather than bypassing it."""
    return Incident.from_api(
        {
            "id": "2150a8f5-14e2-45bc-ad9c-2de3eaa67e58",
            "attributes": {
                "public_id": 59848,
                "fields": {"slug": {"value": "IR-59848"}},
                "state": "stable",
                "severity": "SEV-4",
                "created": created,
                "resolved": resolved,
                "title": title,
            },
        }
    )


class ParsedTitleTests(unittest.TestCase):
    def test_html_escaped_matrix_job(self):
        # Real title from IR-59848. Parallel/matrix suffixes are preserved verbatim, and
        # the title is HTML-escaped at declaration time.
        inc = make_incident()
        self.assertEqual(inc.parsed_job, 'new-e2e-sbom: [--run "TestSBOMKubeadmCrioSuite"]')
        self.assertEqual(inc.parsed_branch, "main")

    def test_non_main_branch(self):
        # Real title from IR-59770.
        inc = make_incident(
            title=(
                "[INTERNAL] new-e2e-containers-k8s-latest job is broken on the 7.83.x branch "
                "of the `datadog-agent` repository"
            )
        )
        self.assertEqual(inc.parsed_job, "new-e2e-containers-k8s-latest")
        self.assertEqual(inc.parsed_branch, "7.83.x")

    def test_human_declared_title_does_not_match(self):
        # Real title from IR-58327 — a human-declared incident, not the [INTERNAL] bot.
        inc = make_incident(
            title="cloudregistry CrashLoopBackOff on mudkip-b - Egress gateway blocking ECR us-east-2 authentication"
        )
        self.assertIsNone(inc.parsed_job)
        self.assertIsNone(inc.parsed_branch)


class ClassifyPairTests(unittest.TestCase):
    def test_exact(self):
        self.assertIs(classify_pair("go_e2e_test_binaries", "go_e2e_test_binaries"), MatchTier.EXACT)

    def test_base_strips_parallel_suffix(self):
        tier = classify_pair('new-e2e-sbom: [--run "TestSBOMKubeadmCrioSuite"]', "new-e2e-sbom")
        self.assertIs(tier, MatchTier.BASE)

    def test_prefix_when_title_is_truncated(self):
        # Simulates a future truncated-title regime: title has a clipped name, GitLab has
        # the real, longer one.
        tier = classify_pair("kmt_run_secagent_tests_ubuntu_2004", "kmt_run_secagent_tests_ubuntu_2004_x64_extra")
        self.assertIs(tier, MatchTier.PREFIX)

    def test_token_fallback(self):
        self.assertIs(classify_pair("new-e2e-containers-k8s-latest", "new-e2e-containers-k8s-oldest"), MatchTier.TOKEN)

    def test_none_when_nothing_lines_up(self):
        self.assertIs(classify_pair("go_e2e_test_binaries", "docker_build_agent7_full_arm64"), MatchTier.NONE)

    def test_tiers_order_best_first(self):
        # The enum's ordering is load-bearing: cmd_search sorts on it directly and
        # best_tier is a min(), so nothing may sort better than EXACT or worse than NONE.
        self.assertEqual(
            sorted([MatchTier.NONE, MatchTier.BASE, MatchTier.EXACT, MatchTier.TOKEN, MatchTier.PREFIX]),
            [MatchTier.EXACT, MatchTier.BASE, MatchTier.PREFIX, MatchTier.TOKEN, MatchTier.NONE],
        )


class MatchAgainstTests(unittest.TestCase):
    def test_rates_every_job_not_just_the_first_match(self):
        # One [INTERNAL] title names a single job, but sibling shards of a parallel job all
        # legitimately correspond to it, so every one is rated rather than just the first.
        inc = make_incident()
        matches = inc.match_against(["new-e2e-sbom: [shard 1/2]", "new-e2e-sbom: [shard 2/2]", "unrelated_job"])
        self.assertEqual(matches["new-e2e-sbom: [shard 1/2]"], MatchTier.BASE)
        self.assertEqual(matches["new-e2e-sbom: [shard 2/2]"], MatchTier.BASE)
        self.assertEqual(matches["unrelated_job"], MatchTier.NONE)

    def test_exact_wins_over_base_for_best_tier(self):
        inc = make_incident()
        inc.matches = inc.match_against(['new-e2e-sbom: [--run "TestSBOMKubeadmCrioSuite"]', "new-e2e-sbom: [other]"])
        self.assertIs(inc.best_tier, MatchTier.EXACT)

    def test_best_tier_is_none_when_no_job_matches(self):
        inc = make_incident()
        inc.matches = inc.match_against(["docker_build_agent7_full_arm64"])
        self.assertIs(inc.best_tier, MatchTier.NONE)

    def test_no_matches_when_title_did_not_parse(self):
        inc = make_incident(title="a human-written title")
        self.assertEqual(inc.match_against(["go_e2e_test_binaries"]), {})
        self.assertIs(inc.best_tier, MatchTier.NONE)

    def test_best_tier_is_none_when_no_jobs_were_supplied(self):
        # The tier-3 broad search runs without --job at all.
        inc = make_incident()
        inc.matches = inc.match_against([])
        self.assertIs(inc.best_tier, MatchTier.NONE)


class WasActiveAtTests(unittest.TestCase):
    WINDOW_S = 48 * 3600
    GRACE_S = 6 * 3600
    T_FAIL = parse_iso("2026-08-27T12:45:00Z")

    def test_declared_shortly_after_the_failure_still_counts(self):
        # Incidents are declared after their first casualties, so this is the common case.
        inc = make_incident(created="2026-08-27T13:05:22Z")
        self.assertTrue(inc.was_active_at(self.T_FAIL, self.WINDOW_S, self.GRACE_S))

    def test_declared_past_the_grace_period_does_not_count(self):
        inc = make_incident(created="2026-08-27T20:00:00Z")  # 7h15 later, past the 6h grace
        self.assertFalse(inc.was_active_at(self.T_FAIL, self.WINDOW_S, self.GRACE_S))

    def test_resolved_well_before_the_window_does_not_count(self):
        inc = make_incident(created="2026-08-20T00:00:00Z", resolved="2026-08-24T00:00:00Z")
        self.assertFalse(inc.was_active_at(self.T_FAIL, self.WINDOW_S, self.GRACE_S))

    def test_resolved_inside_the_window_still_counts(self):
        inc = make_incident(created="2026-08-20T00:00:00Z", resolved="2026-08-26T12:00:00Z")
        self.assertTrue(inc.was_active_at(self.T_FAIL, self.WINDOW_S, self.GRACE_S))


class IncidentSerialisationTests(unittest.TestCase):
    def test_from_api_unescapes_title_and_derives_fields(self):
        inc = make_incident()
        self.assertEqual(inc.slug, "IR-59848")
        self.assertEqual(inc.public_id, 59848)
        self.assertEqual(inc.severity, "SEV-4")
        self.assertIsNone(inc.resolved)
        self.assertIn('"TestSBOMKubeadmCrioSuite"', inc.title)  # unescaped, not &quot;
        self.assertEqual(inc.parsed_job, 'new-e2e-sbom: [--run "TestSBOMKubeadmCrioSuite"]')
        self.assertEqual(inc.parsed_branch, "main")
        self.assertEqual(inc.url, "https://app.datadoghq.com/incidents/59848")

    def test_to_dict_is_json_serialisable_and_carries_derived_fields(self):
        # Guards the --json path: derived values live on properties, which generic
        # dataclass serialisation would silently drop.
        import json

        inc = make_incident()
        inc.matches = inc.match_against(["new-e2e-sbom"])
        payload = json.loads(json.dumps(inc.to_dict()))
        self.assertEqual(payload["parsed_job"], 'new-e2e-sbom: [--run "TestSBOMKubeadmCrioSuite"]')
        self.assertEqual(payload["best_tier"], "base")
        self.assertEqual(payload["matches"], {"new-e2e-sbom": "base"})
        self.assertEqual(payload["url"], "https://app.datadoghq.com/incidents/59848")

    def test_str_renders_three_lines_with_title_and_url(self):
        inc = make_incident()
        inc.matches = inc.match_against(["new-e2e-sbom"])
        lines = str(inc).splitlines()
        self.assertEqual(len(lines), 3)
        self.assertIn("IR-59848", lines[0])
        self.assertIn("match=base (new-e2e-sbom)", lines[0])
        self.assertIn("resolved -", lines[0])
        self.assertIn("https://app.datadoghq.com/incidents/59848", lines[2])

    def test_str_summarises_multiple_matches_at_the_same_tier(self):
        inc = make_incident()
        inc.matches = inc.match_against(["new-e2e-sbom: [shard 1/2]", "new-e2e-sbom: [shard 2/2]"])
        self.assertIn("match=base (2 jobs:", str(inc).splitlines()[0])

    def test_str_names_no_job_when_nothing_matched(self):
        # `matches` keeps jobs rated NONE, so the summary must not name them as matches.
        inc = make_incident()
        inc.matches = inc.match_against(["docker_build_agent7_full_arm64"])
        header = str(inc).splitlines()[0]
        self.assertTrue(header.rstrip().endswith("match=none"), header)
        self.assertNotIn("docker_build_agent7_full_arm64", header)

    def test_str_names_no_job_when_no_jobs_were_supplied(self):
        # The tier-3 broad search runs without --job, so there is nothing to count.
        inc = make_incident()
        inc.matches = inc.match_against([])
        header = str(inc).splitlines()[0]
        self.assertTrue(header.rstrip().endswith("match=none"), header)
        self.assertNotIn("0 jobs", header)


class BuildSearchQueriesTests(unittest.TestCase):
    def test_default_service_scoped_pair(self):
        declared, still_open = build_search_queries(1000, 2000, service="datadog-agent-ci", text=None)
        self.assertEqual(declared, "services:datadog-agent-ci created_after:1000 created_before:2000")
        self.assertEqual(still_open, "services:datadog-agent-ci state:(active OR stable) created_before:1000")

    def test_tier_three_drops_service_and_adds_text(self):
        declared, still_open = build_search_queries(1000, 2000, service=None, text="ECR")
        self.assertEqual(declared, "ECR created_after:1000 created_before:2000")
        self.assertEqual(still_open, "ECR state:(active OR stable) created_before:1000")


class DescribeStatusChangeTests(unittest.TestCase):
    def test_declared(self):
        cell = {"attributes": {"content": {"action": "created", "after": {"state": "active", "severity": "SEV-4"}}}}
        self.assertEqual(describe_status_change(cell), "declared (active, SEV-4)")

    def test_direct_state_transition(self):
        # Real shape: IR-59848's active -> stable cell.
        cell = {
            "attributes": {
                "content": {"action": "updated", "before": {"state": "active"}, "after": {"state": "stable"}}
            }
        }
        self.assertEqual(describe_status_change(cell), "active \u2192 stable")

    def test_selections_diff_on_routing_fields_is_noise(self):
        # Real shape: a `teams` selections diff — routing/labelling, not triage-relevant.
        cell = {
            "attributes": {
                "content": {
                    "action": "updated",
                    "before": {"selections": [{"field": {"name": "teams"}, "choices": [{"value": "agent-devx"}]}]},
                    "after": {
                        "selections": [
                            {
                                "field": {"name": "teams"},
                                "choices": [{"value": "agent-devx"}, {"value": "ci-execution"}],
                            }
                        ]
                    },
                }
            }
        }
        self.assertIsNone(describe_status_change(cell))

    def test_selections_diff_on_severity_is_reported(self):
        cell = {
            "attributes": {
                "content": {
                    "action": "updated",
                    "before": {"selections": [{"field": {"name": "severity"}, "choices": [{"value": "SEV-4"}]}]},
                    "after": {"selections": [{"field": {"name": "severity"}, "choices": [{"value": "SEV-2"}]}]},
                }
            }
        }
        self.assertEqual(describe_status_change(cell), "severity: ['SEV-4'] \u2192 ['SEV-2']")


class MarkdownCellTests(unittest.TestCase):
    def test_truncates_and_unescapes(self):
        cell = {"attributes": {"content": {"author": {"name": "Kevin Fairise"}, "content": "a" * 500 + " &amp; more"}}}
        who, text = render_markdown_cell(cell, max_chars=50)
        self.assertEqual(who, "Kevin Fairise")
        self.assertTrue(text.endswith("\u2026"))
        self.assertLessEqual(len(text), 51)

    def test_falls_back_to_email_then_unknown(self):
        cell = {"attributes": {"content": {"author": {"email": "regis@example.com"}, "content": "hi"}}}
        who, _ = render_markdown_cell(cell, max_chars=50)
        self.assertEqual(who, "regis@example.com")


class SlimTimelineTests(unittest.TestCase):
    def test_drops_routing_cells_and_sorts_chronologically(self):
        cells = [
            {
                "attributes": {
                    "created": "2026-08-27T13:19:22Z",
                    "cell_type": "markdown",
                    "content": {"author": {"name": "Kevin"}, "content": "Trying to replicate"},
                }
            },
            {
                "attributes": {
                    "created": "2026-08-27T13:05:22.790651+00:00",
                    "cell_type": "incident_status_change",
                    "content": {"action": "created", "after": {"state": "active", "severity": "SEV-4"}},
                }
            },
            {
                "attributes": {
                    "created": "2026-08-27T13:05:26+00:00",
                    "cell_type": "incident_workstream_change",
                    "content": {"action": "created"},
                }
            },
        ]
        entries = slim_timeline(cells, max_chars=400)
        # The workstream_change cell is dropped; the remaining two are chronological.
        # Note the mixed `Z` / `+00:00` suffixes: ordering must not rely on string compare.
        self.assertEqual(len(entries), 2)
        self.assertEqual(entries[0]["kind"], "status")
        self.assertEqual(entries[1]["kind"], "note")


if __name__ == "__main__":
    unittest.main()
