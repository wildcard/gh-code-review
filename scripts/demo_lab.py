#!/usr/bin/env python3
"""Create, render, and verify the public gh-code-review demo lab."""

from __future__ import annotations

import argparse
import base64
import json
import subprocess
import sys
import urllib.parse
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[1]
SCENARIOS_PATH = ROOT / "demo" / "scenarios.json"
EVIDENCE_PATH = ROOT / "demo" / "evidence.json"


class DemoError(RuntimeError):
    """A user-facing demo-lab failure."""


def load_json(path: Path) -> dict[str, Any]:
    return json.loads(path.read_text(encoding="utf-8"))


def emit(value: Any) -> None:
    json.dump(value, sys.stdout, indent=2, sort_keys=True)
    sys.stdout.write("\n")


def run(command: list[str], *, payload: dict[str, Any] | None = None) -> Any:
    process = subprocess.run(
        command,
        input=None if payload is None else json.dumps(payload),
        text=True,
        capture_output=True,
        check=False,
    )
    if process.returncode != 0:
        detail = process.stderr.strip() or process.stdout.strip()
        raise DemoError(f"{' '.join(command)} failed: {detail}")
    if not process.stdout.strip():
        return None
    try:
        return json.loads(process.stdout)
    except json.JSONDecodeError:
        return process.stdout.strip()


def gh_api(method: str, endpoint: str, payload: dict[str, Any] | None = None) -> Any:
    command = ["gh", "api", "--method", method, endpoint]
    if payload is not None:
        command.extend(["--input", "-"])
    return run(command, payload=payload)


def catalog() -> dict[str, Any]:
    value = load_json(SCENARIOS_PATH)
    if value.get("schema_version") != "1.0":
        raise DemoError("demo/scenarios.json must use schema_version 1.0")
    return value


def find_scenario(scenario_id: str) -> dict[str, Any]:
    for scenario in catalog()["scenarios"]:
        if scenario["id"] == scenario_id:
            return scenario
    raise DemoError(f"unknown scenario: {scenario_id}")


def require_confirm(value: bool, operation: str) -> None:
    if not value:
        raise DemoError(f"{operation} writes to GitHub; rerun with --confirm")


def repository_owner(repository: str) -> str:
    return repository.split("/", 1)[0]


def command_list(args: argparse.Namespace) -> None:
    value = catalog()
    if args.json:
        emit(value)
        return
    for scenario in value["scenarios"]:
        features = ", ".join(scenario["features"])
        print(f"{scenario['id']}: {scenario['title']}")
        print(f"  {features}")


def command_create(args: argparse.Namespace) -> None:
    require_confirm(args.confirm, "create")
    value = catalog()
    repository = value["repository"]
    scenario = find_scenario(args.scenario)
    branch = scenario["branch"]
    target_path = scenario["target_path"]
    proposed_path = ROOT / scenario["proposed_path"]
    if not proposed_path.is_file():
        raise DemoError(f"missing proposed fixture: {proposed_path}")

    base = gh_api("GET", f"repos/{repository}/git/ref/heads/main")
    base_sha = base["object"]["sha"]
    branch_ref = f"refs/heads/{branch}"
    branch_created = False
    try:
        gh_api(
            "POST",
            f"repos/{repository}/git/refs",
            {"ref": branch_ref, "sha": base_sha},
        )
        branch_created = True
        content = base64.b64encode(proposed_path.read_bytes()).decode("ascii")
        gh_api(
            "PUT",
            f"repos/{repository}/contents/{target_path}",
            {
                "message": f"demo: {scenario['title'].lower()}",
                "content": content,
                "branch": branch,
            },
        )
        pull = gh_api(
            "POST",
            f"repos/{repository}/pulls",
            {
                "title": f"demo: {scenario['title'].lower()}",
                "head": branch,
                "base": "main",
                "body": (
                    "Sanitized live fixture for the gh-code-review user guide.\n\n"
                    f"Scenario: `{scenario['id']}`\n\n"
                    "This pull request is intentionally not for merging."
                ),
            },
        )
        try:
            gh_api(
                "POST",
                f"repos/{repository}/issues/{pull['number']}/labels",
                {"labels": ["demo"]},
            )
        except DemoError:
            pass
    except Exception:
        if branch_created:
            try:
                encoded = urllib.parse.quote(branch, safe="/")
                gh_api("DELETE", f"repos/{repository}/git/refs/heads/{encoded}")
            except DemoError:
                pass
        raise

    emit(
        {
            "ok": True,
            "operation": "demo.create",
            "scenario": scenario["id"],
            "pull_request": pull["number"],
            "url": pull["html_url"],
            "head_sha": pull["head"]["sha"],
            "branch": branch,
        }
    )


