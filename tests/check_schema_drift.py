#!/usr/bin/env python3
"""Fail when the public schema, Go structs, and Python fallback disagree."""

import importlib.util
import json
import pathlib
import re
import sys


ROOT = pathlib.Path(__file__).resolve().parents[1]


def go_json_fields(type_name):
    source = (ROOT / "internal" / "model" / "model.go").read_text(encoding="utf-8")
    match = re.search(rf"type {type_name} struct \{{(.*?)\n\}}", source, re.DOTALL)
    if not match:
        raise RuntimeError(f"Go type {type_name} not found")
    return {
        field
        for field in re.findall(r'json:"([^",]+)', match.group(1))
        if field != "-"
    }


def load_fallback():
    path = ROOT / "skills" / "gh-code-review" / "scripts" / "gh_code_review_fallback.py"
    spec = importlib.util.spec_from_file_location("gh_code_review_fallback", path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def main():
    schema = json.loads((ROOT / "schema" / "v1" / "review.schema.json").read_text())
    schema_manifest = set(schema["properties"])
    schema_comment = set(schema["$defs"]["comment"]["properties"])
    fallback = load_fallback()
    comparisons = [
        ("manifest schema vs Go", schema_manifest, go_json_fields("Manifest")),
        ("comment schema vs Go", schema_comment, go_json_fields("Comment")),
        ("manifest schema vs fallback", schema_manifest, fallback.MANIFEST_FIELDS),
        ("comment schema vs fallback", schema_comment, fallback.COMMENT_FIELDS),
    ]
    failed = False
    for name, expected, actual in comparisons:
        if expected != actual:
            failed = True
            print(f"{name}: missing={sorted(expected - actual)} extra={sorted(actual - expected)}")
    if failed:
        return 1
    print("schema, Go structs, and Python fallback fields are in sync")
    return 0


if __name__ == "__main__":
    sys.exit(main())
