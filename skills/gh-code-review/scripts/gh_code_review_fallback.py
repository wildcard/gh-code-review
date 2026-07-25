#!/usr/bin/env python3
"""Standard-library gh api fallback for gh-code-review inspect/validate/submit."""

import argparse
import hashlib
import json
import os
import re
import subprocess
import sys
import time
from pathlib import Path

SCHEMA_VERSION = "1.0"
EXIT_VALIDATION = 2
EXIT_HEAD_CHANGED = 3
EXIT_AUTH = 4
EXIT_API = 5
EXIT_PARTIAL = 6
EXIT_CONFLICT = 8
EVENTS = {"COMMENT", "APPROVE", "REQUEST_CHANGES"}
MANIFEST_FIELDS = {
    "schema_version", "repository", "pull_request", "expected_head_sha",
    "event", "summary", "idempotency_key", "comments",
}
COMMENT_FIELDS = {
    "client_id", "subject", "path", "body", "line", "side", "start_line",
    "start_side", "replacement", "severity", "category", "confidence",
    "rule_id", "evidence",
}
HUNK_RE = re.compile(r"^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@")


class ToolError(Exception):
    def __init__(self, exit_code, code, message, retryable=False, details=None):
        super().__init__(message)
        self.exit_code = exit_code
        self.code = code
        self.retryable = retryable
        self.details = details


def envelope(ok, operation, repository="", pull_request=0, result=None, error=None):
    value = {"schema_version": SCHEMA_VERSION, "ok": ok, "operation": operation}
    if repository:
        value["repository"] = repository
    if pull_request:
        value["pull_request"] = pull_request
    if result is not None:
        value["result"] = result
    if error is not None:
        value["error"] = {
            "code": error.code,
            "message": str(error),
            "retryable": error.retryable,
        }
        if error.details is not None:
            value["error"]["details"] = error.details
    return value


def emit(value):
    json.dump(value, sys.stdout, indent=2, ensure_ascii=False)
    sys.stdout.write("\n")


def gh_api(method, endpoint, payload=None):
    command = ["gh", "api", "--method", method, endpoint]
    data = None
    if payload is not None:
        command += ["--input", "-"]
        data = json.dumps(payload)
    process = None
    for attempt in range(3):
        process = subprocess.run(command, input=data, text=True, capture_output=True)
        if process.returncode == 0:
            break
        message = process.stderr.strip() or process.stdout.strip() or "gh api failed"
        lower = message.lower()
        rate_limited = "http 429" in lower or "secondary rate limit" in lower or "rate limit exceeded" in lower
        if rate_limited and attempt < 2:
            time.sleep(attempt + 1)
            continue
        if "authentication" in lower or "http 401" in lower or "http 403" in lower:
            raise ToolError(EXIT_AUTH, "AUTHORIZATION", message)
        if "http 409" in lower or "http 422" in lower:
            raise ToolError(EXIT_CONFLICT, "GITHUB_CONFLICT", message)
        raise ToolError(EXIT_API, "GITHUB_API", message, retryable=rate_limited)
    if not process.stdout.strip():
        return None
    return json.loads(process.stdout)


def graphql(query, variables):
    return gh_api("POST", "graphql", {"query": query, "variables": variables})


def paginate(endpoint):
    values = []
    page = 1
    separator = "&" if "?" in endpoint else "?"
    while True:
        batch = gh_api("GET", f"{endpoint}{separator}per_page=100&page={page}")
        if not isinstance(batch, list):
            raise ToolError(EXIT_API, "GITHUB_API", "paginated endpoint did not return an array")
        values.extend(batch)
        if len(batch) < 100:
            return values
        page += 1


def get_check_runs(repository, sha):
    result = {"total_count": 0, "check_runs": []}
    page = 1
    while True:
        batch = gh_api(
            "GET",
            f"repos/{repository}/commits/{sha}/check-runs?per_page=100&page={page}",
        )
        result["total_count"] = batch["total_count"]
        result["check_runs"].extend(batch["check_runs"])
        if len(batch["check_runs"]) < 100:
            return result
        page += 1


def split_repository(repository):
    parts = repository.split("/")
    if len(parts) != 2 or not all(parts):
        raise ToolError(EXIT_VALIDATION, "REPOSITORY", "repository must be owner/name")
    return parts