def open_pull_for_scenario(repository: str, scenario: dict[str, Any]) -> dict[str, Any]:
    owner = repository_owner(repository)
    query = urllib.parse.urlencode(
        {"state": "open", "head": f"{owner}:{scenario['branch']}"}
    )
    pulls = gh_api("GET", f"repos/{repository}/pulls?{query}")
    if len(pulls) != 1:
        raise DemoError(
            f"expected one open PR for {scenario['id']}, found {len(pulls)}"
        )
    return pulls[0]


def command_advance(args: argparse.Namespace) -> None:
    require_confirm(args.confirm, "advance")
    value = catalog()
    repository = value["repository"]
    scenario = find_scenario(args.scenario)
    followup_path = scenario.get("followup_path")
    if not followup_path:
        raise DemoError(f"scenario {scenario['id']} has no followup_path")
    pull = open_pull_for_scenario(repository, scenario)
    branch = scenario["branch"]
    target_path = scenario["target_path"]
    encoded_ref = urllib.parse.quote(branch, safe="")
    current = gh_api(
        "GET",
        f"repos/{repository}/contents/{target_path}?ref={encoded_ref}",
    )
    content = base64.b64encode((ROOT / followup_path).read_bytes()).decode("ascii")
    updated = gh_api(
        "PUT",
        f"repos/{repository}/contents/{target_path}",
        {
            "message": f"demo: advance {scenario['id']} head",
            "content": content,
            "branch": branch,
            "sha": current["sha"],
        },
    )
    emit(
        {
            "ok": True,
            "operation": "demo.advance",
            "scenario": scenario["id"],
            "pull_request": pull["number"],
            "url": pull["html_url"],
            "old_head_sha": pull["head"]["sha"],
            "new_head_sha": updated["commit"]["sha"],
        }
    )


def command_render(args: argparse.Namespace) -> None:
    value = catalog()
    repository = value["repository"]
    scenario = find_scenario(args.scenario)
    template_path = ROOT / "demo" / "manifests" / f"{scenario['id']}.json"
    if not template_path.is_file():
        raise DemoError(f"scenario {scenario['id']} has no manifest template")
    pull = gh_api("GET", f"repos/{repository}/pulls/{args.pull_request}")
    manifest = load_json(template_path)
    manifest["repository"] = repository
    manifest["pull_request"] = args.pull_request
    manifest["expected_head_sha"] = pull["head"]["sha"]
    if manifest.get("idempotency_key"):
        manifest["idempotency_key"] = manifest["idempotency_key"].replace(
            "__HEAD_SHA__", pull["head"]["sha"]
        )
    output_path = Path(args.output).expanduser().resolve()
    output_path.parent.mkdir(parents=True, exist_ok=True)
    output_path.write_text(json.dumps(manifest, indent=2) + "\n", encoding="utf-8")
    emit(
        {
            "ok": True,
            "operation": "demo.render",
            "scenario": scenario["id"],
            "pull_request": args.pull_request,
            "head_sha": pull["head"]["sha"],
            "output": str(output_path),
        }
    )


def validate_catalog(value: dict[str, Any]) -> list[str]:
    issues: list[str] = []
    scenarios = value.get("scenarios", [])
    required = set(value.get("required_features", []))
    covered: set[str] = set()
    ids: set[str] = set()
    branches: set[str] = set()
    targets: set[str] = set()
    for scenario in scenarios:
        scenario_id = scenario.get("id", "")
        if not scenario_id or scenario_id in ids:
            issues.append(f"duplicate or missing scenario id: {scenario_id!r}")
        ids.add(scenario_id)
        branch = scenario.get("branch", "")
        if not branch or branch in branches:
            issues.append(f"duplicate or missing branch for {scenario_id}")
        branches.add(branch)
        target = scenario.get("target_path", "")
        if not target or target in targets:
            issues.append(f"duplicate or missing target_path for {scenario_id}")
        targets.add(target)
        for field in ("proposed_path", "expected_path", "guide"):
            path = ROOT / scenario.get(field, "")
            if not path.is_file():
                issues.append(f"{scenario_id}: missing {field} {path}")
        features = set(scenario.get("features", []))
        covered.update(features)
        if not features:
            issues.append(f"{scenario_id}: features cannot be empty")
        agent = scenario.get("agent")
        if agent and "{pr_url}" not in agent.get("prompt", ""):
            issues.append(f"{scenario_id}: agent prompt must contain {{pr_url}}")
    missing = sorted(required - covered)
    extra = sorted(covered - required)
    if missing:
        issues.append(f"required features not covered: {', '.join(missing)}")
    if extra:
        issues.append(f"features missing from required_features: {', '.join(extra)}")
    return issues


