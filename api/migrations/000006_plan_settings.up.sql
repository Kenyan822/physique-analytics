-- 計画の設定（要件 P-01 / P-05）。
--
-- これまで private/config.json にあり、手元の MCP からしか読めなかった。
-- API（栄養目標の表示 = 要件 N-05）と Web の設定画面から使うため DB に移す。
--
-- **1ユーザー前提の単一行テーブルがある。** 複数ユーザーに広げるときは
-- user_id を足して主キーを変える（issue #51 と同じ作業になる）。

-- 身体の基本値。正規化FFMI と海軍式推定に身長が要る
create table profile (
  -- 単一行であることを制約で保証する。行が増えると「どれが正か」が曖昧になる
  id          boolean     primary key default true check (id),
  height_cm   numeric(4,1) check (height_cm > 0 and height_cm < 300),
  start_date  date,
  created_at  timestamptz not null default now(),
  updated_at  timestamptz not null default now()
);

-- フェーズ（要件 P-01）。期間ごとの目標ペース
create table plan_phases (
  id              uuid        primary key default gen_random_uuid(),
  name            text        not null check (length(name) between 1 and 100),
  starts_on       date        not null,
  ends_on         date        not null,
  -- 週あたりの体重変化の目標。負なら減量。±2kg/週 を超える計画は現実的でない
  goal_kg_per_week numeric(4,2) not null check (goal_kg_per_week between -2 and 2),

  created_at      timestamptz not null default now(),
  updated_at      timestamptz not null default now(),

  constraint plan_phases_period check (starts_on <= ends_on)
);

create index plan_phases_period_idx on plan_phases (starts_on, ends_on);

-- 栄養パラメータ（要件 P-05）。
--
-- タンパク質と脂質は体重あたりの係数で持つ。炭水化物は残余なので持たない
-- （docs/03-分析ロジック.md 分析1）。
--
-- **小数第2位まで取る。** 脂質 0.85g/kg が numeric(3,1) では 0.9 に丸まり、
-- 体重72kg で 61.2g が 64.8g になる（炭水化物の残余も 8g ずれる）。
create table nutrition_settings (
  id                     boolean     primary key default true check (id),
  cut_protein_g_per_kg   numeric(4,2) not null check (cut_protein_g_per_kg > 0 and cut_protein_g_per_kg <= 5),
  cut_fat_g_per_kg       numeric(4,2) not null check (cut_fat_g_per_kg > 0 and cut_fat_g_per_kg <= 5),
  deep_cut_protein_g_per_kg numeric(4,2) not null check (deep_cut_protein_g_per_kg > 0 and deep_cut_protein_g_per_kg <= 5),
  deep_cut_fat_g_per_kg  numeric(4,2) not null check (deep_cut_fat_g_per_kg > 0 and deep_cut_fat_g_per_kg <= 5),
  bulk_protein_g_per_kg  numeric(4,2) not null check (bulk_protein_g_per_kg > 0 and bulk_protein_g_per_kg <= 5),
  bulk_fat_g_per_kg      numeric(4,2) not null check (bulk_fat_g_per_kg > 0 and bulk_fat_g_per_kg <= 5),
  -- この体脂肪率を下回ったらタンパク質を上げる（LBM 保護）
  deep_cut_bf_threshold  numeric(3,1) not null check (deep_cut_bf_threshold > 0 and deep_cut_bf_threshold < 50),
  -- 炭水化物の下限。割ったらトレーニングの質が落ちる
  carb_min_g             integer     not null check (carb_min_g >= 0 and carb_min_g <= 1000),

  created_at             timestamptz not null default now(),
  updated_at             timestamptz not null default now()
);

-- 部位別の MEV/MRV（要件 P-05）。
--
-- 一括の 10-20 では判定できない。肩は3分割、背中は広背筋/僧帽筋に分けており、
-- 間接刺激の多い部位は直接種目の基準を下げる（docs/03-分析ロジック.md 分析4）。
create table volume_ranges (
  muscle_group text        primary key,
  mev          integer     not null check (mev >= 0 and mev <= 50),
  mrv          integer     not null check (mrv >= 0 and mrv <= 50),
  created_at   timestamptz not null default now(),
  updated_at   timestamptz not null default now(),

  constraint volume_ranges_order check (mev <= mrv)
);

-- 既定値を入れておく。設定しないまま分析すると全部位が「判定不能」になる。
-- 値は config.example.json（公開用の例）と同じ
insert into volume_ranges (muscle_group, mev, mrv) values
  ('胸', 10, 20),
  ('広背筋', 10, 20),
  ('僧帽筋', 8, 18),
  ('肩前部', 4, 12),
  ('肩中部', 8, 14),
  ('肩後部', 4, 14),
  ('上腕二頭', 8, 16),
  ('上腕三頭', 8, 16),
  ('大腿四頭', 10, 20),
  ('ハム', 8, 16),
  ('臀部', 6, 14),
  ('ふくらはぎ', 8, 16),
  ('腹', 4, 16);

insert into nutrition_settings
  (id, cut_protein_g_per_kg, cut_fat_g_per_kg,
   deep_cut_protein_g_per_kg, deep_cut_fat_g_per_kg,
   bulk_protein_g_per_kg, bulk_fat_g_per_kg,
   deep_cut_bf_threshold, carb_min_g)
values (true, 2.4, 0.85, 2.6, 0.85, 2.2, 1.0, 13.0, 200);