def get_threads(repository, pull_request):
    owner, name = split_repository(repository)
    query = """
query($owner:String!, $name:String!, $number:Int!, $after:String) {
  repository(owner:$owner, name:$name) {
    pullRequest(number:$number) {
      id
      reviewThreads(first:100, after:$after) {
        pageInfo { hasNextPage endCursor }
        nodes {
          id path line startLine diffSide isResolved isOutdated subjectType
          comments(first:100) {
            pageInfo { hasNextPage endCursor }
            nodes { id databaseId body url author { login } }
          }
        }
      }
    }
  }
}"""
    threads = []
    after = None
    pull_request_id = ""
    while True:
        data = graphql(
            query,
            {"owner": owner, "name": name, "number": pull_request, "after": after},
        )
        pull = data["data"]["repository"]["pullRequest"]
        pull_request_id = pull["id"]
        connection = pull["reviewThreads"]
        for node in connection["nodes"]:
            comments = list(node["comments"]["nodes"])
            page_info = node["comments"]["pageInfo"]
            if page_info["hasNextPage"]:
                comments.extend(get_thread_comments(node["id"], page_info["endCursor"]))
            threads.append(
                {
                    "id": node["id"],
                    "path": node["path"],
                    "line": node.get("line"),
                    "start_line": node.get("startLine"),
                    "side": node.get("diffSide"),
                    "is_resolved": node["isResolved"],
                    "is_outdated": node["isOutdated"],
                    "subject_type": node["subjectType"].lower(),
                    "comments": [
                        {
                            "id": comment["id"],
                            "database_id": comment.get("databaseId"),
                            "body": comment["body"],
                            "url": comment["url"],
                            "author": (comment.get("author") or {}).get("login", ""),
                        }
                        for comment in comments
                    ],
                }
            )
        page_info = connection["pageInfo"]
        if not page_info["hasNextPage"]:
            return threads, pull_request_id
        after = page_info["endCursor"]


def get_thread_comments(thread_id, after):
    query = """
query($id:ID!, $after:String) {
  node(id:$id) {
    ... on PullRequestReviewThread {
      comments(first:100, after:$after) {
        pageInfo { hasNextPage endCursor }
        nodes { id databaseId body url author { login } }
      }
    }
  }
}"""
    comments = []
    while True:
        data = graphql(query, {"id": thread_id, "after": after})
        connection = data["data"]["node"]["comments"]
        comments.extend(connection["nodes"])
        if not connection["pageInfo"]["hasNextPage"]:
            return comments
        after = connection["pageInfo"]["endCursor"]


def parse_files(raw_files, include_patches=False):
    files = []
    locations = {}
    previous = {}
    for raw in raw_files:
        patch = raw.get("patch") or ""
        file_value = {
            "path": raw["filename"],
            "status": raw["status"],
            "additions": raw["additions"],
            "deletions": raw["deletions"],
            "changes": raw["changes"],
            "binary": not patch and raw["changes"] > 0 and raw["additions"] == 0 and raw["deletions"] == 0,
            "truncated": (
                (not patch and raw["changes"] > 0 and not (raw["additions"] == 0 and raw["deletions"] == 0))
                or patch.endswith("\n...")
                or len(patch) >= 65535
            ),
        }
        if raw.get("previous_filename"):
            file_value["previous_path"] = raw["previous_filename"]
            previous[raw["previous_filename"]] = raw["filename"]
        if include_patches:
            file_value["patch"] = patch
        files.append(file_value)
        old_line = new_line = 0
        hunk = -1
        for line in patch.splitlines():
            match = HUNK_RE.match(line)
            if match:
                old_line, new_line = int(match.group(1)), int(match.group(3))
                hunk += 1
                continue
            if hunk < 0 or not line:
                continue
            prefix = line[0]
            if prefix == " ":
                locations[(raw["filename"], old_line, "LEFT")] = hunk
                locations[(raw["filename"], new_line, "RIGHT")] = hunk
                old_line += 1
                new_line += 1
            elif prefix == "-":
                locations[(raw["filename"], old_line, "LEFT")] = hunk
                old_line += 1
            elif prefix == "+":
                locations[(raw["filename"], new_line, "RIGHT")] = hunk
                new_line += 1
    return files, locations, previous


