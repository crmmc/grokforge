#!/usr/bin/env python3
"""Validate Next.js build artifacts against frontend performance budgets."""

from __future__ import annotations

import argparse
import json
import sys
from dataclasses import dataclass
from pathlib import Path
from typing import Any


@dataclass(frozen=True)
class Threshold:
    warning: float | None = None
    critical: float | None = None


@dataclass(frozen=True)
class CheckResult:
    name: str
    actual: float
    warning: float | None
    critical: float | None
    status: str


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Validate GrokForge frontend build output against budget thresholds."
    )
    parser.add_argument(
        "--budget",
        default="performance-budgets.json",
        help="Path to the frontend performance budget JSON file.",
    )
    parser.add_argument(
        "--build-dir",
        default="web/.next",
        help="Path to the Next.js build output directory.",
    )
    parser.add_argument(
        "--output",
        help="Optional path to write the collected metrics/report as JSON.",
    )
    parser.add_argument(
        "--summary",
        action="store_true",
        help="Print a human-readable summary.",
    )
    parser.add_argument(
        "--fail-on-violation",
        action="store_true",
        help="Exit with status 1 when any warning or critical violation is found.",
    )
    return parser.parse_args()


def load_json(path: Path) -> dict[str, Any]:
    if not path.exists():
        raise FileNotFoundError(f"File not found: {path}")
    return json.loads(path.read_text())


def size_in_kb(path: Path) -> float:
    return round(path.stat().st_size / 1024, 1)


def collect_metrics(build_dir: Path) -> dict[str, Any]:
    app_manifest = load_json(build_dir / "app-build-manifest.json")["pages"]
    route_manifest = load_json(build_dir / "app-path-routes-manifest.json")
    build_manifest = load_json(build_dir / "build-manifest.json")

    routes: dict[str, dict[str, float]] = {}
    for internal_route, public_route in sorted(route_manifest.items(), key=lambda item: item[1]):
        assets = app_manifest[internal_route]
        total_js_kb = round(
            sum(size_in_kb(build_dir / asset) for asset in assets if asset.endswith(".js")),
            1,
        )
        total_css_kb = round(
            sum(size_in_kb(build_dir / asset) for asset in assets if asset.endswith(".css")),
            1,
        )
        routes[public_route] = {
            "total_js_kb": total_js_kb,
            "total_css_kb": total_css_kb,
        }

    root_main_files = build_manifest.get("rootMainFiles", [])
    root_main_js_kb = round(
        sum(size_in_kb(build_dir / asset) for asset in root_main_files if asset.endswith(".js")),
        1,
    )

    css_total_kb = round(
        sum(size_in_kb(path) for path in (build_dir / "static" / "css").glob("*.css")),
        1,
    )

    vendor_chunks = []
    for chunk_path in (build_dir / "static" / "chunks").glob("*.js"):
        if chunk_path.name.startswith(("framework-", "main-", "webpack-", "polyfills-")):
            continue
        if f"static/chunks/{chunk_path.name}" in root_main_files:
            continue
        vendor_chunks.append(size_in_kb(chunk_path))

    largest_lazy_chunk_js_kb = round(max(vendor_chunks, default=0.0), 1)

    return {
        "root_main_js_kb": root_main_js_kb,
        "largest_lazy_chunk_js_kb": largest_lazy_chunk_js_kb,
        "css_total_kb": css_total_kb,
        "routes": routes,
    }


def parse_threshold(raw: Any) -> Threshold:
    if raw is None:
        return Threshold()
    if isinstance(raw, (int, float)):
        value = float(raw)
        return Threshold(warning=value, critical=value)
    if not isinstance(raw, dict):
        raise TypeError(f"Unsupported threshold config: {raw!r}")
    warning = raw.get("warning")
    critical = raw.get("critical")
    return Threshold(
        warning=float(warning) if warning is not None else None,
        critical=float(critical) if critical is not None else None,
    )


def evaluate_check(name: str, actual: float, threshold: Threshold) -> CheckResult:
    if threshold.critical is not None and actual > threshold.critical:
        return CheckResult(name, actual, threshold.warning, threshold.critical, "critical")
    if threshold.warning is not None and actual > threshold.warning:
        return CheckResult(name, actual, threshold.warning, threshold.critical, "warning")
    return CheckResult(name, actual, threshold.warning, threshold.critical, "pass")


