-- 「全部」の引数に合計の PFC を入れる（#344）。
--
-- ADR-0017 は「全部」を**合計そのもの**と定義している。
--
--   固定部 = 合計 − 引数   →   全部なら 0
--
-- ところがクライアントは「全部」のとき PFC 欄を出していない（合計と同じなので
-- 入れる意味が無い）ため、**0 のまま保存されていた**。
--
--   固定部 = 21.1 − 0 = 21.1
--   出力   = 21.1 + 0 × (入力量/30) = 21.1   ← 量を変えても効かない
--
-- 以降はサーバ（normalizeFoodItem）が埋める。ここは既存の行を直す。
--
-- **0 の行だけを直す。** 正しく入っている行に触ると、合計を後から変えた
-- ケースで引数の値を上書きしてしまう。
update food_item_components c
set protein_g = coalesce(i.protein_g, 0),
    fat_g     = coalesce(i.fat_g, 0),
    carb_g    = coalesce(i.carb_g, 0)
from food_items i
where c.food_item_id = i.id
  and c.covers_all
  and c.protein_g = 0
  and c.fat_g = 0
  and c.carb_g = 0;