def compact_ranges(files, locations):
    result = {}
    for file_value in files:
        path = file_value["path"]
        result[path] = {"LEFT": [], "RIGHT": []}
        for side in ("LEFT", "RIGHT"):
            lines = sorted(
                line for candidate, line, candidate_side in locations
                if candidate == path and candidate_side == side
            )
            if not lines:
                continue
            start = end = lines[0]
            for line in lines[1:]:
                if line <= end + 1:
                    end = max(end, line)
                else:
                    result[path][side].append([start, end])
                    start = end = line
            result[path][side].append([start, end])
    return result


def suggestion_body(comment):
    body = comment.get("body", "").strip()
    if "replacement" not in comment:
        return body
    if body:
        body += "\n\n"
    replacement = comment["replacement"].rstrip("\n")
    return f"{body}```suggestion\n{replacement}\n```"


def normalize(text):
    return " ".join(text.strip().split())


def cache_root():
    if sys.platform == "darwin":
        return Path.home() / "Library" / "Caches"
    if os.name == "nt":
        return Path(os.environ.get("LOCALAPPDATA", str(Path.home() / "AppData" / "Local")))
    return Path(os.environ.get("XDG_CACHE_HOME", str(Path.home() / ".cache")))


def journal_path(key):
    digest = hashlib.sha256(key.encode()).hexdigest()
    return cache_root() / "gh-code-review" / "transactions" / f"{digest}.json"


def load_journal(key):
    if not key:
        return None
    path = journal_path(key)
    if not path.exists():
        return None
    return json.loads(path.read_text(encoding="utf-8"))


def save_journal(entry):
    key = entry.get("idempotency_key")
    if not key:
        return
    target = journal_path(key)
    target.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
    temporary = target.with_suffix(".tmp")
    temporary.write_text(json.dumps(entry, indent=2) + "\n", encoding="utf-8")
    temporary.chmod(0o600)
    temporary.replace(target)


def derive_event(comments):
    if not comments:
        return "APPROVE"
    if any(comment.get("severity", "").lower() == "blocking" for comment in comments):
        return "REQUEST_CHANGES"
    return "COMMENT"


