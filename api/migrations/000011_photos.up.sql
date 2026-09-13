-- 身体写真（要件 B-04 / B-05 / B-07）。月1回・3枚（正面・側面・背面）。
--
-- **画像そのものは DB に入れない**（ADR-0008）。R2 に置き、ここには
-- 位置（キー）とメタデータだけを持つ。DB に入れるとバックアップが重くなり、
-- 取り出すたびに DB の帯域を使う。
--
-- 3年計画において**写真は最終的に最も価値の高いデータ**になる
-- （docs/02-データモデル.md）。体重が横ばいでも見た目が変わるフェーズが
-- 必ず来るため、そのとき数値だけでは進捗が見えない。

create table body_photos (
  id          uuid        primary key default gen_random_uuid(),
  -- 撮影日（JST。ADR-0013）
  date        date        not null,
  -- 正面 / 側面 / 背面。**同じ日に同じ向きは1枚**
  pose        text        not null check (pose in ('front', 'side', 'back')),

  -- ストレージ上のキー。バケット名は持たない（環境で変わる）
  storage_key text        not null check (length(storage_key) between 1 and 500),
  mime_type   text        not null check (mime_type in ('image/jpeg', 'image/png', 'image/heic')),
  byte_size   bigint      not null check (byte_size > 0),

  -- 撮影条件を残す。**条件を固定しないと比較が成立しない**
  -- （docs/02-データモデル.md）
  note        text        check (note is null or length(note) <= 500),

  created_at  timestamptz not null default now(),
  updated_at  timestamptz not null default now(),
  deleted_at  timestamptz
);

-- 同じ日・同じ向きは1枚。撮り直しは上書きになる
create unique index body_photos_date_pose_unique
  on body_photos (date, pose) where deleted_at is null;

-- 時系列で並べる（要件 B-07）
create index body_photos_date_idx on body_photos (date) where deleted_at is null;