def verify_live_scenario(
    repository: str, scenario_id: str, evidence: dict[str, Any]
) -> list[str]:
    issues: list[str] = []
    number = evidence["pull_request"]
    pull = gh_api("GET", f"repos/{repository}/pulls/{number}")
    if pull["html_url"] != evidence["pull_url"]:
        issues.append(f"{scenario_id}: pull URL changed")
    expected_state = evidence.get("pull_state")
    if expected_state and pull["state"].upper() != expected_state:
        issues.append(
            f"{scenario_id}: expected PR state {expected_state}, got {pull['state']}"
        )
    flat_comments = gh_api("GET", f"repos/{repository}/issues/{number}/comments")
    if len(flat_comments) != evidence.get("flat_pr_comments", 0):
        issues.append(
            f"{scenario_id}: expected {evidence.get('flat_pr_comments', 0)} "
            f"flat PR comments, got {len(flat_comments)}"
        )
    reviews = gh_api("GET", f"repos/{repository}/pulls/{number}/reviews")
    review_by_id = {review["id"]: review for review in reviews}
    for expected in evidence.get("reviews", []):
        review = review_by_id.get(expected["id"])
        if not review:
            issues.append(f"{scenario_id}: missing review {expected['id']}")
            continue
        if review["state"] != expected["state"]:
            issues.append(
                f"{scenario_id}: review {expected['id']} expected "
                f"{expected['state']}, got {review['state']}"
            )
    comments = gh_api("GET", f"repos/{repository}/pulls/{number}/comments")
    comment_ids = {comment["id"] for comment in comments}
    for comment_id in evidence.get("review_comment_ids", []):
        if comment_id not in comment_ids:
            issues.append(f"{scenario_id}: missing review comment {comment_id}")
    return issues


def command_verify(args: argparse.Namespace) -> None:
    value = catalog()
    issues = validate_catalog(value)
    evidence = load_json(EVIDENCE_PATH)
    recorded = evidence.get("scenarios", {})
    expected_ids = {scenario["id"] for scenario in value["scenarios"]}
    recorded_ids = set(recorded)
    if not args.allow_incomplete and recorded_ids != expected_ids:
        missing = sorted(expected_ids - recorded_ids)
        extra = sorted(recorded_ids - expected_ids)
        if missing:
            issues.append(f"evidence missing scenarios: {', '.join(missing)}")
        if extra:
            issues.append(f"evidence contains unknown scenarios: {', '.join(extra)}")
    if args.live:
        for scenario_id, scenario_evidence in recorded.items():
            issues.extend(
                verify_live_scenario(value["repository"], scenario_id, scenario_evidence)
            )
    if issues:
        emit({"ok": False, "operation": "demo.verify", "issues": issues})
        raise DemoError(f"demo verification failed with {len(issues)} issue(s)")
    emit(
        {
            "ok": True,
            "operation": "demo.verify",
            "live": args.live,
            "scenario_count": len(value["scenarios"]),
            "evidence_count": len(recorded),
            "feature_count": len(value["required_features"]),
        }
    )


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description=__doc__)
    subparsers = parser.add_subparsers(dest="command", required=True)

    list_parser = subparsers.add_parser("list", help="List demo scenarios")
    list_parser.add_argument("--json", action="store_true")
    list_parser.set_defaults(handler=command_list)

    create_parser = subparsers.add_parser(
        "create", help="Create one live dummy pull request"
    )
    create_parser.add_argument("--scenario", required=True)
    create_parser.add_argument("--confirm", action="store_true")
    create_parser.set_defaults(handler=command_create)

    advance_parser = subparsers.add_parser(
        "advance", help="Advance a scenario PR head for stale-head testing"
    )
    advance_parser.add_argument("--scenario", required=True)
    advance_parser.add_argument("--confirm", action="store_true")
    advance_parser.set_defaults(handler=command_advance)

    render_parser = subparsers.add_parser(
        "render", help="Render a live manifest from a scenario template"
    )
    render_parser.add_argument("--scenario", required=True)
    render_parser.add_argument("--pull-request", type=int, required=True)
    render_parser.add_argument("--output", required=True)
    render_parser.set_defaults(handler=command_render)

    verify_parser = subparsers.add_parser(
        "verify", help="Verify catalog structure and optional live evidence"
    )
    verify_parser.add_argument("--live", action="store_true")
    verify_parser.add_argument("--allow-incomplete", action="store_true")
    verify_parser.set_defaults(handler=command_verify)
    return parser


def main() -> int:
    parser = build_parser()
    args = parser.parse_args()
    try:
        args.handler(args)
        return 0
    except DemoError as error:
        print(f"demo_lab: {error}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
