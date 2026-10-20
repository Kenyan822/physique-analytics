-- **履歴は失われる。** 今日時点で有効な1件だけが単一行に残る。
-- 今日より後に始まる予約と、過去の履歴は消える
create table manual_targets_single (
  id          boolean     primary key default true check (id),

  protein_g   numeric(6,1) not null check (protein_g >= 0 and protein_g <= 1000),
  fat_g       numeric(6,1) not null check (fat_g >= 0 and fat_g <= 1000),
  carb_g      numeric(6,1) not null check (carb_g >= 0 and carb_g <= 2000),

  updated_at  timestamptz not null default now()
);

insert into manual_targets_single (protein_g, fat_g, carb_g, updated_at)
select protein_g, fat_g, carb_g, created_at
from manual_targets
where starts_on <= current_date
order by starts_on desc
limit 1;

drop table manual_targets;
alter table manual_targets_single rename to manual_targets;
alter index manual_targets_single_pkey rename to manual_targets_pkey;

comment on table manual_targets is
  '手で決めた摂取目標。単一行。あるときは自動計算（A-02）より優先する';

alter table manual_targets enable row level security;
