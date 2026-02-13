#!/usr/bin/env python3
from __future__ import annotations

import argparse
import os
import subprocess
import sys
from pathlib import Path


def repo_root() -> Path:
    return Path(__file__).resolve().parent.parent


def main() -> int:
    p = argparse.ArgumentParser(description="Export labelled window CSV via server/cmd/collectdataset.")
    p.add_argument("--from", dest="date_from", required=True, help="Start date YYYY-MM-DD (UTC)")
    p.add_argument("--to", dest="date_to", required=True, help="End date YYYY-MM-DD (UTC)")
    p.add_argument(
        "--out",
        type=Path,
        default=Path("data/raw_windows.csv"),
        help="Output CSV path (default: data/raw_windows.csv under ml/)",
    )
    p.add_argument(
        "--database-url",
        default=os.environ.get("DATABASE_URL", ""),
        help="Postgres URL (default: $DATABASE_URL)",
    )
    p.add_argument(
        "--user",
        type=int,
        default=0,
        help="If set, export only this user id (default: all users)",
    )
    p.add_argument(
        "--session-id",
        default="",
        help="Optional session id filter",
    )
    args = p.parse_args()

    if not args.database_url:
        print("prepare_dataset: set --database-url or DATABASE_URL", file=sys.stderr)
        return 1

    root = repo_root()
    server_dir = root / "server"
    out = args.out if args.out.is_absolute() else Path(__file__).resolve().parent / args.out
    out.parent.mkdir(parents=True, exist_ok=True)

    cmd = [
        "go",
        "run",
        "./cmd/collectdataset",
        "-from",
        args.date_from,
        "-to",
        args.date_to,
        "-o",
        str(out),
        "-database-url",
        args.database_url,
    ]
    if args.user:
        cmd.extend(["-user", str(args.user)])
    if args.session_id:
        cmd.extend(["-session-id", args.session_id])

    print("Running:", " ".join(cmd), file=sys.stderr)
    print("cwd:", server_dir, file=sys.stderr)

    env = os.environ.copy()
    env["DATABASE_URL"] = args.database_url

    r = subprocess.run(cmd, cwd=server_dir, env=env)
    if r.returncode != 0:
        return r.returncode

    print(f"Wrote {out}", file=sys.stderr)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
