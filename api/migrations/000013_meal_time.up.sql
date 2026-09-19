-- 食事に「食べた時刻」を持たせる。
--
-- **表形式の入力画面（#188）が時刻を必要とする。** 区分（朝食/昼食/…）を
-- 手で選ばせるのをやめ、時刻から導出する。
--
-- timestamptz にしない。date を別に持っているので二重になり、
-- タイムゾーンの解釈が混ざる。ADR-0013 の「JST の壁時計で持つ」に揃える。
--
-- **列名を at にしない。** Postgres の予約語（AT TIME ZONE）で、
-- 使うたびに引用符が要る。

-- null 許容。**既存の記録には時刻が無い**ので、埋められない
alter table meals add column eaten_at time;

comment on column meals.eaten_at is
  'JST の壁時計での時刻。null は「時刻を記録していない」。slot はここから導出する';

-- 表示は時刻順にする。日付での絞り込みと組み合わせるので複合にする。
-- nulls last: 時刻が無い既存の記録を後ろに送る
create index meals_date_eaten_at_idx
  on meals (date, eaten_at nulls last)
  where deleted_at is null;

-- 名前を任意にする。
--
-- **PFC だけ入れて済ませたい**（#188）。「何を食べたか」は後から足せるが、
-- 入力の瞬間に思い出せないと記録そのものを諦めることになる。
--
-- check (length(name) between 1 and 200) はそのまま残す。
-- **NULL に対する CHECK は通る**（結果が NULL なので false ではない）ので、
-- 「無し」と「1〜200文字」の両方だけが入る状態は保たれる。
alter table meals alter column name drop not null;

comment on column meals.name is
  '食べたもの。null は「記録していない」。N-02 の候補には出ない';
