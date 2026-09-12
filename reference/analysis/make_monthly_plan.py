#!/usr/bin/env python3
"""月次目標テーブルを生成する（個人の計画なので private/ に出力する）。

身体データ（身長・体重・体脂肪率・挙上重量）は config から読む。
計画のブロック構成は BLOCKS で定義する。

    python3 reference/analysis/make_monthly_plan.py                          # private/config.json を使う
    python3 reference/analysis/make_monthly_plan.py --config config.example.json \
        --out-docs /tmp/plan.md --out-csv /tmp/plan.csv            # 雛形で試す

P1 完了時点と各大会後の実測 LBM を起点に、以降は必ず再計算する。
月次目標は固定値ではなく、実測を起点に引き直すもの。
"""
import argparse
import json
from pathlib import Path

# 年次サイクル（案B）: 毎年夏の初心者向け大会（BBJ / サマスタのスタイリッシュガイ級、
# ステージ体脂肪率 11%）に出場する。BF9.5% の深い仕上げをやめ、以下2点で LBM を稼ぐ。
#   1. 仕上げを BF11% にする → LBM損失 -0.98 → -0.69kg、仕上げ期間 4 → 3ヶ月
#   2. 増量ペースを +0.2 → +0.15kg/週 に落とす → 脂肪の増加が遅く、BF上限までの期間が伸びる
#      （+0.2kg/週 は BF15% に5.7ヶ月で到達し積めるLBMは+1.48kg、+0.15kg/週 なら8.2ヶ月で+1.80kg）
BLOCKS = [
    ("P0 基盤",      1, +0.15, 21.2, +1.5,  +3.0,
     "**食事は変えない**。今の摂取をそのまま記録してTDEEと習慣的摂取量を把握する。"
     "タンパク質だけ体重×2.0gを確保。脚を週16セットへ漸増(8→12→16)・記録習慣の確立"),
    # P1 は段階的減量（tapered cut）。体脂肪率が高いうちは速く、下がるにつれて減速する。
    ("P1-A カット 1.0%/週", 1, -0.35, 18.1, -0.5, +2.0,
     "最も速いフェーズ。月2.9kg。BF が高い今しかこのペースは使えない"),
    ("P1-B カット 0.8%/週", 1, -0.28, 15.8, -0.5, +2.0,
     "月2.3kg。ここから炭水化物を確保するため赤字の一部を歩数で作る"),
    ("P1-C カット 0.6%/週", 1, -0.25, 13.3, +0.0, +3.0,
     "月2.3kg。腹筋の輪郭が出始める"),
    ("P1-D カット 0.45%/週", 2, -0.16, 11.2, +0.0, +3.0,
     "月1.0kg。**映える体の達成**。ここで写真のベースラインを撮る"),
    ("Y1 ダイエットブレイク", 1, +0.20, 11.5, +0.5, +1.0,
     "メンテナンスカロリーに戻す。代謝適応とホルモンの回復。仕上げを新しい代謝基準から始める"),
    ("Y1 仕上げ",    2, -0.08, 11.0, -0.3,  -0.5,
     "★第1回大会 2027-05。BF11.5→11%はほぼ維持で足りる。完走が目的"),
    ("Y1 増量",     10, +0.22, 15.5, +1.9,  +3.2,
     "**下半身の初期適応が最大の10ヶ月**。+0.15kg/週。3年計画で最も LBM を稼げる期間"),
    ("Y2 仕上げ",    3, -0.23, 11.0, -0.7,  -1.5,
     "★第2回大会 2028-06。BF15.5→11%で3ヶ月。ここから入賞を狙える体になる"),
    ("Y2 増量",     10, +0.19, 15.5, +1.6,  +2.5,
     "伸びは鈍り始める。弱点部位に配分を寄せる。肩/ウエスト比を毎月確認"),
    ("Y3 仕上げ",    3, -0.23, 11.0, -0.7,  -1.5,
     "★第3回大会 2029-07。3年計画の集大成"),
    ("オフ",         2, +0.15, 12.0, +1.0,  +1.5,
     "大会後2週はメンテナンスカロリー。その後 次サイクルの増量へ"),
]

def norm_ffmi(lbm: float, height_m: float) -> float:
    return lbm / height_m**2 + 6.1 * (1.8 - height_m)


def month_labels(start_y: int = 2026, start_m: int = 9, n: int = 37) -> list[str]:
    out, y, m = [], start_y, start_m
    for _ in range(n):
        out.append(f"{y}-{m:02d}")
        m += 1
        if m == 13:
            y, m = y + 1, 1
    return out


