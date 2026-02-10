#!/usr/bin/env python3
"""
Stage 2 — train a multiclass classifier on labelled windows.

- Uses only rows with non-empty label (same as collectdataset -training-only=true export).
- Train / validation / test split by session_id (no session appears in two splits).
- Drops GPU features if they are all-zero in the training frame (or use --drop-gpu).
- Includes active_process, foreground_window_title, game_name as one-hot features.

Saves: models/classifier.joblib (sklearn Pipeline) and models/training_metadata.json
"""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path
import joblib
import numpy as np
import pandas as pd
from sklearn.compose import ColumnTransformer
from sklearn.ensemble import HistGradientBoostingClassifier
from sklearn.metrics import classification_report, f1_score
from sklearn.model_selection import GroupShuffleSplit
from sklearn.pipeline import Pipeline
from sklearn.preprocessing import OneHotEncoder, StandardScaler

from constants import GPU_COLS, LABEL_COL, META_COLS, TEXT_COLS


def _numeric_feature_columns(df: pd.DataFrame, drop_gpu: set[str]) -> list[str]:
    skip = set(META_COLS) | set(TEXT_COLS) | drop_gpu
    cols = []
    for c in df.columns:
        if c in skip or c == LABEL_COL:
            continue
        if pd.api.types.is_numeric_dtype(df[c]):
            cols.append(c)
    return sorted(cols)