def static_validate(manifest, derive=False):
    issues = []

    def add(code, message, comment=None):
        issue = {"code": code, "message": message}
        if comment:
            issue["client_id"] = comment.get("client_id", "")
            issue["path"] = comment.get("path", "")
        issues.append(issue)

    for field in sorted(set(manifest) - MANIFEST_FIELDS):
        add("UNKNOWN_FIELD", f"unknown manifest field: {field}")
    if manifest.get("schema_version") != SCHEMA_VERSION:
        add("SCHEMA_VERSION", f'schema_version must be "{SCHEMA_VERSION}"')
    try:
        split_repository(manifest.get("repository", ""))
    except ToolError:
        add("REPOSITORY", "repository must be owner/name")
    if not isinstance(manifest.get("pull_request"), int) or manifest["pull_request"] <= 0:
        add("PULL_REQUEST", "pull_request must be a positive integer")
    if not manifest.get("expected_head_sha"):
        add("EXPECTED_HEAD_SHA", "expected_head_sha is required")
    if not manifest.get("event") and derive:
        manifest["event"] = derive_event(manifest.get("comments", []))
    if manifest.get("event") not in EVENTS:
        add("EVENT", "event must be COMMENT, APPROVE, or REQUEST_CHANGES")
    if manifest.get("event") in {"APPROVE", "REQUEST_CHANGES"} and not manifest.get("summary", "").strip():
        add("SUMMARY", "summary is required for APPROVE and REQUEST_CHANGES")
    if not isinstance(manifest.get("comments"), list):
        add("COMMENTS", "comments must be an array")
        return issues
    client_ids = set()
    fingerprints = set()
    for comment in manifest.get("comments", []):
        for field in sorted(set(comment) - COMMENT_FIELDS):
            add("UNKNOWN_FIELD", f"unknown comment field: {field}", comment)
        client_id = comment.get("client_id", "")
        if not client_id:
            add("CLIENT_ID", "client_id is required", comment)
        elif client_id in client_ids:
            add("DUPLICATE_CLIENT_ID", "client_id must be unique", comment)
        client_ids.add(client_id)
        subject = comment.get("subject")
        if subject not in {"line", "file"}:
            add("SUBJECT", "subject must be line or file", comment)
        path = comment.get("path", "")
        if not path or path.startswith("/") or path.startswith("../"):
            add("PATH", "path must be repository-relative", comment)
        if not comment.get("body", "").strip() and "replacement" not in comment:
            add("BODY", "body or replacement is required", comment)
        if subject == "file":
            if any(comment.get(key) for key in ("line", "side", "start_line", "start_side")):
                add("FILE_LOCATION", "file subjects cannot include line or side fields", comment)
            if "replacement" in comment:
                add("FILE_SUGGESTION", "file subjects cannot contain replacements", comment)
        elif subject == "line":
            side = comment.get("side", "").upper()
            start_side = comment.get("start_side", "").upper()
            if not isinstance(comment.get("line"), int) or comment["line"] <= 0:
                add("LINE", "line subjects require a positive line", comment)
            if side not in {"LEFT", "RIGHT"}:
                add("SIDE", "line subjects require side LEFT or RIGHT", comment)
            if comment.get("start_line"):
                if comment["start_line"] > comment["line"]:
                    add("START_LINE", "start_line must not be greater than line", comment)
                if start_side not in {"LEFT", "RIGHT"}:
                    add("START_SIDE", "range subjects require start_side", comment)
                if start_side != side:
                    add("MIXED_SIDE_RANGE", "ranges cannot cross diff sides", comment)
            elif comment.get("start_side"):
                add("START_SIDE", "start_side requires start_line", comment)
            if "replacement" in comment and (side != "RIGHT" or (start_side and start_side != "RIGHT")):
                add("SUGGESTION_SIDE", "replacements are valid only on RIGHT-side line or range subjects", comment)
        confidence = comment.get("confidence")
        if confidence is not None and (not isinstance(confidence, (int, float)) or confidence < 0 or confidence > 1):
            add("CONFIDENCE", "confidence must be between 0 and 1", comment)
        if comment.get("severity") and comment["severity"] not in {"blocking", "non_blocking", "suggestion", "nit"}:
            add("SEVERITY", "severity must be blocking, non_blocking, suggestion, or nit", comment)
        if comment.get("category") and comment["category"] not in {
            "correctness", "security", "performance", "reliability",
            "maintainability", "testing", "API-design", "documentation",
            "accessibility",
        }:
            add("CATEGORY", "category is not supported by schema v1", comment)
        identity = (
            path,
            subject,
            comment.get("start_line", 0),
            comment.get("start_side", "").upper(),
            comment.get("line", 0),
            comment.get("side", "").upper(),
            normalize(suggestion_body(comment)),
        )
        digest = hashlib.sha256(repr(identity).encode()).hexdigest()
        if digest in fingerprints:
            add("DUPLICATE_FINDING", "same normalized finding appears more than once", comment)
        fingerprints.add(digest)
    return issues


