#!/usr/bin/env python3
"""週次分析レポート生成。docs/03-分析ロジック.md の仕様を実装する。

使い方:
    python3 reference/analysis/analyze.py                    # 今日を基準に生成
    python3 reference/analysis/analyze.py --asof 2026-12-07  # 基準日を指定
    python3 reference/analysis/analyze.py --data-dir /path   # 別のデータを分析（動作確認用）
"""
from __future__ import annotations

import argparse
import json
from datetime import date, datetime, timedelta
from pathlib import Path

import numpy as np
import pandas as pd

KCAL_PER_KG_FAT = 7700.0
TREND_WINDOW_DAYS = 21     # 体重トレンドの回帰窓
TDEE_MIN_DAYS = 10         # TDEE推定に必要な最低記録日数
E1RM_WINDOW_DAYS = 42      # e1RM 傾きの回帰窓（6週）
E1RM_MAX_REPS = 12         # Epley はこれを超えると推定が過大になる
FLOOR_KCAL_PER_KG = 24.0   # 摂取量の下限(体重×kcal)。これ以下は削らず活動量で作る

# 種目 -> 部位。表記は data/workouts.csv の exercise 列と完全一致させる。
# 種目 -> 部位。表記は data/workouts.csv の exercise 列と完全一致させる。
# 肩は3分割、背中は広背筋/僧帽筋に分割している。サイドレイズ過多のような
# 部位内の偏りは、一括の「肩」では検出できないため。
# ロー系の広背/僧帽の割り当ては、本人の狙い（グリップ・軌道）に合わせてある。
EXERCISE_MUSCLE = {
    # 胸
    "ベンチプレス": "胸", "バーベルベンチプレス": "胸", "ダンベルベンチプレス": "胸",
    "インクラインベンチプレス": "胸", "インクラインダンベルプレス": "胸",
    "チェストプレス": "胸", "ディップス": "胸", "ケーブルフライ": "胸", "ペックフライ": "胸",
    # 背中（広背筋）
    "荷重懸垂": "広背筋", "懸垂": "広背筋", "ラットプルダウン": "広背筋",
    "ワンハンドロー": "広背筋", "ワンハンドロウ": "広背筋",
    # 背中（僧帽筋・脊柱起立筋）
    "バーベルロー": "僧帽筋", "ベントオーバーロウ": "僧帽筋", "シーテッドロー": "僧帽筋",
    "シーテッドロウ": "僧帽筋", "チェストサポートロー": "僧帽筋", "ローロウ": "僧帽筋",
    "デッドリフト": "僧帽筋", "シュラッグ": "僧帽筋",
    # 肩
    "ミリタリープレス": "肩前部", "ショルダープレス": "肩前部", "フロントレイズ": "肩前部",
    "サイドレイズ": "肩中部", "ダンベルサイドレイズ": "肩中部",
    "ケーブルサイドレイズ": "肩中部", "アップライトロウ": "肩中部",
    "フェイスプル": "肩後部", "リアレイズ": "肩後部",
    # 腕
    "バーベルアームカール": "上腕二頭", "バーベルカール": "上腕二頭",
    "ダンベルアームカール": "上腕二頭", "ダンベルカール": "上腕二頭",
    "ハンマーカール": "上腕二頭", "インクラインカール": "上腕二頭",
    "ナローベンチプレス": "上腕三頭", "ライイングエクステンション": "上腕三頭",
    "スカルクラッシャー": "上腕三頭", "ケーブルプレスダウン": "上腕三頭",
    "フレンチプレス": "上腕三頭",
    # 脚
    "スクワット": "大腿四頭", "バーベルスクワット": "大腿四頭", "レッグプレス": "大腿四頭",
    "ハックスクワット": "大腿四頭", "ブルガリアンスクワット": "大腿四頭",
    "ランジ": "大腿四頭", "レッグエクステンション": "大腿四頭",
    "ルーマニアンデッドリフト": "ハム", "レッグカール": "ハム", "スティフレッグデッドリフト": "ハム",
    "ヒップスラスト": "臀部", "カーフレイズ": "ふくらはぎ",
    # 体幹
    "腹筋": "腹", "アブローラー": "腹", "ケーブルクランチ": "腹", "レッグレイズ": "腹",
}
KEY_EXERCISES = ["ベンチプレス", "スクワット", "デッドリフト", "ミリタリープレス",
                 "荷重懸垂", "バーベルロー"]
LOWER_BODY = {"大腿四頭", "ハム", "臀部", "ふくらはぎ"}


# ---------- 入出力 ----------

def load_config(path: Path) -> dict:
    if not path.exists():
        raise FileNotFoundError(f"config が無い: {path}")
    return json.loads(path.read_text(encoding="utf-8"))


