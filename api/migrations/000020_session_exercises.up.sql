-- その日の種目リスト（要件 T-01・#242）。
--
-- ルーティン（routine_days → template_items）は固定の予定で、その日の都合では
-- 変えられない。**その日だけの並びをここに持つ。** ルーティンには触らない。
--
-- **行が1件も無いセッション = まだ触っていない。** クライアントはルーティンの
-- 並びにフォールバックする。並べ替え・追加・削除をしたら全体を書き込む（全置換）
create table session_exercises (
  session_id  uuid        not null references workout_sessions (id) on delete cascade,
  exercise_id uuid        not null references exercises (id),
  -- 表示順。1 から連番
  item_order  integer     not null check (item_order >= 1),
  created_at  timestamptz not null default now(),

  -- 同じ種目を1日に2回並べない
  primary key (session_id, exercise_id)
);

create index session_exercises_order on session_exercises (session_id, item_order);

comment on table session_exercises is
  'その日の種目リスト。行が無ければルーティンの並びを使う（T-01 / #242）';

-- PostgREST から見えないようにする（CLAUDE.md / #150）
alter table session_exercises enable row level security;
