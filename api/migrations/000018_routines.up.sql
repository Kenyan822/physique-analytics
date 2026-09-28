-- ルーティン（要件 T-01・#232）。
--
-- **テンプレートを1日ぶんとして並べる。** templates / template_items は既に
-- 種目・目標セット・レップ・RIR を持っているので、足すのは巡回の定義だけ。
--
-- 曜日固定にしない。**やった日だけ進む**ので、日程が乱れてもバランスが崩れない。
create table routines (
  id         uuid        primary key default gen_random_uuid(),
  name       text        not null check (length(name) between 1 and 100),

  -- 増量期／減量期で切り替えられるよう複数持てる形にする
  is_active  boolean     not null default false,

  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  deleted_at timestamptz
);

-- **有効なのは1つだけ。** 2つあるとどれを今日のルーティンにするか決められない
create unique index routines_one_active
  on routines ((true)) where is_active and deleted_at is null;

comment on table routines is
  'ルーティン。テンプレートを巡回の順に並べたもの（T-01 / #232）';

-- ルーティンの1日ぶん。**テンプレートを指す**
create table routine_days (
  routine_id  uuid    not null references routines (id) on delete cascade,
  -- 巡回の順。1 から連番
  day_order   integer not null check (day_order >= 1),
  template_id uuid    not null references templates (id),

  primary key (routine_id, day_order),
  -- 同じテンプレートを2日に割り当てない。どちらをやったのか判別できなくなる
  constraint routine_days_template_unique unique (routine_id, template_id)
);

comment on column routine_days.day_order is
  '巡回の順。今日が何日目かはカウンタで持たず、直近のセッションから導く（#232）';

-- PostgREST から見えないようにする（CLAUDE.md / #150）
alter table routines enable row level security;
alter table routine_days enable row level security;
