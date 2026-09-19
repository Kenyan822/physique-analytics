-- **記録した時刻が消える。** 元に戻す手段は無い。
--
-- 名前が null の行があると not null に戻せない。先に埋める
update meals set name = '(名前なし)' where name is null;
alter table meals alter column name set not null;

drop index if exists meals_date_eaten_at_idx;
alter table meals drop column if exists eaten_at;
