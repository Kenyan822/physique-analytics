-- 食事セット（要件 N-03）。
--
-- 「朝食セット」のように毎回同じ組み合わせで食べるものを、ワンタップで
-- 記録できるようにする。**食品マスタではない**（docs/01-要件定義.md §4.3）。
-- あくまで「よく食べる組み合わせ」の保存で、個々の食品は meals と同じ形で持つ。

create table meal_sets (
  id          uuid        primary key default gen_random_uuid(),
  name        text        not null check (length(name) between 1 and 100),
  -- 展開したときの既定の区分。null なら記録時に選ぶ
  slot        text        check (slot in ('朝食', '昼食', '夕食', '間食')),

  created_at  timestamptz not null default now(),
  updated_at  timestamptz not null default now(),
  deleted_at  timestamptz
);

-- 名前は一意にする。同じ名前が並ぶと選ぶときに区別できない
create unique index meal_sets_name_unique on meal_sets (name) where deleted_at is null;

create table meal_set_items (
  id          uuid        primary key default gen_random_uuid(),
  meal_set_id uuid        not null references meal_sets (id) on delete cascade,
  -- 表示順。meals と違い記録ではないので、順序は持ち主が決める
  item_order  integer     not null check (item_order >= 1),

  -- 列は meals と揃える。展開時にそのまま写せるようにするため
  name        text        not null check (length(name) between 1 and 200),
  qty         text        check (qty is null or length(qty) <= 50),
  kcal        integer     check (kcal >= 0 and kcal <= 10000),
  protein_g   numeric(6,1) check (protein_g >= 0 and protein_g <= 1000),
  fat_g       numeric(6,1) check (fat_g >= 0 and fat_g <= 1000),
  carb_g      numeric(6,1) check (carb_g >= 0 and carb_g <= 2000),

  created_at  timestamptz not null default now(),

  constraint meal_set_items_order_unique unique (meal_set_id, item_order)
);

create index meal_set_items_set_idx on meal_set_items (meal_set_id);