def live_validate(manifest, derive=False, resume_review=0):
    issues = static_validate(manifest, derive)
    repository = manifest.get("repository", "")
    pull_request = manifest.get("pull_request", 0)
    if issues:
        return {"valid": False, "issues": issues, "warnings": []}
    pull = gh_api("GET", f"repos/{repository}/pulls/{pull_request}")
    actual_head = pull["head"]["sha"]
    if actual_head.lower() != manifest["expected_head_sha"].lower():
        raise ToolError(
            EXIT_HEAD_CHANGED,
            "HEAD_CHANGED",
            f'pull request head changed from {manifest["expected_head_sha"]} to {actual_head}',
            True,
            {"expected_head_sha": manifest["expected_head_sha"], "actual_head_sha": actual_head},
        )
    current_user = gh_api("GET", "user")["login"]
    if manifest["event"] == "APPROVE" and pull["user"]["login"].lower() == current_user.lower():
        issues.append({"code": "SELF_APPROVAL", "message": "GitHub does not allow authors to approve their own pull requests"})
    raw_files = paginate(f"repos/{repository}/pulls/{pull_request}/files")
    files, locations, previous = parse_files(raw_files)
    file_map = {file_value["path"]: file_value for file_value in files}
    for comment in manifest.get("comments", []):
        path = comment["path"]
        if path not in file_map:
            if path in previous:
                issues.append({"code": "RENAMED_PATH", "message": f'use renamed path "{previous[path]}"', "client_id": comment["client_id"], "path": path})
            else:
                issues.append({"code": "PATH_NOT_IN_DIFF", "message": "path is not changed by this pull request", "client_id": comment["client_id"], "path": path})
            continue
        if comment["subject"] == "file":
            continue
        file_value = file_map[path]
        if file_value["binary"]:
            issues.append({"code": "BINARY_FILE", "message": "binary files support only file-level comments", "client_id": comment["client_id"], "path": path})
            continue
        end = (path, comment["line"], comment["side"].upper())
        if end not in locations:
            issues.append({"code": "LINE_NOT_IN_DIFF", "message": "line is not commentable in the current diff", "client_id": comment["client_id"], "path": path})
            continue
        if comment.get("start_line"):
            start = (path, comment["start_line"], comment["start_side"].upper())
            if start not in locations:
                issues.append({"code": "START_LINE_NOT_IN_DIFF", "message": "start line is not commentable in the current diff", "client_id": comment["client_id"], "path": path})
            elif locations[start] != locations[end]:
                issues.append({"code": "CROSS_HUNK_RANGE", "message": "a multiline comment cannot cross diff hunks", "client_id": comment["client_id"], "path": path})
            else:
                for line in range(comment["start_line"], comment["line"] + 1):
                    if (path, line, comment["side"].upper()) not in locations:
                        issues.append({"code": "RANGE_GAP", "message": f"{comment['side'].upper()} line {line} is not commentable in this range", "client_id": comment["client_id"], "path": path})
                        break
    reviews = paginate(f"repos/{repository}/pulls/{pull_request}/reviews")
    pending = 0
    for review in reviews:
        if review["state"] == "PENDING" and review["user"]["login"].lower() == current_user.lower():
            pending = review["id"]
            if not resume_review or pending != resume_review:
                issues.append({"code": "PENDING_REVIEW_CONFLICT", "message": f"pending review {pending} already exists"})
    existing_comments = paginate(f"repos/{repository}/pulls/{pull_request}/comments")
    for proposed in manifest.get("comments", []):
        for existing in existing_comments:
            if (
                proposed["path"] == existing["path"]
                and proposed.get("line", 0) == (existing.get("line") or 0)
                and proposed.get("side", "").upper() == (existing.get("side") or "").upper()
                and proposed.get("start_line", 0) == (existing.get("start_line") or 0)
                and normalize(suggestion_body(proposed)) == normalize(existing["body"])
            ):
                issues.append({"code": "DUPLICATE_EXISTING", "message": f'an equivalent review comment exists at {existing["html_url"]}', "client_id": proposed["client_id"], "path": proposed["path"]})
                break
    threads, _ = get_threads(repository, pull_request)
    return {
        "valid": not issues,
        "current_head_sha": actual_head,
        "expected_head_sha": manifest["expected_head_sha"],
        "derived_event": manifest["event"] if derive else None,
        "issues": issues,
        "warnings": [],
        "commentable_ranges": compact_ranges(files, locations),
        "pending_review_id": pending or None,
        "pull_request": pull,
        "files": files,
        "existing_threads": len(threads),
    }


def compact_pull_request(pull):
    return {
        "number": pull["number"],
        "node_id": pull.get("node_id", ""),
        "title": pull.get("title", ""),
        "state": pull.get("state", ""),
        "draft": pull.get("draft", False),
        "html_url": pull.get("html_url", ""),
        "head": {
            "sha": pull["head"]["sha"],
            "ref": pull["head"].get("ref", ""),
        },
        "base": {
            "sha": pull["base"]["sha"],
            "ref": pull["base"].get("ref", ""),
        },
        "user": {"login": (pull.get("user") or {}).get("login", "")},
    }


def compact_review(review):
    return {
        "id": review["id"],
        "node_id": review.get("node_id", ""),
        "state": review.get("state", ""),
        "body": review.get("body", ""),
        "commit_id": review.get("commit_id", ""),
        "html_url": review.get("html_url", ""),
        "submitted_at": review.get("submitted_at", ""),
        "user": {"login": (review.get("user") or {}).get("login", "")},
    }