def main() -> None:
    ap = argparse.ArgumentParser()
    ap.add_argument("--config", default="private/config.json")
    ap.add_argument("--out-docs", default="private/docs/02-月次目標.md")
    ap.add_argument("--out-csv", default="private/data/monthly_plan.csv")
    a = ap.parse_args()

    cfg = json.loads(Path(a.config).read_text(encoding="utf-8"))
    height_m = cfg["height_cm"] / 100
    base = cfg["baseline"]
    start_lbm = base["weight_kg"] * (1 - base["bodyfat_pct"] / 100)
    start_bench = float(base["bench_1rm_kg"])
    start_squat = float(base.get("squat_1rm_kg", base["bench_1rm_kg"]))

    rows = []
    lbm, bf, bench, squat = start_lbm, base["bodyfat_pct"], start_bench, start_squat
    labels = month_labels()
    i = 0
    for name, months, dlbm, bf_end, dbench, dsquat, focus in BLOCKS:
        bf_step = (bf_end - bf) / months
        for k in range(months):
            lbm += dlbm
            bf += bf_step
            bench += dbench
            squat += dsquat
            rows.append({
                "month": labels[i], "phase": name, "lbm": lbm, "bf": bf,
                "weight": lbm / (1 - bf / 100), "ffmi": norm_ffmi(lbm, height_m),
                "bench": bench, "squat": squat,
                "focus": focus if k == 0 else "",
            })
            i += 1

    L = ["# 月次目標値", "",
         f"身長 {cfg['height_cm']}cm / 起点 {cfg['start_date']} "
         f"({base['weight_kg']}kg・BF{base['bodyfat_pct']}%・LBM {start_lbm:.1f}kg・"
         f"ベンチ1RM {start_bench:.0f}kg・スクワット1RM {start_squat:.0f}kg)", "",
         "**この表は固定目標ではない。** 体重と体脂肪率は LBM の設計値から逆算した従属変数であり、",
         "実測とズレたら本スクリプトのパラメータを更新して引き直す。", "",
         "追う順序は **LBM → 体脂肪率 → 体重**。体重は最後に決まる数字で、目標ではない。", "",
         "## フェーズ別サマリ", "",
         "| フェーズ | 期間 | 月数 | 体重 | 体脂肪率 | LBM | 重点 |",
         "|---|---|---|---|---|---|---|"]

    idx = 0
    for name, months, *_, focus in BLOCKS:
        seg = rows[idx:idx + months]
        idx += months
        L.append(f"| {name} | {seg[0]['month']} 〜 {seg[-1]['month']} | {months} | "
                 f"{seg[-1]['weight']:.1f} kg | {seg[-1]['bf']:.1f} % | {seg[-1]['lbm']:.1f} kg | {focus} |")

    L += ["", "## 月次目標（各月末の到達目標）", "",
          "| 月 | フェーズ | 体重 | BF% | LBM | FFMI | ベンチ e1RM | スクワット e1RM |",
          "|---|---|---|---|---|---|---|---|"]
    for r in rows:
        L.append(f"| {r['month']} | {r['phase'].split()[0]} | {r['weight']:.1f} | {r['bf']:.1f} | "
                 f"{r['lbm']:.1f} | {r['ffmi']:.1f} | {r['bench']:.0f} | {r['squat']:.0f} |")

    f = rows[-1]
    L += ["", "## 3年後の到達点", "",
          "| 指標 | 起点 | 到達 | 差 |", "|---|---|---|---|",
          f"| 体重 | {base['weight_kg']} kg | {f['weight']:.1f} kg | {f['weight']-base['weight_kg']:+.1f} kg |",
          f"| 体脂肪率 | {base['bodyfat_pct']} % | {f['bf']:.1f} % | {f['bf']-base['bodyfat_pct']:+.1f} pt |",
          f"| LBM | {start_lbm:.1f} kg | {f['lbm']:.1f} kg | {f['lbm']-start_lbm:+.1f} kg |",
          f"| 正規化FFMI | {norm_ffmi(start_lbm, height_m):.1f} | {f['ffmi']:.1f} | "
          f"{f['ffmi']-norm_ffmi(start_lbm, height_m):+.1f} |",
          f"| ベンチ e1RM | {start_bench:.0f} kg | {f['bench']:.0f} kg | {f['bench']-start_bench:+.0f} kg |",
          f"| スクワット e1RM | {start_squat:.0f} kg | {f['squat']:.0f} kg | {f['squat']-start_squat:+.0f} kg |",
          "", "### 筋力の伸びと見た目の伸びは別物", "",
          f"ベンチは +{(f['bench']/start_bench-1)*100:.0f}% 伸びるが、"
          f"LBM は +{(f['lbm']/start_lbm-1)*100:.1f}% しか増えない。",
          "差分は神経系適応・技術・レバレッジ最適化によるもので、見た目には現れない。",
          "挙上重量はオーバーロードが効いている証拠として追う指標であり、見た目の目標そのものではない。", ""]

    out_docs, out_csv = Path(a.out_docs), Path(a.out_csv)
    out_docs.parent.mkdir(parents=True, exist_ok=True)
    out_csv.parent.mkdir(parents=True, exist_ok=True)
    out_docs.write_text("\n".join(L) + "\n", encoding="utf-8")
    out_csv.write_text(
        "month,phase,weight_kg,bodyfat_pct,lbm_kg,ffmi,bench_e1rm,squat_e1rm\n"
        + "".join(f"{r['month']},{r['phase']},{r['weight']:.1f},{r['bf']:.1f},"
                 f"{r['lbm']:.1f},{r['ffmi']:.1f},{r['bench']:.0f},{r['squat']:.0f}\n" for r in rows),
        encoding="utf-8")
    print(f"生成: {out_docs} / {out_csv} ({len(rows)}ヶ月分)")
    print(f"最終: {f['weight']:.1f}kg / BF{f['bf']:.1f}% / LBM {f['lbm']:.1f}kg / FFMI {f['ffmi']:.1f}")


if __name__ == "__main__":
    main()
