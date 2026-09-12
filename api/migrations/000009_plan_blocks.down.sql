alter table profile
  drop column if exists baseline_weight_kg,
  drop column if exists baseline_bodyfat_pct,
  drop column if exists baseline_month;

drop table if exists plan_blocks;
