import json
import tempfile
import threading
import unittest
import urllib.request
from http.server import HTTPServer
from pathlib import Path

from tasks.libs.cws import local_e2e

EVENT = {
    "agent": {"rule_id": "my_rule"},
    "date": "2026-10-23T16:29:02.429264436Z",
    "process": {"executable": {"name": "touch"}, "ancestors": [{"pid": 1}]},
    "exit": {"code": 0},
    "file": {"in_upper_layer": False},
}


class TestOverrides(unittest.TestCase):
    def test_nested_key_is_created_and_value_parsed_as_yaml(self):
        config = {"runtime_security_config": {"enabled": True}}
        local_e2e.apply_override(config, "runtime_security_config.activity_dump.enabled=true")
        local_e2e.apply_override(config, "runtime_security_config.event_server.retention=1s")
        self.assertEqual(
            config["runtime_security_config"],
            {"enabled": True, "activity_dump": {"enabled": True}, "event_server": {"retention": "1s"}},
        )

    def test_rejects_missing_equal_sign(self):
        with self.assertRaises(ValueError):
            local_e2e.apply_override({}, "runtime_security_config.enabled")

    def test_rejects_descending_into_a_scalar(self):
        with self.assertRaises(ValueError):
            local_e2e.apply_override({"a": 1}, "a.b=2")


class TestExpectations(unittest.TestCase):
    def test_matching_expectations(self):
        expects = ["process.executable.name=touch", "exit.code=0", "file.in_upper_layer=false"]
        self.assertEqual(local_e2e.check_expectations(EVENT, expects), [])

    def test_list_index(self):
        self.assertEqual(local_e2e.check_expectations(EVENT, ["process.ancestors.0.pid=1"]), [])

    def test_mismatch_and_missing_are_reported(self):
        failures = local_e2e.check_expectations(EVENT, ["process.executable.name=cat", "process.file.name=touch"])
        self.assertEqual(len(failures), 2)
        self.assertIn("got 'touch', want 'cat'", failures[0])
        self.assertIn("missing", failures[1])

    def test_value_may_contain_equal_sign(self):
        event = {"process": {"args": "a=b"}}
        self.assertEqual(local_e2e.check_expectations(event, ["process.args=a=b"]), [])


class TestEventFields(unittest.TestCase):
    def test_rule_id(self):
        self.assertEqual(local_e2e.rule_id(EVENT), "my_rule")
        self.assertIsNone(local_e2e.rule_id({"raw_message": "x"}))

    def test_event_time_handles_nanoseconds_and_zulu(self):
        self.assertAlmostEqual(local_e2e.event_time(EVENT), 1792772942.429264, places=5)
        self.assertEqual(local_e2e.event_time({"date": "2026-10-23T16:29:02Z"}), 1792772942.0)
        self.assertIsNone(local_e2e.event_time({}))
        self.assertIsNone(local_e2e.event_time({"date": "yesterday"}))

    def test_rule_statuses_only_lists_file_policies(self):
        ruleset_loaded = {
            "policies": [
                {"name": "a.policy", "source": "file", "rules": [{"id": "ok", "status": "loaded"}]},
                {
                    "name": "a.policy",
                    "source": "file",
                    "rules": [{"id": "bad", "status": "error", "message": "field not found"}],
                },
                {"name": "internal", "source": "internal", "rules": [{"id": "x", "status": "loaded"}]},
            ]
        }
        self.assertEqual(
            local_e2e.rule_statuses(ruleset_loaded),
            [("a.policy", "ok", "loaded", ""), ("a.policy", "bad", "error", "field not found")],
        )


class TestSink(unittest.TestCase):
    def test_intake_payload_round_trip(self):
        with tempfile.TemporaryDirectory() as tmp:
            events_file = Path(tmp) / "events.jsonl"
            handler = type("H", (local_e2e._SinkHandler,), {"events_file": events_file})
            server = HTTPServer(("127.0.0.1", 0), handler)
            thread = threading.Thread(target=server.serve_forever, daemon=True)
            thread.start()
            try:
                # The intake payload: a JSON array whose `message` is the serialized event.
                body = json.dumps([{"message": json.dumps(EVENT)}, {"message": "not json"}]).encode()
                request = urllib.request.Request(
                    f"http://127.0.0.1:{server.server_port}/api/v2/secruntime", data=body, method="POST"
                )
                urllib.request.urlopen(request).read()
            finally:
                server.shutdown()
                server.server_close()

            records = local_e2e.read_events(events_file)
            self.assertEqual([r["event"] for r in records], [EVENT, {"raw_message": "not json"}])
            self.assertEqual(records[0]["path"], "/api/v2/secruntime")

    def test_partial_trailing_line_is_ignored(self):
        with tempfile.TemporaryDirectory() as tmp:
            events_file = Path(tmp) / "events.jsonl"
            events_file.write_text(json.dumps({"event": EVENT}) + "\n" + '{"event": {"ag')
            self.assertEqual(len(local_e2e.read_events(events_file)), 1)


if __name__ == "__main__":
    unittest.main()
