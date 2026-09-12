# reference/

**Phase 0 のプロトタイプ実装。本番の実行経路からは外れている。**

分析ロジックの正は Go（`api/internal/analytics`）に移した（[ADR-0011](../docs/adr/0011-go-analytics.md)）。ここに残しているのは2つの目的のため。

1. **Go 移植の検証基準。** 同一データに対して Go 実装の出力がこれと一致することを、受け入れ条件とする
2. **設計の検証過程の記録。** TDEE の逆算、Epley + RIR補正、部位別 MEV/MRV といった分析ロジックがどう検証されたかが残る

## 実行

```bash
pip install -r analysis/requirements-dev.txt
python3 analysis/make_sample_data.py
python3 analysis/analyze.py \
  --data-dir ../data/sample --config ../config.example.json \
  --asof 2026-10-31 --no-write
```

## テスト

```bash
pytest analysis/tests -v      # 23件
ruff check analysis/
mypy analysis/
```

**このテストは CI で実行し続ける。** 検証基準として機能させるには、これ自体が正しく動いている必要があるため。
