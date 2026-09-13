-- **これを流すと穴が開く。** up の正確な逆で、PostgREST から public スキーマが
-- また誰でも読み書きできる状態に戻る。
--
-- 巻き戻しの手段として置いてあるだけで、通常は流さない。
-- 流すなら、Supabase ダッシュボードでサインアップを閉じてあることを先に確認する。

-- anon / authenticated は Supabase が作るロール。素の Postgres には無いので、
-- 居るときだけ戻す（up と同じ理由）
do $$
declare r text;
begin
  foreach r in array array['anon', 'authenticated'] loop
    if not exists (select 1 from pg_roles where rolname = r) then
      continue;
    end if;

    execute format(
      'alter default privileges in schema public grant all on tables to %I', r);
    execute format(
      'alter default privileges in schema public grant all on sequences to %I', r);
    execute format('grant all on all tables in schema public to %I', r);
    execute format('grant all on all sequences in schema public to %I', r);
  end loop;
end $$;

do $$
declare t record;
begin
  for t in
    select tablename from pg_tables where schemaname = 'public'
  loop
    execute format('alter table public.%I disable row level security', t.tablename);
  end loop;
end $$;
