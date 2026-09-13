-- **これを流すと穴が開く。** up の正確な逆で、PostgREST から public スキーマが
-- また誰でも読み書きできる状態に戻る。
--
-- 巻き戻しの手段として置いてあるだけで、通常は流さない。
-- 流すなら、Supabase ダッシュボードでサインアップを閉じてあることを先に確認する。

alter default privileges in schema public grant all on tables to anon, authenticated;
alter default privileges in schema public grant all on sequences to anon, authenticated;

grant all on all tables in schema public to anon, authenticated;
grant all on all sequences in schema public to anon, authenticated;

do $$
declare t record;
begin
  for t in
    select tablename from pg_tables where schemaname = 'public'
  loop
    execute format('alter table public.%I disable row level security', t.tablename);
  end loop;
end $$;
