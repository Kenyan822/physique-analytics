#!/usr/bin/env python3
"""デモ用のサンプルデータを data/sample/ に生成する。

実データを公開しない代わりに、clone した人が分析を動かせるようにするためのもの。
計画（config.json）に沿った P0（体重維持・TDEE実測）と P1-A（-1.0%/週）の
2フェーズ分を生成するので、TDEE の動的推定が切り替わる様子が確認できる。

    python3 reference/analysis/make_sample_data.py
    python3 reference/analysis/analyze.py --data-dir data/sample --config config.example.json --no-write
"""
import argparse
import json
from pathlib import Path

import numpy as np
import pandas as pd

START, END = "2026-09-07", "2026-10-31"
P1_START = "2026-10-01"          # ここから減量開始
rng = np.random.default_rng(42)

# 6日サイクル（1日4種目・週6日・脚は週1回の配分）
# 重量は 1RM ベンチ100kg / スクワット103kg 相当に合わせてある
CYCLE = {
    0: [("ベンチプレス", 5, 81, 5, 2), ("インクラインベンチプレス", 4, 60, 9, 2),
        ("ディップス", 4, 76, 11, 2), ("ケーブルフライ", 3, 20, 14, 3)],
    1: [("スクワット", 4, 77, 8, 2), ("レッグプレス", 4, 150, 10, 2),
        ("レッグエクステンション", 4, 50, 12, 2), ("レッグカール", 4, 40, 12, 2)],
    2: [("ナローベンチプレス", 4, 62, 10, 2), ("バーベルアームカール", 4, 35, 10, 2),
        ("ライイングエクステンション", 4, 30, 10, 2), ("ダンベルアームカール", 4, 14, 10, 2)],
    3: [("荷重懸垂", 4, 86, 7, 2), ("ラットプルダウン", 4, 60, 11, 2),
        ("ワンハンドロー", 3, 32, 11, 2), ("ルーマニアンデッドリフト", 4, 90, 9, 2)],
    4: [("ケーブルサイドレイズ", 5, 9, 14, 3), ("ダンベルサイドレイズ", 4, 10, 16, 3),
        ("フェイスプル", 4, 25, 14, 3), ("ミリタリープレス", 4, 45, 7, 2)],
    5: [("デッドリフト", 4, 100, 5, 2), ("バーベルロー", 4, 70, 9, 2),
        ("シーテッドロー", 4, 60, 12, 2), ("腹筋", 4, 0, 15, 2)],
}


