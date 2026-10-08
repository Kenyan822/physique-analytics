-- **rough の記録は manual に戻る**（制約を戻すと rough の行が残れないため）。
-- ざっくり入力だったという印は失われる。PFC・kcal の値は消えない
update meals set source = 'manual' where source = 'rough';

alter table meals drop constraint meals_source_check;
alter table meals add constraint meals_source_check
  check (source in ('manual', 'ai_estimated'));