def read_table(path: Path) -> pd.DataFrame:
    if not path.exists():
        raise FileNotFoundError(f"データファイルが無い: {path}")
    df = pd.read_csv(path)
    if "date" not in df.columns:
        raise ValueError(f"{path.name} に date 列が無い")
    if len(df):
        df["date"] = pd.to_datetime(df["date"], errors="raise")
    return df


def col_series(df: pd.DataFrame, name: str) -> pd.Series:
    """列が無い場合も空 Series を返す。Apple Watch 未導入時や古い CSV との互換のため。"""
    return df[name].dropna() if name in df.columns else pd.Series(dtype=float)


def window(df: pd.DataFrame, asof: pd.Timestamp, days: int) -> pd.DataFrame:
    if not len(df):
        return df
    return df[(df["date"] > asof - timedelta(days=days)) & (df["date"] <= asof)]


# ---------- 計算 ----------

def slope_per_day(df: pd.DataFrame, col: str) -> tuple[float | None, int]:
    """日次データに線形回帰をかけ、傾き(単位/日)と有効点数を返す。"""
    d = df[["date", col]].dropna()
    if len(d) < 4:
        return None, len(d)
    x = (d["date"] - d["date"].min()).dt.total_seconds() / 86400.0
    slope = float(np.polyfit(x.to_numpy(), d[col].to_numpy(dtype=float), 1)[0])
    return slope, len(d)


def estimate_tdee(daily: pd.DataFrame, asof: pd.Timestamp) -> dict:
    """分析1: TDEE = 平均摂取kcal - 体重トレンド(kg/日) * 7700"""
    w = window(daily, asof, TREND_WINDOW_DAYS)
    kcal = w["kcal"].dropna()
    wslope, n_w = slope_per_day(w, "weight_kg")
    mean_kcal: float | None = float(kcal.mean()) if len(kcal) else None
    tdee: float | None = None
    if mean_kcal is not None and wslope is not None and len(kcal) >= TDEE_MIN_DAYS:
        tdee = mean_kcal - wslope * KCAL_PER_KG_FAT
    return {"n_kcal": len(kcal), "n_weight": n_w, "window": TREND_WINDOW_DAYS,
            "mean_kcal": mean_kcal,
            "weight_slope_week": wslope * 7 if wslope is not None else None,
            "tdee": tdee}


def norm_ffmi(lbm: float, height_cm: float) -> float:
    """正規化FFMI。除脂肪体重を身長で正規化し、身長差を補正した筋肉量の指標。
    18-19=未トレーニング / 20-21=明らかに鍛えている / 22-23=ジムで目立つ / 24-25=ナチュラル上限。
    体重より先にこれを見る。体重は身長に強く依存し、比較に使えない。"""
    h = height_cm / 100.0
    return lbm / h**2 + 6.1 * (1.8 - h)


def navy_bodyfat(waist_cm: float, neck_cm: float, height_cm: float) -> float:
    """海軍式 体脂肪率推定(男性)。体組成計とは独立した推定値。"""
    return 495 / (1.0324 - 0.19077 * np.log10(waist_cm - neck_cm)
                  + 0.15456 * np.log10(height_cm)) - 450


def e1rm(weight: float, reps: float, rir: float) -> float:
    """Epley + RIR補正。限界レップ数 r = reps + rir。"""
    return weight * (1 + (reps + rir) / 30.0)


def add_e1rm(wo: pd.DataFrame) -> tuple[pd.DataFrame, int, int]:
    """e1RM 列を付ける。計算できないセットは除外し、理由別の件数を返す。

    除外するのは (a) RIR 未記録 (b) 限界レップ数が E1RM_MAX_REPS 超。
    (b) を残すと Epley が推定1RMを過大評価し、時系列が種目内で比較できなくなる。
    """
    if not len(wo):
        return wo.assign(e1rm=pd.Series(dtype=float), muscle=pd.Series(dtype=object)), 0, 0
    df = wo.copy()
    df["muscle"] = df["exercise"].map(EXERCISE_MUSCLE).fillna("未分類")
    have = df[["weight_kg", "reps", "rir"]].notna().all(axis=1)
    r = df["reps"] + df["rir"]
    usable = have & (r <= E1RM_MAX_REPS)
    df["e1rm"] = np.where(usable, e1rm(df["weight_kg"], df["reps"], df["rir"]), np.nan)
    return df, int((~have).sum()), int((have & (r > E1RM_MAX_REPS)).sum())


def e1rm_trend(wo: pd.DataFrame, exercise: str, asof: pd.Timestamp) -> dict:
    """種目ごとに、日別最大 e1RM の回帰傾き(kg/週)を出す。"""
    d = wo[wo["exercise"] == exercise]
    d = window(d, asof, E1RM_WINDOW_DAYS)[["date", "e1rm"]].dropna()
    if not len(d):
        return {"latest": None, "slope_week": None, "n_sessions": 0}
    daily_max = d.groupby("date", as_index=False)["e1rm"].max()
    s, n = slope_per_day(daily_max, "e1rm")
    return {"latest": float(daily_max.sort_values("date")["e1rm"].iloc[-1]),
            "slope_week": s * 7 if s is not None else None,
            "n_sessions": n if n else len(daily_max)}


