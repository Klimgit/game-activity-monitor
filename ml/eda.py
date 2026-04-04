#!/usr/bin/env python3
from __future__ import annotations

import argparse
import sys
from pathlib import Path

import numpy as np
import pandas as pd

from constants import GPU_COLS, LABEL_COL

EXPECTED_LABELS = frozenset({"active_gameplay", "afk", "loading", "menu"})


def _gpu_useful(df: pd.DataFrame) -> dict[str, bool]:
    out = {}
    for c in GPU_COLS:
        if c not in df.columns:
            out[c] = False
            continue
        s = pd.to_numeric(df[c], errors="coerce").fillna(0)
        out[c] = bool(np.nanmax(np.abs(s.values)) > 1e-9)
    return out


def _empty_text_mask(s: pd.Series) -> pd.Series:
    return s.fillna("").astype(str).str.strip().str.len() == 0


def _near_constant_numeric(df: pd.DataFrame, skip: set[str]) -> list[str]:
    bad = []
    for c in df.select_dtypes(include=[np.number]).columns:
        if c in skip:
            continue
        v = df[c].dropna()
        if len(v) == 0:
            bad.append(f"{c} (all NaN)")
            continue
        if v.nunique() <= 1:
            bad.append(f"{c} (single value)")
            continue
        if float(v.std()) < 1e-12:
            bad.append(f"{c} (std≈0)")
    return bad


