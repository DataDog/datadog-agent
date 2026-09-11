import json
import re
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
DOCS = ROOT / "docs/dev"
FLOW = DOCS / "gensim-global-1hz-e2e-flow.md"
SCORING_MATRIX = DOCS / "gensim-global-1hz-scoring-matrix.json"


class E2EFlowTest(unittest.TestCase):
    def setUp(self):
        self.flow = FLOW.read_text()
        self.matrix = json.loads(SCORING_MATRIX.read_text())

    def test_flow_has_brief_precursor_numbered_runbook_and_dated_status(self):
        required_sections = {
            "## What this experiment is",
            "## Precursor: e2e Smoke Run",
            "## Branches and external references",
            "## How the smoke becomes a scaled experiment",
            "## How to run the experiment at scale",
            "### 1. Freeze inputs",
            "### 2. Preflight the campaign",
            "### 3. Run captures",
            "### 4. Preserve evidence and clean up",
            "### 5. Archive through sims-controller",
            "### 6. Validate the archive directly",
            "### 7. Preview and ingest through Eval Data Portal",
            "### 8. Verify Scenario Store",
            "### 9. Run DDEval",
            "### 10. Score paired outcomes",
            "## Status update — 2026-09-10",
            "## Why identity linkage matters",
            "## Experiment requirements",
            "## Related documents",
        }
        headings = {line for line in self.flow.splitlines() if line.startswith("##")}
        self.assertTrue(required_sections.issubset(headings))
        self.assertIn("**Current stop point:**", self.flow)
        self.assertIn("archive_objects", self.flow)
        self.assertIn("72c8193a-4e6c-40cf-8120-510623bd1295", self.flow)
        self.assertNotIn("[an:", self.flow)
        self.assertIn("**Objective:** prove that the exact approved inputs are safe to mutate remotely", self.flow)
        self.assertIn("gs-episode-worker", self.flow)
        self.assertIn("version-3 provenance ledger", self.flow)
        self.assertIn("packet adaptations", self.flow)
        self.assertIn("two source packets encode an empty Helm value with single quotes", self.flow)
        self.assertIn("#### Why we did not stagger or parallelize captures", self.flow)
        self.assertIn("a fixed time delay", self.flow.lower())
        self.assertIn("prevents evidence from one arm", self.flow)
        self.assertIn("campaign arm workers", self.flow)
        self.assertIn("not Cloud Build subjobs", self.flow)
        self.assertNotIn("is a **Confluence access runbook**, not validation code", self.flow)
        self.assertNotIn("- **Excluded:** 2467", self.flow)

        freeze_section = self.flow.split("### 1. Freeze inputs", 1)[1].split("### 2. Preflight", 1)[0]
        status_section = self.flow.split("## Status update — 2026-09-10", 1)[1]
        self.assertNotIn("selected 2601 uses `corrected_2601_v1`", freeze_section)
        self.assertIn("selected 2601 uses `corrected_2601_v1`", status_section)

        h2 = [line for line in self.flow.splitlines() if line.startswith("## ")]
        self.assertEqual("## Status update — 2026-09-10", h2[-1])
        self.assertLess(self.flow.index("## Experiment requirements"), self.flow.index("## Status update — 2026-09-10"))

    def test_title_and_introduction_are_written_for_a_new_reader(self):
        self.assertTrue(self.flow.startswith("# How GenSim tests whether 1 Hz metrics improve Bits AI SRE\n"))
        for explanation in (
            "GenSim creates repeatable incidents",
            "Bits AI SRE",
            "control capture",
            "1 Hz capture",
            "DDEval",
        ):
            with self.subTest(explanation=explanation):
                self.assertIn(explanation, self.flow)

    def test_flow_counts_match_scoring_matrix(self):
        status = self.matrix["status"]
        self.assertIn(f"| Arms | {status['campaign_arms']} |", self.flow)
        self.assertIn(
            f"| DDEval candidates | {status['ddeval_candidates_after_event_and_telemetry_gates']} |",
            self.flow,
        )
        self.assertIn(f"| Current-arm DDEval runs | {status['ddeval_runs_completed']} |", self.flow)
        blocker = status["blocker"].split(";", 1)[0].removeprefix("rpc error: code = ")
        self.assertIn(blocker, self.flow)

    def test_links_are_external_and_local_context_is_collapsed(self):
        link_targets = re.findall(r"\[[^]]+\]\(([^)]+)\)", self.flow)
        local_targets = [target for target in link_targets if not target.startswith("https://")]
        self.assertEqual([], local_targets)

        required_links = {
            "https://github.com/DataDog/gensim/tree/ella/global-1hz-episode-campaign-controller",
            "https://github.com/DataDog/datadog-agent/tree/ella/global-1hz-metric-resolution-experiment",
            "https://github.com/DataDog/dd-source/pull/58419",
            "https://github.com/DataDog/dd-source/tree/ella/global-1hz-eval-prompt-current",
            "https://datadoghq.atlassian.net/wiki/spaces/ChatBot/pages/6833180773/ddeval+Running+Alert+Eval#Quick-start%3A-your-first-eval",
            "https://datadoghq.atlassian.net/wiki/spaces/ODP/pages/5629050883#S3-Access-for-Archived-Episode-Data",
            "https://app.datadoghq.com/bits-ai/investigations/1dab9720-bd55-4a08-b32b-c1eee9c33091",
        }
        self.assertTrue(required_links.issubset(link_targets))
        self.assertGreaterEqual(self.flow.count("<details>"), 3)
        self.assertEqual(self.flow.count("<details>"), self.flow.count("</details>"))

if __name__ == "__main__":
    unittest.main()