def weekly_volume(wo: pd.DataFrame, asof: pd.Timestamp) -> pd.DataFrame:
    """分析4: 部位別の有効セット数(RIR<=4)とトン数。直近7日 vs その前7日。"""
    def agg(df):
        eff = df[(df["rir"].isna()) | (df["rir"] <= 4)].copy()
        if not len(eff):
            return pd.DataFrame({"muscle": pd.Series(dtype=object),
                                 "sets": pd.Series(dtype=float),
                                 "tonnage": pd.Series(dtype=float)})
        eff["load"] = eff["weight_kg"] * eff["reps"]
        return (eff.groupby("muscle")
                   .agg(sets=("muscle", "size"), tonnage=("load", "sum"))
                   .reset_index())

    cur = agg(window(wo, asof, 7))
    prev = agg(window(wo, asof - timedelta(days=7), 7))
    m = cur.merge(prev, on="muscle", how="outer", suffixes=("", "_prev")).fillna(0)
    return m.sort_values("sets", ascending=False)


def next_contest(cfg: dict, asof: pd.Timestamp) -> dict | None:
    """次の大会と、そこまでの残り週数を返す。"""
    for c in cfg.get("contests", []):
        end = pd.Timestamp(c["date"] + "-01") + pd.offsets.MonthEnd(0)
        if end >= asof:
            return {**c, "end": end, "weeks": (end - asof).days / 7.0}
    return None


def nutrition_targets(cfg: dict, bw: float, bf: float | None, goal: float,
                      tdee: float | None) -> tuple[str, float, float, float | None]:
    """フェーズと体脂肪率から PFC 目標を出す。

    タンパク質は減量が深くなるほど上げる（LBM 保護）。脂質はホルモン維持の下限。
    炭水化物は残余で決まる従属変数だが、これが枯れるとトレーニングの質が落ちるため
    下限を割ったら「摂取を削る」ではなく「消費側で赤字を作る」判断に回す。
    """
    n = cfg["nutrition"]
    if goal > 0:
        key = "bulk"
    elif bf is not None and bf < n.get("deep_cut_bf_threshold", 13.0):
        key = "deep_cut"
    else:
        key = "cut"
    d = n[key]
    prot = bw * d["protein_g_per_kg"]
    fat = bw * d["fat_g_per_kg"]
    carb = None
    if tdee is not None:
        intake = tdee + goal * KCAL_PER_KG_FAT / 7
        # P と F だけで摂取枠を超えることがある。負の目標は指示にならないので
        # 0 で止める。この時点で炭水化物の下限警告が出る
        carb = max(0.0, (intake - prot * 4 - fat * 9) / 4)
    return key, prot, fat, carb


def vol_range(cfg: dict, muscle: str) -> tuple[int, int]:
    """部位別の MEV/MRV。細分化した部位は一括の 10-20 では判定できないため個別に持つ。"""
    v = cfg.get("volume_sets_per_muscle", {})
    d = v.get(muscle) or v.get("default") or {"min": 10, "max": 20}
    return int(d["min"]), int(d["max"])


def phase_of(cfg: dict, asof: pd.Timestamp) -> dict:
    for p in cfg["phases"]:
        if pd.Timestamp(p["from"]) <= asof <= pd.Timestamp(p["to"]):
            return p
    return {"name": "計画期間外", "goal_kg_per_week": 0.0}


# ---------- レポート ----------

def fmt(v, spec="+.2f", none="判定不可"):
    return none if v is None else format(v, spec)