def main() -> None:
    ap = argparse.ArgumentParser()
    ap.add_argument("--config", default="../config.example.json")
    ap.add_argument("--out-dir", default="../data/sample")
    a = ap.parse_args()

    cfg = json.loads(Path(a.config).read_text(encoding="utf-8"))
    base = cfg["baseline"]
    w0 = float(base["weight_kg"])
    bf0 = float(base["bodyfat_pct"])
    OUT = Path(a.out_dir)
    OUT.mkdir(parents=True, exist_ok=True)
    days = pd.date_range(START, END, freq="D")
    p1 = pd.Timestamp(P1_START)
    n = len(days)

    # --- 体重: P0 は維持、P1-A は -1.0%/週。日々±0.35kg の水分ノイズを乗せる ---
    w, cur = [], w0
    for d in days:
        if d >= p1:
            cur -= w0 * 0.010 / 7      # -1.0%/週
        w.append(cur)
    weight = np.array(w) + rng.normal(0, 0.35, n)
    is_cut = np.array([d >= p1 for d in days])

    daily = pd.DataFrame({
        "date": days.strftime("%Y-%m-%d"),
        "weight_kg": weight.round(1),
        "bodyfat_pct": (np.where(is_cut, np.linspace(bf0 - 0.2, bf0 - 3.3, n), bf0 - 0.1)
                        + rng.normal(0, 0.4, n)).round(1),
        # P0 は TDEE 相当(2800)、P1-A は赤字(2150)
        "kcal": np.where(is_cut, rng.normal(2150, 90, n), rng.normal(2800, 110, n)).round().astype(int),
        "protein_g": np.where(is_cut, rng.normal(w0 * 2.4, 11, n),
                              rng.normal(w0 * 2.0, 11, n)).round().astype(int),
        "fat_g": np.where(is_cut, rng.normal(65, 7, n), rng.normal(85, 9, n)).round().astype(int),
        "carb_g": np.where(is_cut, rng.normal(205, 22, n), rng.normal(320, 28, n)).round().astype(int),
        "sleep_h": rng.normal(7.1, 0.7, n).round(1),
        "steps": np.where(is_cut, rng.normal(11500, 1100, n), rng.normal(8500, 1000, n)).round().astype(int),
        "fatigue": rng.integers(2, 5, n),
        # Apple Watch 由来（未導入なら空でよい列）
        "hrv_ms": np.where(is_cut, rng.normal(56, 5, n), rng.normal(62, 5, n)).round().astype(int),
        "resting_hr": np.where(is_cut, rng.normal(55, 2, n), rng.normal(52, 2, n)).round().astype(int),
        "deep_sleep_min": rng.normal(72, 10, n).round().astype(int),
        "note": [""] * n,
    })
    daily.to_csv(OUT / "daily.csv", index=False)

    # --- トレーニング: 6日サイクルを回し、7日目は休み ---
    rows, cycle_day = [], 0
    for i, d in enumerate(days):
        if i % 7 == 6:
            continue                                  # 週1日はオフ
        drift = -0.3 * (i / n) if d >= p1 else 0.0    # 減量中はわずかに低下
        for ex, sets, base_w, reps, rir in CYCLE[cycle_day % 6]:
            for s in range(1, sets + 1):
                rows.append({
                    "date": d.strftime("%Y-%m-%d"), "exercise": ex, "set_no": s,
                    "weight_kg": round(base_w * (1 + drift / 100), 1) if base_w else 0,
                    "reps": int(reps + rng.integers(-1, 2)),
                    "rir": int(max(0, rir + rng.integers(-1, 2))),
                })
        cycle_day += 1
    pd.DataFrame(rows).to_csv(OUT / "workouts.csv", index=False)

    # --- 周囲長: 日曜朝に計測 ---
    sundays = [d for d in days if d.weekday() == 6]
    k = len(sundays)
    pd.DataFrame({
        "date": [d.strftime("%Y-%m-%d") for d in sundays],
        "neck_cm": np.linspace(39.0, 38.2, k).round(1),
        "shoulder_cm": np.linspace(118.0, 117.0, k).round(1),
        "chest_cm": np.linspace(103.0, 100.8, k).round(1),
        "waist_navel_cm": np.linspace(86.0, 81.0, k).round(1),
        "hip_cm": np.linspace(97.0, 94.6, k).round(1),
        "arm_r_cm": np.linspace(35.0, 34.7, k).round(1),
        "thigh_r_cm": np.linspace(54.0, 53.4, k).round(1),
        "calf_r_cm": np.linspace(36.0, 35.8, k).round(1),
    }).to_csv(OUT / "measures.csv", index=False)

    # 月次目標（あれば同梱する。無くても分析は動く）
    plan = OUT / "monthly_plan.csv"
    if not plan.exists():
        import subprocess
        subprocess.run(["python3", "reference/analysis/make_monthly_plan.py", "--config", a.config,
                        "--out-docs", "/tmp/_sample_plan.md", "--out-csv", str(plan)],
                       check=False, capture_output=True)

    print(f"生成: {OUT}/  daily {n}行 / workouts {len(rows)}行 / measures {k}行")
    print(f"  期間 {START} 〜 {END}（P0: 体重維持 → P1-A: -1.0%/週）")
    print(f"  起点 {w0}kg / BF{bf0}%（{a.config} より）")
    print(f"  確認: python3 reference/analysis/analyze.py --data-dir {OUT} --config {a.config} --no-write")


if __name__ == "__main__":
    main()
