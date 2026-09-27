-- **完全には戻せない。** 統合した量を2つに割る情報が無いので、
-- 基準量＝既定＝amount として戻す（計算結果は保たれる）。
alter table food_items
  add column base_amount numeric(8,2) check (base_amount is null or base_amount > 0),
  add column base_unit text not null default 'g' check (length(base_unit) between 1 and 10),
  add column scales_with_amount boolean not null default false;

alter table food_item_components
  add column basis_amount numeric(8,2),
  add column default_amount numeric(8,2);

update food_item_components set basis_amount = amount, default_amount = amount;

-- covers_all の引数は項目側の設定に戻す
update food_items i
set scales_with_amount = true, base_amount = c.amount, base_unit = c.unit
from food_item_components c
where c.food_item_id = i.id and c.covers_all;

delete from food_item_components where covers_all;

-- 本体を固定部に戻す
update food_items i
set
  protein_g = greatest(coalesce(i.protein_g, 0) - c.p, 0),
  fat_g     = greatest(coalesce(i.fat_g, 0) - c.f, 0),
  carb_g    = greatest(coalesce(i.carb_g, 0) - c.c, 0)
from (
  select food_item_id, sum(protein_g) as p, sum(fat_g) as f, sum(carb_g) as c
  from food_item_components group by food_item_id
) c
where c.food_item_id = i.id;

alter table food_item_components
  alter column basis_amount set not null,
  alter column default_amount set not null,
  drop constraint if exists food_item_components_amount_positive,
  drop column covers_all,
  drop column amount;

alter table food_items
  add constraint food_items_scaling_needs_basis
  check (not scales_with_amount or base_amount is not null);