def main() -> int:
    ap = argparse.ArgumentParser(description="Train window-state classifier.")
    ap.add_argument("csv", type=Path, help="Labelled window CSV")
    ap.add_argument(
        "--out-dir",
        type=Path,
        default=Path("models"),
        help="Directory for classifier.joblib and training_metadata.json",
    )
    ap.add_argument("--seed", type=int, default=42)
    ap.add_argument(
        "--drop-gpu",
        choices=("auto", "always", "never"),
        default="auto",
        help="auto: drop GPU cols if all zero in CSV; always/never: force",
    )
    ap.add_argument("--val-frac", type=float, default=0.15, help="Fraction of sessions for validation")
    ap.add_argument("--test-frac", type=float, default=0.15, help="Fraction of sessions for test")
    args = ap.parse_args()

    csv_path = args.csv if args.csv.is_absolute() else Path(__file__).resolve().parent / args.csv
    if not csv_path.exists():
        print(f"train: file not found: {csv_path}", file=sys.stderr)
        return 1

    df = pd.read_csv(csv_path)
    df = df[df[LABEL_COL].astype(str).str.strip() != ""].copy()
    if len(df) < 10:
        print("train: need more labelled rows (>=10)", file=sys.stderr)
        return 1

    if "session_id" not in df.columns:
        print("train: missing session_id", file=sys.stderr)
        return 1

    # GPU columns
    drop_gpu: set[str] = set()
    if args.drop_gpu == "always":
        drop_gpu = set(GPU_COLS)
    elif args.drop_gpu == "never":
        drop_gpu = set()
    else:
        for c in GPU_COLS:
            if c not in df.columns:
                drop_gpu.add(c)
                continue
            s = pd.to_numeric(df[c], errors="coerce").fillna(0)
            if np.nanmax(np.abs(s.values)) <= 1e-9:
                drop_gpu.add(c)

    num_cols = _numeric_feature_columns(df, drop_gpu)
    text_cols = [c for c in TEXT_COLS if c in df.columns]

    for c in text_cols:
        df[c] = df[c].fillna("").astype(str)

    y = df[LABEL_COL].astype(str)
    groups = df["session_id"].values
    idx = np.arange(len(df))

    # First: hold out test sessions
    gss_test = GroupShuffleSplit(n_splits=1, test_size=args.test_frac, random_state=args.seed)
    try:
        tr_val_idx, te_idx = next(gss_test.split(idx, y, groups=groups))
    except ValueError as e:
        print(f"train: cannot split sessions for test ({e})", file=sys.stderr)
        return 1

    df_tv = df.iloc[tr_val_idx].reset_index(drop=True)
    y_tv = y.iloc[tr_val_idx].reset_index(drop=True)
    groups_tv = df_tv["session_id"].values
    idx_tv = np.arange(len(df_tv))

    # Second: validation sessions from the remainder
    rel_val = args.val_frac / max(1e-9, (1.0 - args.test_frac))
    rel_val = min(0.99, max(0.01, rel_val))
    gss_val = GroupShuffleSplit(n_splits=1, test_size=rel_val, random_state=args.seed + 1)
    try:
        tr_idx, va_idx = next(gss_val.split(idx_tv, y_tv, groups=groups_tv))
    except ValueError:
        tr_idx, va_idx = idx_tv, np.array([], dtype=int)

    df_train = df_tv.iloc[tr_idx]
    df_val = df_tv.iloc[va_idx] if len(va_idx) else df_tv.iloc[[]]
    df_test = df.iloc[te_idx]

    feature_cols = num_cols + text_cols
    X_train = df_train[feature_cols]
    y_train = y_tv.iloc[tr_idx]
    X_val = df_val[feature_cols] if len(df_val) else pd.DataFrame(columns=feature_cols)
    y_val = y_tv.iloc[va_idx] if len(va_idx) else pd.Series(dtype=str)
    X_test = df_test[feature_cols]
    y_test = y.iloc[te_idx].reset_index(drop=True)

    train_sessions = df_train["session_id"].nunique()
    val_sessions = df_val["session_id"].nunique() if len(df_val) else 0
    test_sessions = df_test["session_id"].nunique()

    if len(X_train) == 0:
        print("train: empty train split — not enough sessions", file=sys.stderr)
        return 1

    transformers = [
        ("num", StandardScaler(), num_cols),
    ]
    if text_cols:
        transformers.append(
            (
                "cat",
                OneHotEncoder(handle_unknown="ignore", max_categories=40, sparse_output=False),
                text_cols,
            )
        )

    pre = ColumnTransformer(transformers, remainder="drop", verbose_feature_names_out=False)

    clf = HistGradientBoostingClassifier(
        max_depth=12,
        learning_rate=0.08,
        max_iter=200,
        random_state=args.seed,
    )
    pipe = Pipeline([("prep", pre), ("clf", clf)])
    pipe.fit(X_train, y_train)

    out_dir = args.out_dir if args.out_dir.is_absolute() else Path(__file__).resolve().parent / args.out_dir
    out_dir.mkdir(parents=True, exist_ok=True)
    joblib.dump(pipe, out_dir / "classifier.joblib")

    labels = sorted(y.unique().tolist())
    report_lines: list[str] = []

    def eval_split(name: str, Xp: pd.DataFrame, yp: pd.Series) -> None:
        if len(Xp) == 0:
            report_lines.append(f"=== {name} (empty) ===\n")
            return
        pred = pipe.predict(Xp)
        report_lines.append(f"=== {name} (n={len(Xp)}) ===\n")
        report_lines.append(classification_report(yp, pred, zero_division=0))
        macro = f1_score(yp, pred, average="macro", zero_division=0)
        report_lines.append(f"macro-F1: {macro:.4f}\n")

    eval_split("train", X_train, y_train)
    if len(X_val):
        eval_split("validation", X_val, y_val)
    eval_split("test", X_test, y_test)

    report_txt = "".join(report_lines)
    print(report_txt)
    (out_dir / "metrics.txt").write_text(report_txt, encoding="utf-8")

    meta = {
        "csv": str(csv_path.resolve()),
        "seed": args.seed,
        "drop_gpu_mode": args.drop_gpu,
        "dropped_gpu_columns": sorted(drop_gpu),
        "numeric_features": num_cols,
        "text_features": text_cols,
        "labels": labels,
        "session_counts": {
            "train": int(train_sessions),
            "val": int(val_sessions),
            "test": int(test_sessions),
        },
        "row_counts": {
            "train": int(len(X_train)),
            "val": int(len(X_val)),
            "test": int(len(X_test)),
        },
    }
    (out_dir / "training_metadata.json").write_text(json.dumps(meta, indent=2), encoding="utf-8")
    print(f"Saved {out_dir / 'classifier.joblib'}", file=sys.stderr)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