def validate_metrics(budget: dict[str, Any], metrics: dict[str, Any]) -> dict[str, Any]:
    budget_metrics = budget.get("budgets", {})
    checks: list[CheckResult] = []
    notices: list[str] = []

    for metric_name in ("root_main_js_kb", "largest_lazy_chunk_js_kb", "css_total_kb"):
        if metric_name not in budget_metrics:
            notices.append(f"Missing top-level budget for {metric_name}")
            continue
        checks.append(
            evaluate_check(metric_name, float(metrics[metric_name]), parse_threshold(budget_metrics[metric_name]))
        )

    route_budgets = budget_metrics.get("routes", {})
    for route, route_budget in route_budgets.items():
        route_metrics = metrics["routes"].get(route)
        if route_metrics is None:
            notices.append(f"Missing route metrics for {route}")
            continue
        for metric_name, threshold_raw in route_budget.items():
            if metric_name not in route_metrics:
                notices.append(f"Missing route metric {route}:{metric_name}")
                continue
            checks.append(
                evaluate_check(
                    f"{route}:{metric_name}",
                    float(route_metrics[metric_name]),
                    parse_threshold(threshold_raw),
                )
            )

    summary = {
        "passed": sum(1 for check in checks if check.status == "pass"),
        "warnings": sum(1 for check in checks if check.status == "warning"),
        "critical": sum(1 for check in checks if check.status == "critical"),
        "total": len(checks),
    }

    return {
        "name": budget.get("name", "Frontend Performance Budget"),
        "version": budget.get("version", "1.0.0"),
        "metrics": metrics,
        "summary": summary,
        "checks": [check.__dict__ for check in checks],
        "notices": notices,
        "valid": summary["warnings"] == 0 and summary["critical"] == 0,
    }


def render_summary(report: dict[str, Any]) -> str:
    lines = [
        "=" * 64,
        f"{report['name']} (v{report['version']})",
        "=" * 64,
        (
            f"Checks: {report['summary']['total']} | "
            f"passed: {report['summary']['passed']} | "
            f"warnings: {report['summary']['warnings']} | "
            f"critical: {report['summary']['critical']}"
        ),
        "",
        "Collected metrics:",
        f"- root_main_js_kb: {report['metrics']['root_main_js_kb']}",
        f"- largest_lazy_chunk_js_kb: {report['metrics']['largest_lazy_chunk_js_kb']}",
        f"- css_total_kb: {report['metrics']['css_total_kb']}",
        "",
        "Route totals:",
    ]

    for route, route_metrics in sorted(report["metrics"]["routes"].items()):
        lines.append(
            f"- {route}: js={route_metrics['total_js_kb']}KB css={route_metrics['total_css_kb']}KB"
        )

    flagged = [check for check in report["checks"] if check["status"] != "pass"]
    if flagged:
        lines.extend(["", "Budget issues:"])
        for check in flagged:
            lines.append(
                f"- {check['status'].upper()}: {check['name']} actual={check['actual']} "
                f"warning={check['warning']} critical={check['critical']}"
            )

    if report["notices"]:
        lines.extend(["", "Notices:"])
        for notice in report["notices"]:
            lines.append(f"- {notice}")

    return "\n".join(lines)


def main() -> int:
    args = parse_args()
    budget_path = Path(args.budget)
    build_dir = Path(args.build_dir)

    try:
        budget = load_json(budget_path)
        metrics = collect_metrics(build_dir)
        report = validate_metrics(budget, metrics)
    except Exception as exc:  # noqa: BLE001
        print(f"Error: {exc}", file=sys.stderr)
        return 1

    if args.output:
        output_path = Path(args.output)
        output_path.parent.mkdir(parents=True, exist_ok=True)
        output_path.write_text(json.dumps(report, indent=2))

    if args.summary or not args.output:
        print(render_summary(report))

    if args.fail_on_violation and not report["valid"]:
        return 1

    return 0


if __name__ == "__main__":
    sys.exit(main())
