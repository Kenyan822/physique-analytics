-- 初期スキーマ。openapi.yaml の components.schemas に対応する。
--
-- 方針:
--   * 時刻は timestamptz で保持する。表示・集計の JST 変換はアプリ側（ADR-0013）。
--     session の date だけは「JST における日付」そのものが意味を持つため date 型。
--   * 物理削除しない。deleted_at を立てる（ADR-0014）。
--   * 競合解決は updated_at の Last Write Wins（ADR-0014）。

create extension if not exists "pgcrypto";

-- 部位。openapi.yaml の MuscleGroup enum と一対一で対応させる。
-- 片方だけ変えると生成コードと DB がずれるので、必ず両方を同時に更新すること。
create type muscle_group as enum (
  '胸', '広背筋', '僧帽筋',
  '肩前部', '肩中部', '肩後部',
  '上腕二頭', '上腕三頭',
  '大腿四頭', 'ハム', '臀部', 'ふくらはぎ',
  '腹'
);

create table exercises (
  id               uuid        primary key default gen_random_uuid(),
  name             text        not null,
  muscle_group     muscle_group not null,
  is_compound      boolean     not null default false,
  default_rest_sec integer     check (default_rest_sec >= 0),
  created_at       timestamptz not null default now(),
  updated_at       timestamptz not null default now(),
  deleted_at       timestamptz
);

-- 種目名がゆれると時系列が分断される（openapi.yaml の Exercise.name 参照）。
-- 論理削除した名前は再利用できるよう、生存行だけを一意にする。
create unique index exercises_name_unique on exercises (name) where deleted_at is null;
create index exercises_muscle_group_idx on exercises (muscle_group) where deleted_at is null;

create table templates (
  id         uuid        primary key default gen_random_uuid(),
  name       text        not null,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  deleted_at timestamptz
);

create table template_items (
  template_id     uuid    not null references templates (id) on delete cascade,
  exercise_id     uuid    not null references exercises (id),
  item_order      integer not null check (item_order >= 1),
  target_sets     integer not null check (target_sets between 1 and 20),
  target_reps_min integer check (target_reps_min >= 1),
  target_reps_max integer check (target_reps_max >= 1),
  target_rir      integer check (target_rir between 0 and 10),
  primary key (template_id, item_order),
  check (target_reps_min is null or target_reps_max is null or target_reps_min <= target_reps_max)
);

create table workout_sessions (
  id          uuid        primary key default gen_random_uuid(),
  -- JST における日付（ADR-0013）。timestamptz にすると端末の TZ で日付が動く
  date        date        not null,
  template_id uuid        references templates (id),
  note        text,
  created_at  timestamptz not null default now(),
  updated_at  timestamptz not null default now(),
  deleted_at  timestamptz
);

create index workout_sessions_date_idx on workout_sessions (date desc) where deleted_at is null;

create table workout_sets (
  id          uuid        primary key default gen_random_uuid(),
  session_id  uuid        not null references workout_sessions (id),
  exercise_id uuid        not null references exercises (id),
  set_no      integer     not null check (set_no >= 1),
  -- kg 固定（docs/02-データモデル.md）。lb との混在は集計を壊す
  weight_kg   numeric(6,2) not null check (weight_kg >= 0),
  reps        integer     not null check (reps >= 0),
  -- RIR がないと推定1RMが計算できない。ただし過去データ取り込み（要件 I-01）では
  -- 欠損がありうるため NOT NULL にはしない
  rir         integer     check (rir between 0 and 10),
  created_at  timestamptz not null default now(),
  updated_at  timestamptz not null default now(),
  deleted_at  timestamptz
);

create unique index workout_sets_session_set_no_unique
  on workout_sets (session_id, set_no) where deleted_at is null;
create index workout_sets_exercise_idx on workout_sets (exercise_id) where deleted_at is null;

-- 同期（/v1/sync）は updated_at で差分を引く。全テーブルで引くのでインデックスを張る
create index exercises_updated_at_idx        on exercises (updated_at);
create index templates_updated_at_idx        on templates (updated_at);
create index workout_sessions_updated_at_idx on workout_sessions (updated_at);
create index workout_sets_updated_at_idx     on workout_sets (updated_at);
