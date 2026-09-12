"""分析ロジックのテスト。

仕様は docs/03-分析ロジック.md に明文化されているため、そのままテストケースになる。
UI ではなく計算ロジックを純粋関数として切り出しているのはこのため。
"""
import numpy as np
import pandas as pd
import pytest

import analyze


class TestE1RM:
    """Epley + RIR補正: e1RM = weight × (1 + (reps + rir) / 30)"""

    def test_基準ケース(self):
        # 1RM 100kg の人が 75kg で 8回、余力2回 → 限界10回相当
        assert analyze.e1rm(75, 8, 2) == pytest.approx(100.0)

    def test_RIRが結果を変える(self):
        """同じ重量×レップでも RIR が違えば別の意味を持つ"""
        limit = analyze.e1rm(80, 8, 0)   # 限界8回
        spare = analyze.e1rm(80, 8, 2)   # 限界10回相当
        assert spare > limit
        assert limit == pytest.approx(101.33, abs=0.01)
        assert spare == pytest.approx(106.67, abs=0.01)

    def test_1RMは重量そのもの(self):
        assert analyze.e1rm(100, 1, 0) == pytest.approx(103.33, abs=0.01)


class TestNormFFMI:
    """正規化FFMI = LBM/身長m² + 6.1×(1.8 − 身長m)"""

    def test_基準ケース(self):
        assert analyze.norm_ffmi(60.0, 175.0) == pytest.approx(19.90, abs=0.01)

    def test_身長が低いほど同じLBMでも高く出る(self):
        assert analyze.norm_ffmi(64.0, 165) > analyze.norm_ffmi(64.0, 180)

    def test_身長の2乗ぶんのLBMでFFMIが1動く(self):
        """FFMI 1.0 に相当する LBM の差は 身長(m)^2 kg"""
        h = 1.75
        diff = analyze.norm_ffmi(60.0 + h**2, h * 100) - analyze.norm_ffmi(60.0, h * 100)
        assert diff == pytest.approx(1.0, abs=0.01)


class TestAddE1RM:
    """限界12レップ超は Epley の推定が崩れるため除外する"""

    def _df(self, rows):
        return pd.DataFrame(rows, columns=["date", "exercise", "set_no", "weight_kg", "reps", "rir"])

    def test_高レップは除外される(self):
        df = self._df([
            ("2026-10-01", "ベンチプレス", 1, 80, 5, 2),    # 限界7 → 採用
            ("2026-10-01", "サイドレイズ", 1, 10, 15, 2),   # 限界17 → 除外
        ])
        out, no_rir, high_rep = analyze.add_e1rm(df)
        assert high_rep == 1
        assert no_rir == 0
        assert out["e1rm"].notna().sum() == 1

    def test_RIR欠損は除外される(self):
        df = self._df([("2026-10-01", "ベンチプレス", 1, 80, 5, None)])
        out, no_rir, high_rep = analyze.add_e1rm(df)
        assert no_rir == 1
        assert out["e1rm"].isna().all()

    def test_未登録種目は未分類になる(self):
        df = self._df([("2026-10-01", "謎の種目", 1, 50, 8, 2)])
        out, _, _ = analyze.add_e1rm(df)
        assert out["muscle"].iloc[0] == "未分類"


class TestNavyBodyfat:
    def test_妥当な範囲に収まる(self):
        bf = analyze.navy_bodyfat(waist_cm=86, neck_cm=39, height_cm=175.0)
        assert 15 < bf < 25

    def test_ウエストが太いほど高く出る(self):
        slim = analyze.navy_bodyfat(76, 39, 175.0)
        wide = analyze.navy_bodyfat(90, 39, 175.0)
        assert wide > slim


