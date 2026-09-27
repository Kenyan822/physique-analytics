-- 引数を「合計のうち一部」にする（要件 N-02 / ADR-0017 の再変更・#224）。
--
-- **これまで**（#218）: total = 本体 + Σ引数。本体は固定部だった
-- **これから**:          固定部 = 本体 − Σ引数。本体は合計
--
--   total = 固定部 + Σ 引数PFC × (入力量 / 登録時の量)
--
-- あわせて引数の「基準量」と「既定」を1つに統合する。2つ持っていると
-- 区別がつかないという指摘があったため。
--
-- **既存の登録の計算結果を変えない**のがこの移行の目的。順序が要る。

-- ---- 1. 引数を1つの量に統合する -------------------------------------
--
-- 新しい量は「いつもの量」。PFC はその量ぶんに按分する。
--   旧: 基準量 100 で P23、いつもの量 200  →  寄与は 46
--   新: 量 200 で P46                      →  寄与は 46（同じ）
alter table food_item_components
  add column amount numeric(8,2),
  -- 合計そのものを表すか。**立つと引数は1つだけ**（プロテイン）
  add column covers_all boolean not null default false;

update food_item_components
set
  -- 既定が 0 の行は基準量を使う。**0 では割れない**
  amount = case when default_amount > 0 then default_amount else basis_amount end,
  protein_g = protein_g
    * (case when default_amount > 0 then default_amount else basis_amount end) / basis_amount,
  fat_g = fat_g
    * (case when default_amount > 0 then default_amount else basis_amount end) / basis_amount,
  carb_g = carb_g
    * (case when default_amount > 0 then default_amount else basis_amount end) / basis_amount
where basis_amount > 0;

-- 基準量が壊れている行の保険。按分できないので量だけ入れる
update food_item_components set amount = 1 where amount is null or amount <= 0;

alter table food_item_components
  alter column amount set not null,
  add constraint food_item_components_amount_positive check (amount > 0);

-- ---- 2. 本体を「合計」にする ----------------------------------------
--
-- **1 のあとに走らせる。** 按分後の値（＝旧モデルでの寄与）を足す。
-- 旧: 本体 = 固定部  →  新: 本体 = 固定部 + Σ引数 = 合計
update food_items i
set
  protein_g = coalesce(i.protein_g, 0) + c.p,
  fat_g     = coalesce(i.fat_g, 0) + c.f,
  carb_g    = coalesce(i.carb_g, 0) + c.c
from (
  select food_item_id,
         sum(protein_g) as p, sum(fat_g) as f, sum(carb_g) as c
  from food_item_components
  group by food_item_id
) c
where c.food_item_id = i.id;

-- ---- 3. 「量に比例する」項目を引数に変換する -------------------------
--
-- #218 で入れた scales_with_amount の置き換え。引数を持たない項目なので
-- 2 の対象外。本体はすでに合計なので、それをそのまま引数にする
insert into food_item_components
  (food_item_id, item_order, name, unit, amount, protein_g, fat_g, carb_g, covers_all,
   basis_amount, default_amount)
select
  id, 1, '量', coalesce(base_unit, 'g'), base_amount,
  coalesce(protein_g, 0), coalesce(fat_g, 0), coalesce(carb_g, 0), true,
  base_amount, base_amount
from food_items
where scales_with_amount
  and base_amount is not null
  and base_amount > 0
  and not exists (select 1 from food_item_components c where c.food_item_id = food_items.id);

-- ---- 4. 古い列を落とす ----------------------------------------------
alter table food_item_components
  drop column basis_amount,
  drop column default_amount;

alter table food_items
  drop constraint if exists food_items_scaling_needs_basis;

alter table food_items
  drop column scales_with_amount,
  drop column base_unit,
  drop column base_amount;

comment on column food_item_components.amount is
  '登録したときの量。基準にも初期値にもなる（ADR-0017 / #224）';
comment on column food_item_components.covers_all is
  '合計そのものを表すか。立つと引数は1つだけ（ADR-0017 / #224）';
comment on table food_items is
  '食品マスタ。protein_g 等は**合計**で、引数はその内訳（ADR-0017 / #224）';
