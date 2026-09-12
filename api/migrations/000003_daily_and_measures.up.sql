-- 日次の記録と周囲長。
--
-- CSV（data/sample/daily.csv / measures.csv）と同じ項目を持つ。
-- 分析の入力であり（TDEE の逆算・正規化FFMI・回復判定）、
-- CSV 取り込み（要件 I-01）の受け皿でもある。
--
-- 1日1行。date を主キーにする代わりに uuid を振っているのは、
-- 他のテーブルと同様に同期（ADR-0014）で id を使うため。

create table daily_metrics (
  id              uuid        primary key default gen_random_uuid(),
  -- JST における日付（ADR-0013）。1日1行
  date            date        not null,

  -- 体組成。体重は毎日、体脂肪率は測れた日だけ
  weight_kg       numeric(5,2) check (weight_kg > 0 and weight_kg < 300),
  bodyfat_pct     numeric(4,1) check (bodyfat_pct >= 0 and bodyfat_pct < 70),

  -- 栄養。TDEE の逆算に kcal が要る
  kcal            integer     check (kcal >= 0),
  protein_g       integer     check (protein_g >= 0),
  fat_g           integer     check (fat_g >= 0),
  carb_g          integer     check (carb_g >= 0),

  -- 回復。Apple Watch から入る想定（Phase 2 以降）
  sleep_h         numeric(3,1) check (sleep_h >= 0 and sleep_h <= 24),
  steps           integer     check (steps >= 0),
  -- 主観的な疲労度 1-5
  fatigue         integer     check (fatigue between 1 and 5),
  hrv_ms          integer     check (hrv_ms > 0),
  resting_hr      integer     check (resting_hr > 0 and resting_hr < 200),
  deep_sleep_min  integer     check (deep_sleep_min >= 0),

  note            text,

  created_at      timestamptz not null default now(),
  updated_at      timestamptz not null default now(),
  deleted_at      timestamptz
);

-- 1日1行。論理削除した日は再登録できるよう生存行だけを一意にする
create unique index daily_metrics_date_unique on daily_metrics (date) where deleted_at is null;
create index daily_metrics_updated_at_idx on daily_metrics (updated_at);

create table body_measurements (
  id             uuid        primary key default gen_random_uuid(),
  date           date        not null,

  -- すべて cm。海軍式の体脂肪率推定には neck と waist_navel が要る
  neck_cm        numeric(4,1) check (neck_cm > 0),
  shoulder_cm    numeric(4,1) check (shoulder_cm > 0),
  chest_cm       numeric(4,1) check (chest_cm > 0),
  waist_navel_cm numeric(4,1) check (waist_navel_cm > 0),
  hip_cm         numeric(4,1) check (hip_cm > 0),
  arm_r_cm       numeric(4,1) check (arm_r_cm > 0),
  thigh_r_cm     numeric(4,1) check (thigh_r_cm > 0),
  calf_r_cm      numeric(4,1) check (calf_r_cm > 0),

  created_at     timestamptz not null default now(),
  updated_at     timestamptz not null default now(),
  deleted_at     timestamptz
);

create unique index body_measurements_date_unique on body_measurements (date) where deleted_at is null;
create index body_measurements_updated_at_idx on body_measurements (updated_at);
