-- 血液検査（要件 B-08）。年2回。
--
-- **検査項目を列にしない。** クリニックや検査パネルによって項目が違い、
-- 列で持つと「うちの検査票に無い項目」「列に無い項目」の両方が発生する。
-- 名前・値・単位・基準範囲を行として持ち、表そのものを写せる形にする。
--
-- 基準範囲もクリニックごとに違うので、検査票に書いてある値を一緒に入れる。
-- 固定の基準を持つと、別のクリニックで測ったときに誤判定になる。

create table blood_tests (
  id          uuid        primary key default gen_random_uuid(),
  -- 採血日（JST。ADR-0013）
  date        date        not null,
  clinic      text        check (clinic is null or length(clinic) <= 100),
  note        text        check (note is null or length(note) <= 1000),

  created_at  timestamptz not null default now(),
  updated_at  timestamptz not null default now(),
  deleted_at  timestamptz
);

-- 同じ日に2回採血することは無い。生存行だけを一意にする
create unique index blood_tests_date_unique on blood_tests (date) where deleted_at is null;
create index blood_tests_date_idx on blood_tests (date) where deleted_at is null;

create table blood_test_items (
  id            uuid        primary key default gen_random_uuid(),
  blood_test_id uuid        not null references blood_tests (id) on delete cascade,
  -- 検査票の並び順。項目名で並べ替えると検査票と見比べられない
  item_order    integer     not null check (item_order >= 1),

  name          text        not null check (length(name) between 1 and 100),
  -- 数値で入らない項目（「陰性」など）があるので null を許す
  value         numeric(12,3),
  -- 数値で入らない項目の生の表記
  text_value    text        check (text_value is null or length(text_value) <= 50),
  unit          text        check (unit is null or length(unit) <= 30),

  -- 基準範囲。**検査票に書いてある値を入れる**（クリニックごとに違う）
  ref_low       numeric(12,3),
  ref_high      numeric(12,3),

  created_at    timestamptz not null default now(),

  constraint blood_test_items_order_unique unique (blood_test_id, item_order),
  -- 下限が上限を超えていたら入力ミス
  constraint blood_test_items_ref_range check (ref_low is null or ref_high is null or ref_low <= ref_high)
);

create index blood_test_items_test_idx on blood_test_items (blood_test_id);