def main() -> int:
    p = argparse.ArgumentParser(description="EDA for game-activity window CSV.")
    p.add_argument("csv", type=Path, help="Path to raw_windows.csv")
    p.add_argument(
        "--report",
        type=Path,
        default=None,
        help="Optional path to write eda_report.txt",
    )
    p.add_argument(
        "--top-k",
        type=int,
        default=12,
        help="Top frequent values for categorical text columns",
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

    log(f"File: {path}")
    log(f"Rows: {len(df)}  Columns: {len(df.columns)}")
    log("")

    if LABEL_COL not in df.columns:
        log(f"Missing column {LABEL_COL!r}")
        return 1

    # --- Time range ---
    if "window_start" in df.columns:
        ts = pd.to_datetime(df["window_start"], utc=True, errors="coerce")
        if ts.notna().any():
            log(f"window_start range (UTC): {ts.min()} … {ts.max()}")
            log("")

    # --- Sessions ---
    if "session_id" in df.columns:
        n_sess = df["session_id"].nunique()
        per_sess = df.groupby("session_id").size()
        log(f"Unique session_id: {n_sess}")
        log(f"Rows per session — min {per_sess.min()}, max {per_sess.max()}, mean {per_sess.mean():.1f}")
        if len(per_sess) > 1 and per_sess.max() / max(1, per_sess.min()) > 30:
            log(
                "⚠ Very uneven rows per session (long vs short sessions); "
                "stratify collection or weight rows if needed."
            )
        if n_sess < 8:
            log(
                "⚠ Few distinct sessions: session-based train/val/test is noisy; "
                "aim for more sessions (e.g. 10+) for stable metrics."
            )
        log("")

    # --- Labels ---
    vc = df[LABEL_COL].astype(str).str.strip()
    vc = vc.replace("", "(empty)").value_counts(dropna=False)
    log("Label distribution:")
    n = len(df)
    for k, v in vc.items():
        log(f"  {k!r}: {v} ({100 * v / n:.1f}%)")
    present = {str(x) for x in vc.index if str(x) not in ("(empty)", "nan", "None")}
    missing_lbl = EXPECTED_LABELS - present
    if missing_lbl:
        log(f"  (no rows labelled: {', '.join(sorted(missing_lbl))})")
    min_pct = 100 * vc.min() / n if len(vc) else 0
    if min_pct < 5 and len(vc) > 1:
        log(f"⚠ Strong class imbalance: rarest class ≈ {min_pct:.1f}% of rows")
    log("")

    # --- Duplicates ---
    if "session_id" in df.columns and "window_start" in df.columns:
        dup = df.duplicated(subset=["session_id", "window_start"], keep=False).sum()
        if dup:
            log(f"⚠ Duplicate (session_id, window_start) rows: {dup}")
            log("")

    # --- Text columns ---
    for col in ("game_name", "active_process", "foreground_window_title"):
        if col not in df.columns:
            continue
        empty = int(_empty_text_mask(df[col]).sum())
        log(f"{col}: empty {empty} ({100 * empty / max(1, n):.1f}%)")
        vc2 = (
            df[col]
            .fillna("")
            .astype(str)
            .replace("", "(empty)")
            .value_counts()
            .head(args.top_k)
        )
        log(f"  top {args.top_k}:")
        for k, v in vc2.items():
            log(f"    {k!r}: {v}")
        log("")

    # --- title_match_score vs empty title ---
    if "title_match_score" in df.columns and "foreground_window_title" in df.columns:
        tms = pd.to_numeric(df["title_match_score"], errors="coerce")
        empty_title = _empty_text_mask(df["foreground_window_title"])
        log("title_match_score:")
        log(f"  non-null numeric: {tms.notna().sum()}  |  median (non-null): {tms.median():.4f}")
        log(
            f"  where title empty: rows={empty_title.sum()}, "
            f"match median={tms[empty_title].median() if empty_title.any() else float('nan')}"
        )
        log(
            f"  where title non-empty: rows={(~empty_title).sum()}, "
            f"match median={tms[~empty_title].median() if (~empty_title).any() else float('nan')}"
        )
        log("")

    # --- Numeric overview (exclude obvious IDs from near-constant scan) ---
    skip_const = {"user_id", "session_id", "window_index"}
    bad_num = _near_constant_numeric(df, skip_const)
    if bad_num:
        log("Near-constant or useless numeric columns (check before modelling):")
        for b in bad_num[:25]:
            log(f"  • {b}")
        if len(bad_num) > 25:
            log(f"  … +{len(bad_num) - 25} more")
        log("")

    num_cols = [c for c in df.select_dtypes(include=[np.number]).columns if c not in skip_const]
    log("Numeric describe (key columns only):")
    key_num = [
        c
        for c in num_cols
        if c
        in (
            "duration_s",
            "mouse_moves",
            "mouse_clicks",
            "speed_avg",
            "speed_max",
            "keystrokes",
            "cpu_avg",
            "mem_avg",
            "cursor_accel_avg",
            "title_match_score",
        )
    ]
    show = [c for c in key_num if c in df.columns]
    if show:
        log(df[show].describe().T.to_string())
    else:
        log(df[num_cols].describe().to_string())
    log("")

    gpu_ok = _gpu_useful(df)
    log("GPU columns — usable if any non-zero (otherwise drop for training):")
    for c, ok in gpu_ok.items():
        log(f"  {c}: {'yes' if ok else 'no (all zero / missing)'}")
    if not any(gpu_ok.values()):
        log("→ Recommend --drop-gpu auto (default in Colab notebook).")
    log("")

    # --- Heuristic recommendations ---
    log("--- Suggestions ---")
    if "session_id" in df.columns and df["session_id"].nunique() < 8:
        log("• Add more distinct sessions (several long recordings) for reliable session-wise split.")
    if missing_lbl:
        log(f"• Label more intervals so all states appear: missing {sorted(missing_lbl)}.")
    if "foreground_window_title" in df.columns and _empty_text_mask(df["foreground_window_title"]).mean() > 0.3:
        log("• Many empty window titles: title_match_score will be weak; Wayland or focus API limits.")
    log("")

    if args.report:
        args.report.parent.mkdir(parents=True, exist_ok=True)
        args.report.write_text("\n".join(lines) + "\n", encoding="utf-8")
        print(f"Wrote {args.report}", file=sys.stderr)

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
