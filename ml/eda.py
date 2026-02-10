#!/usr/bin/env python3
"""
Stage 1 — exploratory analysis on the exported window CSV.

Reads the file produced by prepare_dataset.py, prints summary stats and class balance,
checks GPU columns (if all zeros, do not use them for modelling), and optionally
writes a text report.

Model training: use colab/train_classifier.ipynb in Colab or Kaggle (no local train script).
"""

from __future__ import annotations

import argparse
import sys
from pathlib import Path

import numpy as np
import pandas as pd

from constants import GPU_COLS, LABEL_COL


def _gpu_useful(df: pd.DataFrame) -> dict[str, bool]:
    out = {}
    for c in GPU_COLS:
        if c not in df.columns:
            out[c] = False
            continue
        s = pd.to_numeric(df[c], errors="coerce").fillna(0)
        out[c] = bool(np.nanmax(np.abs(s.values)) > 1e-9)
    return out


def main() -> int:
    p = argparse.ArgumentParser(description="EDA for game-activity window CSV.")
    p.add_argument("csv", type=Path, help="Path to raw_windows.csv")
    p.add_argument(
        "--report",
        type=Path,
        default=None,
        help="Optional path to write eda_report.txt",
    )
    args = p.parse_args()

    path = args.csv if args.csv.is_absolute() else Path(__file__).resolve().parent / args.csv
    if not path.exists():
        print(f"eda: file not found: {path}", file=sys.stderr)
        return 1

    df = pd.read_csv(path)
    lines: list[str] = []

    def log(msg: str) -> None:
        lines.append(msg)
        print(msg)

    log(f"Rows: {len(df)}  Columns: {len(df.columns)}")
    log(f"Columns: {', '.join(df.columns)}")
    log("")

    if LABEL_COL not in df.columns:
        log(f"Missing column {LABEL_COL!r}")
        return 1

    vc = df[LABEL_COL].astype(str).value_counts(dropna=False)
    log("Label distribution:")
    for k, v in vc.items():
        log(f"  {k!r}: {v} ({100 * v / len(df):.1f}%)")
    log("")

    n_sess = df["session_id"].nunique() if "session_id" in df.columns else 0
    log(f"Unique session_id: {n_sess}")
    log("")

    num_cols = df.select_dtypes(include=[np.number]).columns.tolist()
    log("Numeric summary (head):")
    log(df[num_cols].describe().to_string())
    log("")

    gpu_ok = _gpu_useful(df)
    log("GPU columns — non-zero anywhere (if all False, drop for training):")
    for c, ok in gpu_ok.items():
        log(f"  {c}: {'yes' if ok else 'NO — all zero / missing'}")
    log("")

    empty_title = (df.get("foreground_window_title", pd.Series(dtype=str)).astype(str).str.len() == 0).sum()
    log(f"Empty foreground_window_title rows: {empty_title}")

    if args.report:
        args.report.parent.mkdir(parents=True, exist_ok=True)
        args.report.write_text("\n".join(lines) + "\n", encoding="utf-8")
        print(f"Wrote {args.report}", file=sys.stderr)

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
