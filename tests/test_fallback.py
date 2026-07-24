import importlib.util
import json
import pathlib
import subprocess
import tempfile
import unittest
from unittest import mock


ROOT = pathlib.Path(__file__).resolve().parents[1]
SCRIPT = ROOT / "skills" / "gh-code-review" / "scripts" / "gh_code_review_fallback.py"
SPEC = importlib.util.spec_from_file_location("gh_code_review_fallback", SCRIPT)
fallback = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(fallback)


class FallbackParityTests(unittest.TestCase):
    def fixture(self, name):
        return json.loads((ROOT / "tests" / "fixtures" / name).read_text())

    def test_line_manifest_and_different_size_suggestion(self):
        manifest = self.fixture("review_line.json")
        self.assertEqual([], fallback.static_validate(manifest))
        body = fallback.suggestion_body(manifest["comments"][0])
        self.assertIn("```suggestion\none\nline\n```", body)

    def test_mixed_manifest_and_derived_event(self):
        manifest = self.fixture("review_mixed.json")
        manifest.pop("event")
        self.assertEqual([], fallback.static_validate(manifest, derive=True))
        self.assertEqual("REQUEST_CHANGES", manifest["event"])

    def test_diff_fixture_covers_location_boundaries(self):
        files, locations, previous = fallback.parse_files(self.fixture("diff_files.json"))
        self.assertIn(("src/service.go", 11, "LEFT"), locations)
        self.assertIn(("src/service.go", 11, "RIGHT"), locations)
        self.assertIn(("src/service.go", 13, "RIGHT"), locations)
        self.assertIn(("src/deleted.go", 1, "LEFT"), locations)
        self.assertEqual("src/new_name.go", previous["src/old_name.go"])
        binary = next(item for item in files if item["path"] == "assets/logo.png")
        self.assertTrue(binary["binary"])

    def test_rejects_file_suggestion_and_left_suggestion(self):
        for comment in (
            {"client_id": "f", "subject": "file", "path": "a.go", "body": "x", "replacement": "y"},
            {"client_id": "l", "subject": "line", "path": "a.go", "body": "x", "line": 1, "side": "LEFT", "replacement": "y"},
        ):
            manifest = {
                "schema_version": "1.0",
                "repository": "o/r",
                "pull_request": 1,
                "expected_head_sha": "h",
                "event": "COMMENT",
                "comments": [comment],
            }
            codes = {item["code"] for item in fallback.static_validate(manifest)}
            self.assertTrue({"FILE_SUGGESTION", "SUGGESTION_SIDE"} & codes)

    def test_envelope_is_versioned(self):
        value = fallback.envelope(True, "validate", "o/r", 1, {"valid": True})
        self.assertEqual("1.0", value["schema_version"])
        self.assertTrue(value["ok"])
        self.assertEqual("validate", value["operation"])

    def test_secondary_rate_limit_retries(self):
        responses = [
            subprocess.CompletedProcess([], 1, "", "HTTP 429: secondary rate limit"),
            subprocess.CompletedProcess([], 0, '{"number": 1}', ""),
        ]
        with mock.patch.object(fallback.subprocess, "run", side_effect=responses) as run:
            with mock.patch.object(fallback.time, "sleep") as sleep:
                value = fallback.gh_api("GET", "repos/o/r/pulls/1")
        self.assertEqual(1, value["number"])
        self.assertEqual(2, run.call_count)
        sleep.assert_called_once_with(1)

    def test_local_journal_round_trip(self):
        with tempfile.TemporaryDirectory() as directory:
            with mock.patch.object(fallback, "cache_root", return_value=pathlib.Path(directory)):
                entry = {
                    "idempotency_key": "run-1",
                    "repository": "o/r",
                    "pull_request": 1,
                    "head_sha": "h",
                    "state": "pending",
                }
                fallback.save_journal(entry)
                self.assertEqual(entry, fallback.load_journal("run-1"))
                mode = fallback.journal_path("run-1").stat().st_mode & 0o777
                self.assertEqual(0o600, mode)


if __name__ == "__main__":
    unittest.main()
