-- 本体の PFC と引数を足し算にする（要件 N-02 / ADR-0017 の変更・#218）。
--
-- **これまで**: 引数があると本体の PFC を見なかった。
-- **これから**: 本体は常に効き、引数はその上に足される。
--
--   total = 本体PFC × (scales_with_amount ? 入力量 / base_amount : 1)
--         + Σ 引数PFC × (引数量 / 引数基準量)
--
-- **既存行は既定値のまま計算結果が変わらない。**
-- 引数だけの項目は本体が null（＝0）なので Σ引数 のまま。
-- 引数なしの項目は scales_with_amount = false なので 本体 × 1 のまま。
alter table food_items
  -- 「全量が量に比例する」ときの基準量。プロテインの「30g あたり」の 30。
  -- **比例しないなら使わない**ので null を許す
  add column base_amount numeric(8,2) check (base_amount is null or base_amount > 0),

  -- 表示専用。food_item_components.unit と同じ理由で型で縛らない
  add column base_unit text not null default 'g' check (length(base_unit) between 1 and 10),

  -- 全量が1つの量で決まるか。プロテインのように引数の行を作らずに済ませる
  add column scales_with_amount boolean not null default false;

-- **比例するなら基準量が要る。** 0 では割れないし、null だと計算できない
alter table food_items
  add constraint food_items_scaling_needs_basis
  check (not scales_with_amount or base_amount is not null);

comment on column food_items.base_amount is
  '全量が量に比例するときの基準量。scales_with_amount が false なら使わない';
comment on column food_items.scales_with_amount is
  '本体の PFC を入力量で比例させるか（ADR-0017 / #218）';