def compact_review_comment(comment):
    return {
        "id": comment["id"],
        "node_id": comment.get("node_id", ""),
        "pull_request_review_id": comment.get("pull_request_review_id", 0),
        "path": comment.get("path", ""),
        "body": comment.get("body", ""),
        "commit_id": comment.get("commit_id", ""),
        "subject_type": comment.get("subject_type", ""),
        "line": comment.get("line"),
        "side": comment.get("side", ""),
        "start_line": comment.get("start_line"),
        "start_side": comment.get("start_side", ""),
        "in_reply_to_id": comment.get("in_reply_to_id"),
        "html_url": comment.get("html_url", ""),
        "position": comment.get("position"),
        "user": {"login": (comment.get("user") or {}).get("login", "")},
    }


def compact_checks(checks):
    return {
        "total_count": checks.get("total_count", 0),
        "check_runs": [
            {
                "id": check.get("id", 0),
                "name": check.get("name", ""),
                "status": check.get("status", ""),
                "conclusion": check.get("conclusion", ""),
                "html_url": check.get("html_url", ""),
            }
            for check in checks.get("check_runs", [])
        ],
    }


def compact_status(status):
    return {
        "state": status.get("state", ""),
        "statuses": [
            {
                "context": value.get("context", ""),
                "state": value.get("state", ""),
                "target_url": value.get("target_url", ""),
            }
            for value in status.get("statuses", [])
        ],
    }


def inspect(args):
    repository = args.repo
    pull_request = args.pull_request
    pull = gh_api("GET", f"repos/{repository}/pulls/{pull_request}")
    raw_files = paginate(f"repos/{repository}/pulls/{pull_request}/files")
    files, locations, _ = parse_files(raw_files, args.include_patches)
    reviews = paginate(f"repos/{repository}/pulls/{pull_request}/reviews")
    comments = paginate(f"repos/{repository}/pulls/{pull_request}/comments")
    threads, _ = get_threads(repository, pull_request)
    checks = get_check_runs(repository, pull["head"]["sha"])
    status = gh_api("GET", f"repos/{repository}/commits/{pull['head']['sha']}/status")
    if not args.include_bodies:
        for value in reviews + comments:
            value["body"] = ""
        for thread in threads:
            for comment in thread["comments"]:
                comment["body"] = ""
    return {
        "pull_request": compact_pull_request(pull),
        "files": files,
        "commentable_ranges": compact_ranges(files, locations),
        "reviews": [compact_review(value) for value in reviews],
        "review_comments": [compact_review_comment(value) for value in comments],
        "threads": threads,
        "checks": compact_checks(checks),
        "commit_status": compact_status(status),
    }


def load_manifest(path):
    try:
        content = sys.stdin.read() if path == "-" else Path(path).read_text(encoding="utf-8")
        return json.loads(content)
    except (OSError, json.JSONDecodeError) as error:
        raise ToolError(EXIT_VALIDATION, "MANIFEST", f"read JSON manifest: {error}")


def api_comment(comment):
    value = {"path": comment["path"], "body": suggestion_body(comment)}
    if comment["subject"] == "file":
        value["subject_type"] = "file"
    else:
        value.update({"line": comment["line"], "side": comment["side"].upper()})
        if comment.get("start_line"):
            value.update({"start_line": comment["start_line"], "start_side": comment["start_side"].upper()})
    return value


def add_graphql_thread(review_node_id, comment):
    query = """
mutation($input:AddPullRequestReviewThreadInput!) {
  addPullRequestReviewThread(input:$input) { thread { id } }
}"""
    value = {
        "pullRequestReviewId": review_node_id,
        "body": suggestion_body(comment),
        "path": comment["path"],
        "subjectType": comment["subject"].upper(),
    }
    if comment["subject"] == "line":
        value.update({"line": comment["line"], "side": comment["side"].upper()})
        if comment.get("start_line"):
            value.update({"startLine": comment["start_line"], "startSide": comment["start_side"].upper()})
    return graphql(query, {"input": value})["data"]["addPullRequestReviewThread"]["thread"]["id"]


