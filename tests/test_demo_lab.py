import importlib.util
import json
import subprocess
import sys
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
SCENARIOS_PATH = ROOT / "demo" / "scenarios.json"
EVIDENCE_PATH = ROOT / "demo" / "evidence.json"


def load_demo_lab():
    spec = importlib.util.spec_from_file_location(
        "demo_lab", ROOT / "scripts" / "demo_lab.py"
    )
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


class DemoCatalogTests(unittest.TestCase):
    def setUp(self):
        self.catalog = json.loads(SCENARIOS_PATH.read_text(encoding="utf-8"))

    def test_catalog_covers_every_required_feature(self):
        required = set(self.catalog["required_features"])
        covered = {
            feature
            for scenario in self.catalog["scenarios"]
            for feature in scenario["features"]
        }
        self.assertEqual(required, covered)
        self.assertGreaterEqual(len(required), 30)

    def test_scenarios_have_unique_paths_and_existing_assets(self):
        ids = set()
        branches = set()
        targets = set()
        for scenario in self.catalog["scenarios"]:
            self.assertNotIn(scenario["id"], ids)
            self.assertNotIn(scenario["branch"], branches)
            self.assertNotIn(scenario["target_path"], targets)
            ids.add(scenario["id"])
            branches.add(scenario["branch"])
            targets.add(scenario["target_path"])
            self.assertTrue((ROOT / scenario["proposed_path"]).is_file())
            self.assertTrue((ROOT / scenario["expected_path"]).is_file())
            self.assertTrue((ROOT / scenario["guide"]).is_file())
            if "agent" in scenario:
                self.assertIn("{pr_url}", scenario["agent"]["prompt"])

    def test_manifest_templates_are_valid_after_rendering_tokens(self):
        for path in sorted((ROOT / "demo" / "manifests").glob("*.json")):
            manifest = json.loads(path.read_text(encoding="utf-8"))
            manifest["repository"] = "owner/repo"
            manifest["pull_request"] = 7
            manifest["expected_head_sha"] = "abc123"
            if manifest.get("idempotency_key"):
                manifest["idempotency_key"] = manifest[
                    "idempotency_key"
                ].replace("__HEAD_SHA__", "abc123")
            self.assertEqual("1.0", manifest["schema_version"])
            self.assertIn(manifest["event"], {"COMMENT", "APPROVE", "REQUEST_CHANGES"})
            client_ids = [comment["client_id"] for comment in manifest["comments"]]
            self.assertEqual(len(client_ids), len(set(client_ids)))

    def test_catalog_validator_is_clean(self):
        demo_lab = load_demo_lab()
        self.assertEqual([], demo_lab.validate_catalog(self.catalog))

    def test_write_commands_require_confirmation_before_network(self):
        for command in ("create", "advance"):
            process = subprocess.run(
                [
                    sys.executable,
                    str(ROOT / "scripts" / "demo_lab.py"),
                    command,
                    "--scenario",
                    (
                        "core-transaction"
                        if command == "create"
                        else "guardrails-fallback"
                    ),
                ],
                text=True,
                capture_output=True,
                check=False,
            )
            self.assertEqual(2, process.returncode)
            self.assertIn("rerun with --confirm", process.stderr)

    def test_released_evidence_is_complete(self):
        evidence = json.loads(EVIDENCE_PATH.read_text(encoding="utf-8"))
        if evidence["release"] == "unreleased":
            self.skipTest("live evidence has not been recorded yet")
        expected = {scenario["id"] for scenario in self.catalog["scenarios"]}
        self.assertEqual(expected, set(evidence["scenarios"]))
        for scenario_id, value in evidence["scenarios"].items():
            self.assertGreater(value["pull_request"], 0, scenario_id)
            self.assertTrue(value["pull_url"].startswith("https://github.com/"))
            self.assertEqual(0, value["flat_pr_comments"])
            self.assertTrue(value["commands"], scenario_id)


if __name__ == "__main__":
    unittest.main()
