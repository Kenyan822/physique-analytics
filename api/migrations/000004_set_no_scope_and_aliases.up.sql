-- 1. set_no の一意性の範囲を直す
--
-- 000001 では (session_id, set_no) を一意にしていたが、**set_no は種目ごとに
-- 振られる**。実際の記録も reference/analysis/ の CSV もそうなっている。
--
--   2026-09-07,ベンチプレス,1,81.0,5,3
--   2026-09-07,ベンチプレス,2,81.0,4,1
--   2026-09-07,インクラインベンチプレス,1,60.0,8,3   ← また 1 に戻る
--
-- 旧制約のままだと、1日に複数種目をやった記録が2種目目から入らない。
-- CSV 取り込みで 768 行中 468 行が弾かれて発覚した。

drop index if exists workout_sets_session_set_no_unique;

create unique index workout_sets_session_exercise_set_no_unique
  on workout_sets (session_id, exercise_id, set_no) where deleted_at is null;

-- 2. 種目名の表記ゆれを引き当てる表
--
-- 種目マスタ（000002）には正規名だけを入れている。表記がゆれると時系列が
-- 分断されるため（openapi.yaml の Exercise.name）。
-- 一方、過去に手で書いた CSV には別表記が混ざる（要件 I-01）。
--
-- 引き当てをコードではなく表で持つのは、後から別表記が出てきたときに
-- 1行 insert するだけで済むようにするため。

create table exercise_aliases (
  alias       text not null primary key,
  exercise_id uuid not null references exercises (id) on delete cascade,
  created_at  timestamptz not null default now()
);

create index exercise_aliases_exercise_idx on exercise_aliases (exercise_id);

-- reference/analysis/analyze.py の EXERCISE_MUSCLE にあった別表記。
-- 正規名は 000002 で入れたもの。
insert into exercise_aliases (alias, exercise_id)
select a.alias, e.id
from (values
  ('バーベルベンチプレス',   'ベンチプレス'),
  ('バーベルスクワット',     'スクワット'),
  ('ワンハンドロウ',         'ワンハンドロー'),
  ('シーテッドロウ',         'シーテッドロー'),
  ('バーベルアームカール',   'バーベルカール'),
  ('ダンベルアームカール',   'ダンベルカール'),
  ('ベントオーバーロウ',     'バーベルロー'),
  ('ダンベルサイドレイズ',   'サイドレイズ'),
  ('スカルクラッシャー',     'ライイングエクステンション')
) as a(alias, canonical)
join exercises e on e.name = a.canonical and e.deleted_at is null
on conflict (alias) do nothing;
