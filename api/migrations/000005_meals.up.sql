-- 食事の記録（要件 N-01 / N-02 / N-04）。
--
-- **食品マスタを持たない。** 記録時に名前と PFC を直接入れ、過去の記録が
-- そのまま候補になる（docs/01-要件定義.md §4.3）。マスタ登録という別作業を
-- 挟むと記録の摩擦が増え、続かない。
--
-- 正規化しないので name は自由入力。表記ゆれは候補の並びが分かれるだけで、
-- 集計（daily_metrics.kcal）には影響しない。

create table meals (
  id          uuid        primary key default gen_random_uuid(),
  -- JST における日付（ADR-0013）
  date        date        not null,

  -- 朝食/昼食/夕食/間食。決めずに記録できるよう null を許す
  slot        text        check (slot in ('朝食', '昼食', '夕食', '間食')),

  name        text        not null check (length(name) between 1 and 200),
  -- 「1個」「200g」のような自由記述。単位を型で縛ると入力が止まる
  qty         text        check (qty is null or length(qty) <= 50),

  kcal        integer     check (kcal >= 0 and kcal <= 10000),
  protein_g   numeric(6,1) check (protein_g >= 0 and protein_g <= 1000),
  fat_g       numeric(6,1) check (fat_g >= 0 and fat_g <= 1000),
  carb_g      numeric(6,1) check (carb_g >= 0 and carb_g <= 2000),

  -- **手入力か AI 推定かを残す**（docs/02-データモデル.md）。
  -- 推定値の比率が高い週は、体重トレンドとの整合が取れない可能性があり、
  -- 分析の確度を下げて扱う必要がある
  source      text        not null default 'manual'
                          check (source in ('manual', 'ai_estimated')),

  created_at  timestamptz not null default now(),
  updated_at  timestamptz not null default now(),
  deleted_at  timestamptz
);

-- 日付での絞り込みが主。生存行だけを引く
create index meals_date_idx on meals (date) where deleted_at is null;

-- 同期（ADR-0014）の差分取得用
create index meals_updated_at_idx on meals (updated_at);

-- 頻度順の候補（要件 N-02）。name でまとめて件数を数える
create index meals_name_idx on meals (name) where deleted_at is null;