def build_report(cfg, daily, wo, measures, plan, asof) -> tuple[str, list[str]]:
    L: list[str] = []
    alerts: list[str] = []
    phase = phase_of(cfg, asof)
    goal = float(phase["goal_kg_per_week"])

    L.append(f"# 週次レポート {asof.date()}")
    L.append("")
    L.append(f"**フェーズ**: {phase['name']} / 目標ペース {goal:+.2f} kg/週")
    L.append("")

    # --- 1. 体重トレンド ---
    L.append("## 1. 体重トレンド")
    w7 = window(daily, asof, 7)["weight_kg"].dropna()
    w28 = window(daily, asof, 28)
    slope_w, n_w = slope_per_day(window(daily, asof, TREND_WINDOW_DAYS), "weight_kg")
    slope_week = slope_w * 7 if slope_w is not None else None
    prev7 = window(daily, asof - timedelta(days=7), 7)["weight_kg"].dropna()
    L.append("")
    L.append("| 指標 | 値 |")
    L.append("|---|---|")
    L.append(f"| 7日平均体重 | {fmt(float(w7.mean()) if len(w7) else None, '.2f')} kg (n={len(w7)}) |")
    if len(prev7):
        L.append(f"| 前週7日平均 | {float(prev7.mean()):.2f} kg |")
    L.append(f"| トレンド({TREND_WINDOW_DAYS}日回帰) | {fmt(slope_week)} kg/週 (n={n_w}) |")
    L.append(f"| 目標との乖離 | {fmt(slope_week - goal if slope_week is not None else None)} kg/週 |")
    bf7 = window(daily, asof, 7)["bodyfat_pct"].dropna()
    if len(bf7):
        bslope, _ = slope_per_day(w28, "bodyfat_pct")
        L.append(f"| 体脂肪率7日平均 | {float(bf7.mean()):.1f} % (傾き {fmt(bslope*7*4 if bslope is not None else None)} %/月) |")
    lbm_now = None
    if len(w7) and len(bf7):
        lbm_now = float(w7.mean()) * (1 - float(bf7.mean()) / 100.0)
        L.append(f"| 除脂肪体重(LBM) | {lbm_now:.1f} kg |")
        if cfg.get("height_cm"):
            L.append(f"| **正規化FFMI** | **{norm_ffmi(lbm_now, cfg['height_cm']):.1f}** "
                     "(20-21=明らかに鍛えている / 22-23=ジムで目立つ) |")
    L.append("")

    if slope_week is not None:
        dev = slope_week - goal
        if abs(goal) >= 0.2 and abs(slope_week) < 0.1:
            kcal_w = window(daily, asof, 14)["kcal"].dropna()
            cv = float(kcal_w.std() / kcal_w.mean() * 100) if len(kcal_w) > 2 and kcal_w.mean() else None
            if cv is not None and cv < 10:
                alerts.append(f"**停滞検知**: 体重が横ばい (傾き {slope_week:+.2f} kg/週) で摂取も安定 (CV {cv:.1f}%)。"
                              + ("摂取 -150kcal または 歩数 +2000/日。まず歩数の推移を確認する。" if goal < 0
                                 else "摂取 +200kcal。"))
            else:
                alerts.append(f"**停滞の疑い**: 体重は横ばいだが摂取のばらつきが大きい"
                              f"(CV {fmt(cv, '.1f')}%)。摂取を動かす前に記録精度を確認する。")
        elif abs(dev) > 0.25:
            alerts.append(f"**ペース乖離**: 実測 {slope_week:+.2f} kg/週 vs 目標 {goal:+.2f} kg/週 "
                          f"(差 {dev:+.2f})。摂取 {-dev*KCAL_PER_KG_FAT/7:+.0f} kcal/日 が理論値。")

    bw = float(w7.mean()) if len(w7) else float(cfg["baseline"]["weight_kg"])
    actual_bf = float(bf7.mean()) if len(bf7) else None

    # --- 1.5 大会カウントダウン ---
    ct = next_contest(cfg, asof)
    if ct:
        L.append(f"## 次の大会まで {ct['weeks']:.0f}週 — {ct['date']} {ct['category']}")
        L.append("")
        L.append(f"目標ステージ体脂肪率 **{ct['target_bf']}%** / 位置づけ: {ct['goal']}")
        L.append("")
        if actual_bf is not None and lbm_now is not None and ct["weeks"] > 0:
            # 目標BFに必要な体重と、残り週数から逆算した必要ペース
            need_w = lbm_now / (1 - ct["target_bf"] / 100)
            cur_w = float(w7.mean())
            need_pace_pct = (cur_w - need_w) / cur_w * 100 / ct["weeks"]
            L.append("| 指標 | 値 |")
            L.append("|---|---|")
            L.append(f"| 現在 | {cur_w:.1f} kg / BF {actual_bf:.1f}% |")
            L.append(f"| ステージ体重の目安 | {need_w:.1f} kg (LBM維持前提) |")
            L.append(f"| 必要な減量 | {cur_w - need_w:+.1f} kg |")
            L.append(f"| **必要ペース** | **{need_pace_pct:+.2f} %/週** |")
            if need_pace_pct > 0.7:
                alerts.append(f"**大会までのペースが速すぎる**: 残り{ct['weeks']:.0f}週で "
                              f"{need_pace_pct:.2f}%/週 が必要（安全域は0.7%/週まで）。"
                              "このまま追うと LBM を失う。減量開始を前倒しするか、"
                              "ステージ体脂肪率の目標を緩める。")
            elif need_pace_pct < 0:
                L.append("")
                L.append("既に目標体脂肪率を下回っている。増量に切り替えるか、"
                         "この状態を維持して筋量を積む余地がある。")
        L.append("")

    # --- 2. 当月目標との進捗 ---
    L.append("## 2. 当月目標との進捗")
    L.append("")
    ym = asof.strftime("%Y-%m")
    row = plan[plan["month"] == ym] if plan is not None and len(plan) else None
    if row is None or not len(row):
        L.append(f"{ym} の月次目標が `data/monthly_plan.csv` に無い"
                 "（`python3 reference/analysis/make_monthly_plan.py` で再生成する）。")
    else:
        t = row.iloc[0]
        L.append("月次目標は月末時点の到達値。計画の生成は make_monthly_plan.py。")
        L.append("")
        L.append("| 指標 | 実測 | 当月目標 | 差 |")
        L.append("|---|---|---|---|")
        for label, act, tgt, unit in (
                ("体重", float(w7.mean()) if len(w7) else None, float(t["weight_kg"]), "kg"),
                ("体脂肪率", actual_bf, float(t["bodyfat_pct"]), "%"),
                ("LBM", lbm_now, float(t["lbm_kg"]), "kg"),
                ("正規化FFMI", norm_ffmi(lbm_now, cfg["height_cm"]) if (lbm_now and cfg.get("height_cm")) else None,
                 float(t["ffmi"]), "")):
            u = f" {unit}" if unit else ""
            if act is None:
                L.append(f"| {label} | 記録なし | {tgt:.1f}{u} | — |")
            else:
                d = act - tgt
                L.append(f"| {label} | {act:.1f}{u} | {tgt:.1f}{u} | {0.0 if abs(d) < 0.05 else d:+.1f} |")
        for label, ex, col in (("ベンチ e1RM", "バーベルベンチプレス", "bench_e1rm"),
                               ("スクワット e1RM", "バーベルスクワット", "squat_e1rm")):
            tr = e1rm_trend(wo, ex, asof)
            tgt = float(t[col])
            if tr["latest"] is None:
                L.append(f"| {label} | 記録なし | {tgt:.0f} kg | — |")
            else:
                L.append(f"| {label} | {tr['latest']:.1f} kg | {tgt:.0f} kg | {tr['latest'] - tgt:+.1f} |")
        if lbm_now is not None and lbm_now - float(t["lbm_kg"]) < -1.0:
            alerts.append(f"**LBM が月次目標を {float(t['lbm_kg']) - lbm_now:.1f}kg 下回る**。"
                          "計画の前提が崩れているので、`reference/analysis/make_monthly_plan.py` の "
                          "パラメータを実測値で引き直す。目標に体を合わせるのではなく、目標を実測に合わせる。")
    L.append("")

    # --- 3. TDEE と推奨摂取 ---
    L.append("## 3. 推定TDEE と 来週の推奨摂取")
    t = estimate_tdee(daily, asof)
    L.append("")
    if t["tdee"] is None:
        L.append(f"データ不足で推定不可（摂取記録 {t['n_kcal']}日 / 必要 {TDEE_MIN_DAYS}日、"
                 f"体重記録 {t['n_weight']}日 / 必要 4日）。")
        alerts.append(f"**記録不足**: TDEE が推定できない。これが無いと摂取量の意思決定ができない。"
                      f"直近{TREND_WINDOW_DAYS}日で摂取{t['n_kcal']}日分しか記録が無い。")
    else:
        rec = t["tdee"] + goal * KCAL_PER_KG_FAT / 7
        L.append("| 指標 | 値 |")
        L.append("|---|---|")
        L.append(f"| 平均摂取 (直近{t['window']}日) | {t['mean_kcal']:.0f} kcal |")
        L.append(f"| 体重トレンド | {t['weight_slope_week']:+.2f} kg/週 |")
        L.append(f"| **推定TDEE** | **{t['tdee']:.0f} kcal** |")
        floor = bw * FLOOR_KCAL_PER_KG
        capped = max(rec, floor) if goal < 0 else rec
        L.append(f"| **来週の推奨摂取** | **{capped:.0f} kcal/日** |")
        L.append(f"| 現在の摂取との差 | {capped - t['mean_kcal']:+.0f} kcal/日 |")
        if goal < 0 and rec < floor:
            L.append(f"| 摂取下限 (体重×{FLOOR_KCAL_PER_KG:.0f}) | {floor:.0f} kcal — "
                     f"理論値 {rec:.0f} kcal はこれを下回るため下限を採用 |")
            alerts.append(
                f"**摂取の下限に到達**: 目標ペースに必要な理論値 {rec:.0f} kcal は下限 {floor:.0f} kcal を下回る。"
                "ここから先は摂取を削らず、消費側（歩数・有酸素）でつくる。"
                "削り続けると LBM 損失と代謝適応が進み、増量期の立ち上がりが悪化する。")
    L.append("")

    # --- 4. PFC ---
    L.append("## 4. PFC 目標と達成率 (直近7日平均)")
    d7 = window(daily, asof, 7)
    key, p_t, f_t, c_t = nutrition_targets(cfg, bw, actual_bf, goal, t["tdee"])
    label_map = {"cut": "減量期", "deep_cut": "減量期(BF13%未満・タンパク質を上げる)", "bulk": "増量期"}
    L.append("")
    L.append(f"適用モード: **{label_map[key]}** "
             f"(P {cfg['nutrition'][key]['protein_g_per_kg']}g/kg, "
             f"F {cfg['nutrition'][key]['fat_g_per_kg']}g/kg, C は残余)")
    L.append("")
    L.append("| 栄養素 | 実績 | 目標 | 判定 |")
    L.append("|---|---|---|---|")
    pfc_rows: tuple[tuple[str, str, float | None, str], ...] = (
        ("protein_g", "タンパク質", p_t, "min"),
        ("fat_g", "脂質", f_t, "min"),
        ("carb_g", "炭水化物", c_t, "range"),
    )
    for col, lab, target, kind in pfc_rows:
        v = d7[col].dropna()
        intake: float | None = float(v.mean()) if len(v) else None
        if target is None:
            L.append(f"| {lab} | {fmt(intake, '.0f', '記録なし')} g | TDEE未推定のため算出不可 | — |")
            continue
        if intake is None:
            L.append(f"| {lab} | 記録なし | {target:.0f} g | — |")
            continue
        if kind == "min":
            ok = intake >= target * 0.9
            L.append(f"| {lab} | {intake:.0f} g | {target:.0f} g 以上 | {'✓' if ok else '⚠ 不足'} |")
            if not ok:
                alerts.append(f"**{lab}不足**: {intake:.0f}g / 目標 {target:.0f}g。"
                              + ("減量中の LBM 損失が増える。" if col == "protein_g"
                                 else "ホルモン維持の下限を割っている。"))
        else:
            ok = abs(intake - target) <= target * 0.15
            L.append(f"| {lab} | {intake:.0f} g | {target:.0f} g | {'✓' if ok else '△ 乖離'} |")
    # 炭水化物の下限チェック: 目標値そのものが低すぎる場合は赤字の作り方の問題
    cmin = cfg["nutrition"].get("carb_min_g", 200)
    if c_t is not None and c_t < cmin:
        alerts.append(f"**炭水化物の目標が {c_t:.0f}g（下限 {cmin}g）まで下がっている**。"
                      f"週{cfg.get('max_exercises_per_session', 4)}種目×6日のボリュームを支えられない。"
                      "摂取をこれ以上削らず、歩数を上げて消費側で赤字を作る。"
                      f"歩数 +3000/日 で約{cfg['nutrition'].get('extra_steps_kcal',200)}kcal = 炭水化物 50g 分。")
    L.append("")

    # --- 5. 部位別ボリューム ---
    L.append("## 5. 部位別 有効セット数 (直近7日)")
    vol = weekly_volume(wo, asof)
    L.append("")
    if not len(vol):
        L.append("トレーニング記録なし。")
    else:
        L.append("| 部位 | セット | 前週 | トン数 | MEV-MRV | 判定 |")
        L.append("|---|---|---|---|---|---|")
        excl = set(cfg.get("volume_alert_exclude", []))
        over, under = [], []
        for _, r in vol.iterrows():
            n = int(r["sets"])
            lo, hi = vol_range(cfg, r["muscle"])
            if n < lo:
                judge = "⚠ MEV以下" + ("（意図的に許容）" if r["muscle"] in excl else "")
                if r["muscle"] not in excl:
                    under.append(f"{r['muscle']}{n}")
            elif n > hi:
                judge = "⚠ MRV超"
                over.append(f"{r['muscle']}{n}")
            else:
                judge = "✓"
            L.append(f"| {r['muscle']} | {n} | {int(r['sets_prev'])} | {r['tonnage']:.0f} kg | "
                     f"{lo}-{hi} | {judge} |")
        if over:
            alerts.append(f"**MRV超（回復不足）: {' / '.join(over)}**。"
                          "ボリュームが足りないのではなく多すぎる状態。"
                          "この部位は削って、MEV以下の部位に振り替える。総量を増やしてはいけない。")
        if under and not over:
            alerts.append(f"**MEV以下（刺激不足）: {' / '.join(under)}**。"
                          "この量では筋力の維持しか起こらない。")
    trained = set(vol["muscle"]) if len(vol) else set()
    lower_sets = int(vol[vol["muscle"].isin(LOWER_BODY)]["sets"].sum()) if len(vol) else 0
    if lower_sets < 10:
        alerts.append(f"**下半身のボリュームが週 {lower_sets} セット**（大腿四頭・ハム・臀部・ふくらはぎの合計）。"
                      "スクワット1RM 100kg が挙がる筋力はあるが、MEV（部位あたり週10セット）を"
                      "下回っており筋肥大の刺激が入っていない。フォームは成立しているので、"
                      "脚の日（週1回・4種目）で大腿四頭12セット、"
                      "ルーマニアンデッドリフトを背中の日に置いてハム8セットを確保する。"
                      "部位別の基準は config の volume_sets_per_muscle を参照。")
    if "未分類" in trained:
        alerts.append("**種目名が部位マスタに無い**（未分類として集計）。"
                      "`reference/analysis/analyze.py` の EXERCISE_MUSCLE に追加する。")
    L.append("")

    # --- 6. e1RM ---
    L.append(f"## 6. 主要種目の推定1RM (直近{E1RM_WINDOW_DAYS}日)")
    L.append("")
    rows = []
    for ex in KEY_EXERCISES:
        tr = e1rm_trend(wo, ex, asof)
        if tr["latest"] is not None:
            rows.append((ex, tr))
    if not rows:
        L.append("RIR付きの記録が無いため推定不可。RIR を記録しないと進捗が測れない。")
    else:
        L.append("| 種目 | 推定1RM | 傾き | セッション数 | 判定 |")
        L.append("|---|---|---|---|---|")
        for ex, tr in rows:
            sl = tr["slope_week"]
            if sl is None:
                judge = "データ不足"
            elif goal < 0:
                judge = "✓ 維持" if sl > -0.3 else "⚠ 低下"
            else:
                judge = "✓ 伸長" if sl > 0.1 else "⚠ 停滞"
            L.append(f"| {ex} | {tr['latest']:.1f} kg | {fmt(sl)} kg/週 | {tr['n_sessions']} | {judge} |")
            if sl is not None and goal < 0 and sl <= -0.3:
                alerts.append(f"**筋力低下**: {ex} が {sl:+.2f} kg/週。減量ペースを -0.25kg/週 に緩める。")
            if sl is not None and goal > 0 and sl <= 0:
                alerts.append(f"**増量期に {ex} が伸びていない** ({sl:+.2f} kg/週)。"
                              "ボリュームか回復か摂取のどれが足りていないかを切り分ける。")
    L.append("")

    # --- 7. リカバリと周囲長 ---
    L.append("## 7. リカバリ / 周囲長")
    L.append("")
    sl7 = d7["sleep_h"].dropna()
    fa7 = d7["fatigue"].dropna()
    st7 = d7["steps"].dropna()
    st_prev = window(daily, asof - timedelta(days=7), 7)["steps"].dropna()
    L.append("| 指標 | 値 |")
    L.append("|---|---|")
    L.append(f"| 睡眠 7日平均 | {fmt(float(sl7.mean()) if len(sl7) else None, '.1f')} h |")
    L.append(f"| 疲労 7日平均 | {fmt(float(fa7.mean()) if len(fa7) else None, '.1f')} / 5 |")
    L.append(f"| 歩数 7日平均 | {fmt(float(st7.mean()) if len(st7) else None, '.0f')} "
             f"(前週 {fmt(float(st_prev.mean()) if len(st_prev) else None, '.0f')}) |")
    # --- Apple Watch 由来の客観指標（装着時のみ）---
    d30 = window(daily, asof, 30)
    hrv7, hrv30 = col_series(d7, "hrv_ms"), col_series(d30, "hrv_ms")
    rhr7, rhr30 = col_series(d7, "resting_hr"), col_series(d30, "resting_hr")
    deep7 = col_series(d7, "deep_sleep_min")
    if len(hrv7) >= 4 and len(hrv30) >= 14:
        base, cur = float(hrv30.mean()), float(hrv7.mean())
        ratio = cur / base if base else 1.0
        L.append(f"| HRV 7日平均 | {cur:.0f} ms (30日基準 {base:.0f} / {(ratio-1)*100:+.1f}%) |")
        if ratio < 0.85:
            alerts.append(f"**HRV が基準から {(1-ratio)*100:.0f}% 低下** ({cur:.0f}ms / 基準 {base:.0f}ms)。"
                          "自律神経の疲労は体感より早く出る。回復週を前倒しするか、"
                          "減量中ならペースを1段落とす。")
    if len(rhr7) >= 4 and len(rhr30) >= 14:
        base, cur = float(rhr30.mean()), float(rhr7.mean())
        L.append(f"| 安静時心拍 7日平均 | {cur:.0f} bpm (30日基準 {base:.0f} / {cur-base:+.1f}) |")
        if cur - base > 5:
            alerts.append(f"**安静時心拍が基準比 {cur-base:+.0f}bpm**。疲労・体調不良・回復不足のサイン。"
                          "HRV と合わせて判断し、両方が悪化していれば回復を優先する。")
    if len(deep7):
        v = float(deep7.mean())
        L.append(f"| 深睡眠 7日平均 | {v:.0f} 分 |")
        if v < 60:
            alerts.append(f"**深睡眠が7日平均 {v:.0f}分**（目安60分）。"
                          "睡眠時間が足りていても質が低い。減量ペースより睡眠を優先する。")

    if len(sl7) and float(sl7.mean()) < 6.5:
        alerts.append(f"**睡眠不足**: 7日平均 {float(sl7.mean()):.1f}h。筋力低下と停滞の主要因になる。")
    if len(fa7) and float(fa7.mean()) > 3.5:
        alerts.append(f"**疲労蓄積**: 7日平均 {float(fa7.mean()):.1f}/5。回復週を前倒しする。")
    if len(st7) and len(st_prev) and float(st_prev.mean()) > 0:
        drop = float(st7.mean()) - float(st_prev.mean())
        if drop < -1500 and goal < 0:
            alerts.append(f"**歩数が前週比 {drop:+.0f}**。減量停滞の主犯はほぼこれ。摂取を削る前に歩数を戻す。")

    if len(measures):
        m = measures.sort_values("date").iloc[-1]
        L.append("")
        L.append(f"直近計測 {pd.Timestamp(m['date']).date()}:")
        L.append("")
        L.append("| 部位 | cm | 前回差 |")
        L.append("|---|---|---|")
        prev_m = measures.sort_values("date").iloc[-2] if len(measures) > 1 else None
        for col, label in (("shoulder_cm", "肩"), ("chest_cm", "胸"), ("waist_navel_cm", "ウエスト"),
                           ("arm_r_cm", "上腕"), ("thigh_r_cm", "大腿"), ("neck_cm", "首")):
            if col not in measures.columns or pd.isna(m.get(col)):
                continue
            diff = ""
            if prev_m is not None and not pd.isna(prev_m.get(col)):
                diff = f"{float(m[col]) - float(prev_m[col]):+.1f}"
            L.append(f"| {label} | {float(m[col]):.1f} | {diff} |")
        if not pd.isna(m.get("shoulder_cm")) and not pd.isna(m.get("waist_navel_cm")):
            ratio = float(m["shoulder_cm"]) / float(m["waist_navel_cm"])
            L.append("")
            L.append(f"**肩/ウエスト比 = {ratio:.3f}** "
                     f"({'✓ V字が成立するレンジ' if ratio >= 1.60 else '目標 1.60 以上'})")
        h = cfg.get("height_cm")
        if h and not pd.isna(m.get("waist_navel_cm")) and not pd.isna(m.get("neck_cm")):
            nb = navy_bodyfat(float(m["waist_navel_cm"]), float(m["neck_cm"]), float(h))
            L.append("")
            L.append(f"海軍式 推定体脂肪率: **{nb:.1f}%**（体組成計とは独立した推定。両者が同方向なら信頼度が上がる）")
        elif not h:
            L.append("")
            L.append("> `config.json` の `height_cm` が未設定のため海軍式推定はスキップ。")
    L.append("")

    # --- 記録の欠損チェック ---
    miss_w = 7 - len(window(daily, asof, 7)["weight_kg"].dropna())
    miss_k = 7 - len(window(daily, asof, 7)["kcal"].dropna())
    if miss_w >= 2 or miss_k >= 2:
        alerts.append(f"**記録欠損**: 直近7日で体重 {miss_w}日 / 摂取 {miss_k}日 が未記録。"
                      "分析の前提が崩れる。摂取を動かす判断より記録の復旧が先。")

    # --- 8. アクション ---
    L.append("## 8. 今週のアクション")
    L.append("")
    if not alerts:
        L.append("計画どおり。変更なし。現在の摂取とボリュームを継続する。")
    else:
        for i, a in enumerate(alerts, 1):
            L.append(f"{i}. {a}")
    L.append("")
    return "\n".join(L), alerts


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--asof", default=str(date.today()), help="基準日 YYYY-MM-DD")
    ap.add_argument("--data-dir", default="private/data")
    ap.add_argument("--out-dir", default="private/reports")
    ap.add_argument("--config", default="private/config.json")
    ap.add_argument("--no-write", action="store_true", help="ファイル出力せず標準出力のみ")
    a = ap.parse_args()

    asof = pd.Timestamp(datetime.strptime(a.asof, "%Y-%m-%d").date())
    cfg = load_config(Path(a.config))
    dd = Path(a.data_dir)
    daily = read_table(dd / "daily.csv")
    wo_raw = read_table(dd / "workouts.csv")
    measures = read_table(dd / "measures.csv")
    wo, no_rir, high_rep = add_e1rm(wo_raw)
    plan_path = dd / "monthly_plan.csv"
    plan = pd.read_csv(plan_path) if plan_path.exists() else None

    report, alerts = build_report(cfg, daily, wo, measures, plan, asof)
    notes = []
    if no_rir:
        notes.append(f"RIR 未記録 {no_rir} セット")
    if high_rep:
        notes.append(f"限界レップ {E1RM_MAX_REPS} 超 {high_rep} セット")
    if notes:
        report += (f"\n> 注: {' / '.join(notes)} は e1RM 計算から除外した。"
                   "高レップ種目（サイドレイズ・フライ等）は Epley の推定が崩れるため"
                   "e1RM の追跡対象外であり、これは仕様。\n")

    print(report)
    if not a.no_write:
        out = Path(a.out_dir)
        out.mkdir(parents=True, exist_ok=True)
        p = out / f"{asof.date()}_weekly.md"
        p.write_text(report, encoding="utf-8")
        print(f"\n[出力] {p}  / アラート {len(alerts)}件")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
