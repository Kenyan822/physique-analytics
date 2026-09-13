-- 計画のブロックと起点（要件 P-02 / P-03）。
--
-- 月次目標は**固定値ではなく、起点から毎回計算するもの**
-- （reference/analysis/make_monthly_plan.py）。なので月ごとの表は持たず、
-- 設計パラメータだけを持つ。
--
-- plan_phases（期間 + 目標ペース）とは別に持つ。あちらは「その日の目標ペース」を
-- 引くための日次の表で、こちらは「3年でどう積むか」の設計。粒度も更新頻度も違う。

create table plan_blocks (
  id          uuid        primary key default gen_random_uuid(),
  -- 表示順。期間ではなく順序で並ぶ（開始月は起点から積み上げて決まる）
  block_order integer     not null check (block_order >= 1),
  name        text        not null check (length(name) between 1 and 100),
  months      integer     not null check (months >= 1 and months <= 60),

  -- 1ヶ月あたりの LBM の増減。**これが設計値**
  lbm_delta_kg_per_month numeric(4,2) not null check (lbm_delta_kg_per_month between -2 and 2),
  -- ブロック終了時点の体脂肪率。途中は等分に動かす
  bodyfat_pct_end        numeric(4,1) not null check (bodyfat_pct_end > 0 and bodyfat_pct_end < 60),

  -- そのブロックで何をするか。数字だけでは行動が決まらない
  focus       text        check (focus is null or length(focus) <= 500),

  created_at  timestamptz not null default now(),
  updated_at  timestamptz not null default now(),

  constraint plan_blocks_order_unique unique (block_order)
);

-- 月次目標の起点（要件 P-03）。実測から引き直すときに更新する
alter table profile
  add column baseline_weight_kg   numeric(5,2) check (baseline_weight_kg > 0 and baseline_weight_kg < 300),
  add column baseline_bodyfat_pct numeric(4,1) check (baseline_bodyfat_pct >= 0 and baseline_bodyfat_pct < 70),
  -- 起点の月（YYYY-MM-01 を入れる）。月次目標の1ヶ月目になる
  add column baseline_month       date;
