import unittest
from pathlib import Path
import sys

REPO_ROOT = Path(__file__).resolve().parents[2]
if str(REPO_ROOT) not in sys.path:
    sys.path.insert(0, str(REPO_ROOT))

from tools.logs_agent_ai.frontmatter import parse_frontmatter
from tools.logs_agent_ai.llm import LLMError
from tools.logs_agent_ai.review import (
    ReviewFinding,
    build_review_prompts,
    prepare_review_comments,
    validate_review_payload,
)
from tools.logs_agent_ai.wiki import WikiPage, determine_impacted_page_specs, select_review_pages


class TestLogsAgentAIPathSelection(unittest.TestCase):
    def test_sender_path_maps_to_sender_and_invariants(self):
        specs = determine_impacted_page_specs(["pkg/logs/sender/worker.go"])
        impacted = {spec.path for spec in specs}
        self.assertIn("components/sender.md", impacted)
        self.assertIn("architecture/pipeline-flow.md", impacted)
        self.assertIn("invariants/sender-destination-semantics.md", impacted)
        self.assertIn("invariants/auditor-delivery.md", impacted)

    def test_restart_path_maps_to_restart_pages(self):
        specs = determine_impacted_page_specs(["comp/logs/agent/agentimpl/agent_restart.go"])
        impacted = {spec.path for spec in specs}
        self.assertIn("components/restart-lifecycle.md", impacted)
        self.assertIn("invariants/graceful-restart.md", impacted)


class TestLogsAgentAIWikiMetadata(unittest.TestCase):
    def test_seeded_page_frontmatter_has_required_fields(self):
        page = REPO_ROOT / ".llm/logs-agent/invariants/auditor-delivery.md"
        meta, body = parse_frontmatter(page.read_text())
        self.assertEqual(meta["title"], "Auditor Delivery and Persistence")
        self.assertEqual(meta["kind"], "invariant")
        self.assertIn("comp/logs/auditor/**", meta["owns_globs"])
        self.assertIn("duplicate", body.lower())

    def test_review_selection_keeps_required_invariants(self):
        pages = {
            "components/sender.md": WikiPage(
                path="components/sender.md",
                meta={"owns_globs": ["pkg/logs/sender/**"], "title": "Sender"},
                body="sender body",
            ),
            "invariants/auditor-delivery.md": WikiPage(
                path="invariants/auditor-delivery.md",
                meta={"owns_globs": ["pkg/logs/sender/**"], "title": "Auditor invariant"},
                body="auditor invariant body",
            ),
            "invariants/graceful-restart.md": WikiPage(
                path="invariants/graceful-restart.md",
                meta={"owns_globs": ["comp/logs/agent/agentimpl/**"], "title": "Restart invariant"},
                body="restart invariant body",
            ),
        }
        selected = select_review_pages(
            ["pkg/logs/sender/worker.go"],
            pages,
            ("invariants/graceful-restart.md",),
        )
        selected_paths = [page.path for page in selected]
        self.assertIn("components/sender.md", selected_paths)
        self.assertIn("invariants/graceful-restart.md", selected_paths)


class TestLogsAgentAIReviewPrompts(unittest.TestCase):
    def _page(self, path: str, title: str, body: str) -> WikiPage:
        return WikiPage(path=path, meta={"title": title, "summary": f"summary for {title}"}, body=body)

    def test_sender_auditor_prompt_contains_duplicate_log_context(self):
        diff = """diff --git a/pkg/logs/sender/worker.go b/pkg/logs/sender/worker.go
@@ -10,3 +10,4 @@
-old
+new
"""
        system_prompt, user_prompt = build_review_prompts(
            [
                self._page("components/sender.md", "Sender", "Reliable destinations feed the auditor."),
                self._page("invariants/auditor-delivery.md", "Auditor", "Do not advance offsets early."),
            ],
            diff,
            ["pkg/logs/sender/worker.go"],
        )
        self.assertIn("logs-agent architecture reviewer", system_prompt)
        self.assertIn("destination-to-auditor ack flow", user_prompt)
        self.assertIn("Do not advance offsets early", user_prompt)

    def test_launcher_prompt_contains_source_service_questions(self):
        diff = """diff --git a/pkg/logs/launchers/container/launcher.go b/pkg/logs/launchers/container/launcher.go
@@ -20,3 +20,4 @@
-old
+new
"""
        _, user_prompt = build_review_prompts(
            [
                self._page(
                    "invariants/launcher-source-service-contracts.md",
                    "Launcher contracts",
                    "Container launchers reconcile services and sources.",
                )
            ],
            diff,
            ["pkg/logs/launchers/container/launcher.go"],
        )
        self.assertIn("launcher/source/service interactions", user_prompt)
        self.assertIn("Container launchers reconcile services and sources", user_prompt)

    def test_restart_prompt_contains_flush_and_lifecycle_questions(self):
        diff = """diff --git a/comp/logs/agent/agentimpl/agent_restart.go b/comp/logs/agent/agentimpl/agent_restart.go
@@ -40,3 +40,4 @@
-old
+new
"""
        _, user_prompt = build_review_prompts(
            [
                self._page("components/restart-lifecycle.md", "Restart", "Flush follows pipeline stop."),
                self._page("invariants/graceful-restart.md", "Restart invariant", "Persistent components survive restart."),
            ],
            diff,
            ["comp/logs/agent/agentimpl/agent_restart.go"],
        )
        self.assertIn("graceful degradation and restart lifecycle behavior", user_prompt)
        self.assertIn("Flush follows pipeline stop", user_prompt)


class TestLogsAgentAIReviewValidation(unittest.TestCase):
    def test_rejects_malformed_payload(self):
        with self.assertRaises(LLMError):
            validate_review_payload({"summary": "ok", "findings": [{"path": "foo"}]})

    def test_suppresses_low_confidence_and_unanchored_findings(self):
        findings = [
            ReviewFinding(
                path="pkg/logs/sender/worker.go",
                line=20,
                severity="high",
                title="Risk",
                body="Could duplicate logs",
                wiki_refs=["invariants/auditor-delivery.md"],
                confidence=0.40,
            ),
            ReviewFinding(
                path="pkg/logs/sender/worker.go",
                line=22,
                severity="high",
                title="Anchored risk",
                body="Could duplicate logs",
                wiki_refs=["invariants/auditor-delivery.md"],
                confidence=0.91,
            ),
        ]
        comments, suppressed = prepare_review_comments(
            findings,
            {"pkg/logs/sender/worker.go": {23}},
            ["pkg/logs/sender/worker.go"],
        )
        self.assertEqual(len(comments), 1)
        self.assertEqual(comments[0].line, 23)
        self.assertEqual(suppressed, 1)


if __name__ == "__main__":
    unittest.main()