def submit(args, manifest):
    if not manifest.get("event") and args.derive_event:
        manifest["event"] = derive_event(manifest.get("comments", []))
    if manifest.get("event") != "COMMENT" and not args.confirm_decision:
        raise ToolError(EXIT_VALIDATION, "DECISION_CONFIRMATION_REQUIRED", f'{manifest.get("event")} requires --confirm-decision')
    existing_journal = load_journal(manifest.get("idempotency_key", ""))
    if existing_journal:
        same_target = (
            existing_journal.get("repository") == manifest.get("repository")
            and existing_journal.get("pull_request") == manifest.get("pull_request")
            and existing_journal.get("head_sha", "").lower() == manifest.get("expected_head_sha", "").lower()
        )
        if not same_target:
            raise ToolError(EXIT_CONFLICT, "IDEMPOTENCY_CONFLICT", "idempotency key was already used for a different review transaction")
        if existing_journal.get("state") == "submitted":
            repository, pull_request = manifest["repository"], manifest["pull_request"]
            review = gh_api("GET", f"repos/{repository}/pulls/{pull_request}/reviews/{existing_journal['review_id']}")
            comments = paginate(f"repos/{repository}/pulls/{pull_request}/comments")
            for proposed in manifest.get("comments", []):
                matched = any(
                    proposed["path"] == current["path"]
                    and proposed.get("line", 0) == (current.get("line") or 0)
                    and proposed.get("side", "").upper() == (current.get("side") or "").upper()
                    and proposed.get("start_line", 0) == (current.get("start_line") or 0)
                    and normalize(suggestion_body(proposed)) == normalize(current["body"])
                    for current in comments
                )
                if not matched:
                    raise ToolError(
                        EXIT_CONFLICT,
                        "IDEMPOTENCY_RECONCILIATION",
                        "the submitted review no longer contains every normalized finding",
                        details={"review_id": review["id"], "client_id": proposed["client_id"]},
                    )
            return {
                "dry_run": False,
                "event": manifest["event"],
                "head_sha": manifest["expected_head_sha"],
                "review_id": review["id"],
                "review_node_id": existing_journal.get("review_node_id", ""),
                "review_url": existing_journal.get("review_url", ""),
                "comments_created": len(manifest.get("comments", [])),
                "transport": "journal",
                "idempotent_replay": True,
                "cleanup_attempted": False,
                "cleanup_succeeded": False,
            }
        if not args.resume_review and existing_journal.get("review_id"):
            args.resume_review = existing_journal["review_id"]

    validation = live_validate(manifest, args.derive_event, args.resume_review)
    if not validation["valid"]:
        conflicts = {"PENDING_REVIEW_CONFLICT", "DUPLICATE_EXISTING", "DUPLICATE_FINDING", "DUPLICATE_CLIENT_ID"}
        exit_code = EXIT_CONFLICT if any(issue["code"] in conflicts for issue in validation["issues"]) else EXIT_VALIDATION
        raise ToolError(exit_code, "VALIDATION_FAILED", "review manifest failed validation", details=validation["issues"])
    comments = manifest.get("comments", [])
    line_only = not args.resume_review and all(comment["subject"] == "line" for comment in comments)
    transport = "rest-batch" if line_only else "graphql-pending"
    receipt = {
        "dry_run": args.dry_run,
        "event": manifest["event"],
        "head_sha": manifest["expected_head_sha"],
        "comments_created": len(comments),
        "transport": transport,
        "idempotent_replay": False,
        "cleanup_attempted": False,
        "cleanup_succeeded": False,
    }
    if args.dry_run:
        return receipt
    repository, pull_request = manifest["repository"], manifest["pull_request"]
    pending = None
    if args.resume_review:
        reviews = paginate(f"repos/{repository}/pulls/{pull_request}/reviews")
        pending = next((review for review in reviews if review["id"] == args.resume_review and review["state"] == "PENDING"), None)
        if pending is None:
            raise ToolError(EXIT_CONFLICT, "PENDING_REVIEW_NOT_FOUND", f"pending review {args.resume_review} was not found")
    elif line_only:
        pending = gh_api(
            "POST",
            f"repos/{repository}/pulls/{pull_request}/reviews",
            {
                "commit_id": manifest["expected_head_sha"],
                "comments": [
                    {key: value for key, value in api_comment(comment).items() if key != "subject_type"}
                    for comment in comments
                ],
            },
        )
    else:
        pending = gh_api(
            "POST",
            f"repos/{repository}/pulls/{pull_request}/reviews",
            {"commit_id": manifest["expected_head_sha"]},
        )
    receipt.update({"review_id": pending["id"], "review_node_id": pending["node_id"]})
    journal = {
        "schema_version": SCHEMA_VERSION,
        "idempotency_key": manifest.get("idempotency_key", ""),
        "repository": repository,
        "pull_request": pull_request,
        "head_sha": manifest["expected_head_sha"],
        "review_id": pending["id"],
        "review_node_id": pending["node_id"],
        "state": "pending",
    }
    save_journal(journal)
    try:
        if not line_only:
            for comment in comments:
                add_graphql_thread(pending["node_id"], comment)
        current = gh_api("GET", f"repos/{repository}/pulls/{pull_request}")
        if current["head"]["sha"].lower() != manifest["expected_head_sha"].lower():
            raise ToolError(
                EXIT_HEAD_CHANGED,
                "HEAD_CHANGED",
                "pull request head changed before submission",
                True,
                {"expected_head_sha": manifest["expected_head_sha"], "actual_head_sha": current["head"]["sha"]},
            )
        submitted = gh_api(
            "POST",
            f"repos/{repository}/pulls/{pull_request}/reviews/{pending['id']}/events",
            {"event": manifest["event"], "body": manifest.get("summary", "")},
        )
        receipt["review_url"] = submitted.get("html_url", "")
        journal["state"] = "submitted"
        journal["review_url"] = receipt["review_url"]
        save_journal(journal)
        return receipt
    except ToolError as cause:
        receipt["cleanup_attempted"] = True
        try:
            gh_api("DELETE", f"repos/{repository}/pulls/{pull_request}/reviews/{pending['id']}")
            receipt["cleanup_succeeded"] = True
            journal["state"] = "cleaned"
            journal["error"] = str(cause)
            save_journal(journal)
        except ToolError as cleanup:
            journal["state"] = "cleanup_failed"
            journal["error"] = f"{cause}; cleanup: {cleanup}"
            save_journal(journal)
            raise ToolError(
                EXIT_PARTIAL,
                "PARTIAL_TRANSACTION",
                "review submission failed and the pending review could not be deleted",
                True,
                {"review_id": pending["id"], "cause": str(cause), "cleanup_error": str(cleanup)},
            )
        raise


