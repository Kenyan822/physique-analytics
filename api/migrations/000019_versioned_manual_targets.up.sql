-- 手動の摂取目標を「適用開始日つきの履歴」にする（要件 N-05 / #241）。
--
-- 単一行だと目標を変えたときに前の値が消え、**過去日の残量が現在の目標で
-- 計算されてしまう**。3年分を後から振り返るのが目的なので、ここが狂うと分析にならない。
--
-- **active フラグを持たない。** 「active な行」と「starts_on が最新の行」は同値で、
-- 別に持つと active が2行ある・active なのに開始日が未来、という矛盾した状態を作れる。
-- その日の目標は `where starts_on <= $1 order by starts_on desc limit 1`。
--
-- **mode 列を持たない。** 自動計算に戻す印は、必要になってから足せる
-- （`alter table add column ... default 'manual'`）。

alter table manual_targets rename to manual_targets_single;
alter index manual_targets_pkey rename to manual_targets_single_pkey;

create table manual_targets (
  id          uuid         primary key default gen_random_uuid(),
  -- PUT（今日から）は名前を持たないので null を許す
  name        text         check (name is null or length(name) between 1 and 100),
  -- この日から適用。同じ日に2回変えたら上書き
  starts_on   date         not null unique,

  protein_g   numeric(6,1) not null check (protein_g >= 0 and protein_g <= 1000),
  fat_g       numeric(6,1) not null check (fat_g >= 0 and fat_g <= 1000),
  carb_g      numeric(6,1) not null check (carb_g >= 0 and carb_g <= 2000),

  created_at  timestamptz  not null default now()
);

-- 既存の1行は記録の最古日から始める。**既存の全日が今までどおり同じ目標で評価され、
-- 移行による表示の変化がゼロになる。** profile.start_date は null なので使えない
insert into manual_targets (starts_on, protein_g, fat_g, carb_g, created_at)
select least(
         (select min(date) from meals),
         (select min(date) from body_measurements),
         (select min(date) from workout_sessions),
         current_date),
       protein_g, fat_g, carb_g, updated_at
from manual_targets_single;

drop table manual_targets_single;

comment on table manual_targets is
  '手で決めた摂取目標の履歴。starts_on 以降に適用する。あるときは自動計算（A-02）より優先する';

-- **kcal 列を持たない。** PFC から計算できる（Atwater 4/9/4）。
-- 持つと手入力と計算値が食い違ったときにどちらが正か決められなくなる。

-- PostgREST から見えないようにする（CLAUDE.md / #150）。ポリシーは作らない
alter table manual_targets enable row level security;
