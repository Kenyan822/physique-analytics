-- 手で決めた摂取目標（要件 N-05）。
--
-- いまの目標は体重トレンドから自動計算している（A-02。internal/weekly）。
-- **フェーズを登録するまで何も出ない**ので、記録を始めた直後に残量が見えない。
--
-- **期間で1つしか持たない。** フェーズ単位で変えるもので、日ごとに持つのは過剰。
-- profile と同じ「単一行」の形にする。
create table manual_targets (
  -- 単一行であることを型で保証する。profile と同じ手口
  id          boolean     primary key default true check (id),

  protein_g   numeric(6,1) not null check (protein_g >= 0 and protein_g <= 1000),
  fat_g       numeric(6,1) not null check (fat_g >= 0 and fat_g <= 1000),
  carb_g      numeric(6,1) not null check (carb_g >= 0 and carb_g <= 2000),

  updated_at  timestamptz not null default now()
);

comment on table manual_targets is
  '手で決めた摂取目標。単一行。あるときは自動計算（A-02）より優先する';

-- **kcal 列を持たない。** PFC から計算できる（Atwater 4/9/4）。
-- 持つと手入力と計算値が食い違ったときにどちらが正か決められなくなる。

-- PostgREST から見えないようにする（CLAUDE.md / #150）。
-- ポリシーは作らない —— RLS 有効でポリシーが無ければ、
-- 素通りできないロールからは「行が無い」ように見える
alter table manual_targets enable row level security;