def parser():
    root = argparse.ArgumentParser(description=__doc__)
    commands = root.add_subparsers(dest="command", required=True)
    inspect_parser = commands.add_parser("inspect")
    inspect_parser.add_argument("pull_request", type=int)
    inspect_parser.add_argument("-R", "--repo", required=True)
    inspect_parser.add_argument("--include-patches", action="store_true")
    inspect_parser.add_argument("--include-bodies", action="store_true")
    validate_parser = commands.add_parser("validate")
    validate_parser.add_argument("--input", "-i", required=True)
    validate_parser.add_argument("--derive-event", action="store_true")
    validate_parser.add_argument("--resume-review", type=int, default=0)
    submit_parser = commands.add_parser("submit")
    submit_parser.add_argument("--input", "-i", required=True)
    submit_parser.add_argument("--derive-event", action="store_true")
    submit_parser.add_argument("--confirm-decision", action="store_true")
    submit_parser.add_argument("--resume-review", type=int, default=0)
    submit_parser.add_argument("--dry-run", action="store_true")
    return root


def main():
    args = parser().parse_args()
    operation = args.command
    repository = getattr(args, "repo", "")
    pull_request = getattr(args, "pull_request", 0)
    try:
        if operation == "inspect":
            result = inspect(args)
        else:
            manifest = load_manifest(args.input)
            repository = manifest.get("repository", "")
            pull_request = manifest.get("pull_request", 0)
            if operation == "validate":
                result = live_validate(manifest, args.derive_event, args.resume_review)
                if not result["valid"]:
                    conflicts = {"PENDING_REVIEW_CONFLICT", "DUPLICATE_EXISTING", "DUPLICATE_FINDING", "DUPLICATE_CLIENT_ID"}
                    code = EXIT_CONFLICT if any(issue["code"] in conflicts for issue in result["issues"]) else EXIT_VALIDATION
                    raise ToolError(code, "VALIDATION_FAILED", "review manifest failed validation", details=result["issues"])
            else:
                result = submit(args, manifest)
        emit(envelope(True, operation, repository, pull_request, result=result))
        return 0
    except ToolError as error:
        emit(envelope(False, operation, repository, pull_request, error=error))
        return error.exit_code


if __name__ == "__main__":
    sys.exit(main())
