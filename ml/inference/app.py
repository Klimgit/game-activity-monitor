"""HTTP inference for sklearn window classifier (same feature schema as Colab training)."""

from __future__ import annotations

import json
import os
from pathlib import Path

import joblib
import pandas as pd
from fastapi import FastAPI
from pydantic import BaseModel, Field

MODEL_PATH = os.environ.get("MODEL_PATH", "/models/classifier.joblib")

TEXT_COLS = ("active_process", "foreground_window_title", "game_name")

pipe = joblib.load(MODEL_PATH)
prep = pipe.named_steps["prep"]
cols: list[str] = list(getattr(prep, "feature_names_in_", []) or [])
if not cols:
    meta_path = Path(MODEL_PATH).with_name("training_metadata.json")
    if meta_path.is_file():
        meta = json.loads(meta_path.read_text(encoding="utf-8"))
        cols = list(meta.get("numeric_features", [])) + list(meta.get("text_features", []))
if not cols:
    raise RuntimeError(
        "Could not determine feature columns: need sklearn ColumnTransformer.feature_names_in_ "
        "or training_metadata.json next to the model."
    )

app = FastAPI(title="window-classifier-inference", version="1.0.0")


class PredictBody(BaseModel):
    rows: list[dict] = Field(default_factory=list)


@app.get("/health")
def health() -> dict[str, str]:
    return {"status": "ok"}


@app.post("/predict")
def predict(body: PredictBody) -> dict[str, list[str]]:
    if not body.rows:
        return {"predictions": []}
    df = pd.DataFrame(body.rows)
    for c in cols:
        if c not in df.columns:
            df[c] = "" if c in TEXT_COLS else 0.0
    df = df.loc[:, cols].copy()
    for c in cols:
        if c in TEXT_COLS:
            df[c] = df[c].fillna("").astype(str)
        else:
            df[c] = pd.to_numeric(df[c], errors="coerce").fillna(0.0)
    pred = pipe.predict(df)
    return {"predictions": [str(x) for x in pred.tolist()]}