class TestNutritionTargets:
    """フェーズと体脂肪率で PFC 目標が切り替わる"""

    CFG = {
        "nutrition": {
            "cut": {"protein_g_per_kg": 2.4, "fat_g_per_kg": 0.85},
            "deep_cut": {"protein_g_per_kg": 2.6, "fat_g_per_kg": 0.85},
            "bulk": {"protein_g_per_kg": 2.2, "fat_g_per_kg": 1.0},
            "deep_cut_bf_threshold": 13.0,
        }
    }

    def test_増量期はbulk(self):
        key, p, f, c = analyze.nutrition_targets(self.CFG, 70, 15.0, goal=0.15, tdee=2800)
        assert key == "bulk"
        assert p == pytest.approx(154.0)

    def test_減量期はcut(self):
        key, p, f, c = analyze.nutrition_targets(self.CFG, 70, 15.0, goal=-0.5, tdee=2800)
        assert key == "cut"
        assert p == pytest.approx(168.0)

    def test_体脂肪率13パーセント未満でタンパク質が上がる(self):
        key, p, _, _ = analyze.nutrition_targets(self.CFG, 70, 12.0, goal=-0.5, tdee=2800)
        assert key == "deep_cut"
        assert p == pytest.approx(182.0)

    def test_TDEE未推定なら炭水化物は算出不可(self):
        _, _, _, c = analyze.nutrition_targets(self.CFG, 70, 15.0, goal=-0.5, tdee=None)
        assert c is None

    def test_炭水化物は残余で決まる(self):
        _, p, f, c = analyze.nutrition_targets(self.CFG, 70, 15.0, goal=-0.5, tdee=2800)
        intake = 2800 + (-0.5) * 7700 / 7
        assert c == pytest.approx((intake - p * 4 - f * 9) / 4)


class TestVolRange:
    """MEV/MRV は部位別。間接刺激のある部位は直接種目の基準を下げる"""

    CFG = {
        "volume_sets_per_muscle": {
            "default": {"min": 10, "max": 20},
            "肩前部": {"min": 4, "max": 12},
        }
    }

    def test_部位別の値を返す(self):
        assert analyze.vol_range(self.CFG, "肩前部") == (4, 12)

    def test_未定義の部位はdefault(self):
        assert analyze.vol_range(self.CFG, "胸") == (10, 20)


class TestEstimateTDEE:
    """TDEE = 平均摂取kcal − 体重トレンド(kg/日) × 7700"""

    def _daily(self, n, kcal, daily_change):
        dates = pd.date_range("2026-10-01", periods=n, freq="D")
        return pd.DataFrame({
            "date": dates,
            "kcal": [kcal] * n,
            "weight_kg": [75.0 + daily_change * i for i in range(n)],
        })

    def test_体重横ばいならTDEEは摂取と一致(self):
        df = self._daily(21, 2800, 0.0)
        out = analyze.estimate_tdee(df, pd.Timestamp("2026-10-21"))
        assert out["tdee"] == pytest.approx(2800, abs=1)

    def test_減量中はTDEEが摂取を上回る(self):
        # -0.5kg/週 = -0.0714kg/日 → 約550kcal/日の不足
        df = self._daily(21, 2500, -0.5 / 7)
        out = analyze.estimate_tdee(df, pd.Timestamp("2026-10-21"))
        assert out["tdee"] == pytest.approx(2500 + 0.5 / 7 * 7700, abs=5)

    def test_記録不足なら推定しない(self):
        df = self._daily(5, 2800, 0.0)
        out = analyze.estimate_tdee(df, pd.Timestamp("2026-10-05"))
        assert out["tdee"] is None


class TestWeeklyVolume:
    """有効セットは RIR <= 4 のみ。ウォームアップを含めない"""

    def test_RIR5以上はウォームアップとして除外(self):
        df = pd.DataFrame({
            "date": pd.to_datetime(["2026-10-20"] * 4),
            "exercise": ["ベンチプレス"] * 4,
            "muscle": ["胸"] * 4,
            "weight_kg": [50, 70, 90, 50],
            "reps": [5, 3, 1, 5],
            "rir": [8, 3, 0, 8],          # ピラミッド: 50%はウォームアップ
            "e1rm": [np.nan] * 4,
        })
        out = analyze.weekly_volume(df, pd.Timestamp("2026-10-21"))
        assert int(out[out["muscle"] == "胸"]["sets"].iloc[0]) == 2


class TestColSeries:
    """Apple Watch 未導入時も落ちないこと"""

    def test_列が無ければ空Seriesを返す(self):
        df = pd.DataFrame({"date": [], "weight_kg": []})
        assert len(analyze.col_series(df, "hrv_ms")) == 0
