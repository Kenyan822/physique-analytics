-- 食品マスタ（要件 N-02 / ADR-0017）。
--
-- **引数は任意。** 大半の食品は量が固定なので、基本は名前と PFC だけで登録する。
-- 量が変わるもの（プロテイン・料理の材料）だけ、あとから引数を足せる。
create table food_items (
  id          uuid        primary key default gen_random_uuid(),
  name        text        not null check (length(name) between 1 and 200),

  -- 量の目安。「1杯」「1個」のような自由記述（meals.qty と同じ理由で型で縛らない）
  qty         text        check (qty is null or length(qty) <= 50),

  -- **引数が無いときに使う値。** 構成があるときは構成から計算するので見ない。
  -- kcal を持たないのは PFC から出せるため（Atwater 4/9/4）
  protein_g   numeric(6,1) check (protein_g >= 0 and protein_g <= 1000),
  fat_g       numeric(6,1) check (fat_g >= 0 and fat_g <= 1000),
  carb_g      numeric(6,1) check (carb_g >= 0 and carb_g <= 2000),

  -- 並び順に使う。**選ばれた回数**（N-02 の「頻度順」と同じ考え方）
  used_count  integer     not null default 0 check (used_count >= 0),
  last_used_at timestamptz,

  created_at  timestamptz not null default now(),
  updated_at  timestamptz not null default now(),
  deleted_at  timestamptz
);

-- 名前は一意にする。同じ名前が並ぶと選ぶときに区別できない（meal_sets と同じ）
create unique index food_items_name_unique on food_items (name) where deleted_at is null;

-- よく使うものを上に出す
create index food_items_used_idx
  on food_items (used_count desc, last_used_at desc nulls last)
  where deleted_at is null;

comment on table food_items is
  '食品マスタ。引数が無ければ protein_g 等をそのまま使う（ADR-0017）';

-- 引数（構成）。**あるときだけ行が入る。**
--
-- 「30g あたり P24」を登録し、入力時に 45g と入れたら
-- P = 24 × (45 / 30) = 36 と計算する。
--
-- 料理は行を複数持つ。買った肉の量が変わったら、その行の入力量だけ直せばよい。
create table food_item_components (
  id            uuid        primary key default gen_random_uuid(),
  food_item_id  uuid        not null references food_items (id) on delete cascade,

  -- 表示順。持ち主が決める（meal_set_items と同じ）
  item_order    integer     not null check (item_order >= 1),

  -- 引数の名前。単一引数なら「量」、料理なら「鶏ひき肉」など
  name          text        not null check (length(name) between 1 and 100),

  -- **単位は表示専用。** 計算に要るのは 入力量 / 基準量 の比だけなので、
  -- g / ml / 個 を型で縛らない（ADR-0017）
  unit          text        not null default 'g' check (length(unit) between 1 and 10),

  -- 基準量。パッケージの「n g あたり」の n。**0 では割れない**
  basis_amount  numeric(8,2) not null check (basis_amount > 0),
  -- 入力時の初期値。触らなければこれが使われる
  default_amount numeric(8,2) not null check (default_amount >= 0),

  -- 基準量あたりの PFC
  protein_g     numeric(6,1) not null default 0 check (protein_g >= 0 and protein_g <= 1000),
  fat_g         numeric(6,1) not null default 0 check (fat_g >= 0 and fat_g <= 1000),
  carb_g        numeric(6,1) not null default 0 check (carb_g >= 0 and carb_g <= 2000),

  created_at    timestamptz not null default now(),

  constraint food_item_components_order_unique unique (food_item_id, item_order)
);

create index food_item_components_item_idx on food_item_components (food_item_id);

comment on table food_item_components is
  '食品マスタの引数。基準量あたりの PFC を持ち、入力量との比で計算する（ADR-0017）';

-- PostgREST から見えないようにする（CLAUDE.md / #150）
alter table food_items enable row level security;
alter table food_item_components enable row level security;
