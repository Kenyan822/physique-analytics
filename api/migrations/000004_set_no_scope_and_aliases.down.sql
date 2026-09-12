drop table if exists exercise_aliases;

drop index if exists workout_sets_session_exercise_set_no_unique;

-- 000001 の制約に戻す。ただし種目ごとに set_no が振られた記録が既に
-- 入っていると、重複して作れない。その場合は先にデータを整理する必要がある
create unique index workout_sets_session_set_no_unique
  on workout_sets (session_id, set_no) where deleted_at is null;
