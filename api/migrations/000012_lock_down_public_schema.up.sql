-- public スキーマを PostgREST から締め出す。
--
-- **Supabase は Postgres の前に PostgREST を自動で立てる。** Go API を通らない
-- 別の入口で、既定では public スキーマが丸ごと公開される。
-- `anon`（匿名ロール）に全権限が付き、RLS も無いので、**サインアップすら不要で
-- 全テーブルを読み書きできる**状態だった。
--
-- anon key はブラウザの JS に入る前提の公開情報なので、隠して守ることはできない。
--
-- なぜ自動で公開されるか: Supabase には「postgres が作ったテーブルは
-- anon / authenticated に全権限を渡す」という既定権限がある。golang-migrate は
-- postgres で create table するため、**マイグレーションを足すたびに開く**。
--
-- アプリへの影響は無い。postgres は rolbypassrls = true で RLS を素通りし、
-- テーブルの所有者でもある。締め出されるのは anon / authenticated だけ。

-- 1. 既存テーブルの RLS を有効にする。
--
-- **ポリシーは作らない。** RLS が有効でポリシーが無いテーブルは、
-- 素通りできないロールから見ると「行が1つも無い」ように見える。
-- ここでやりたいのはまさにそれ（anon に何も見せない）。
do $$
declare t record;
begin
  for t in
    select tablename from pg_tables where schemaname = 'public'
  loop
    execute format('alter table public.%I enable row level security', t.tablename);
  end loop;
end $$;

-- 2 と 3 は anon / authenticated が居るときだけ実行する。
--
-- **この2つは Supabase が作るロールで、素の Postgres には無い。**
-- ローカルの docker compose と CI は素の Postgres なので、無条件に revoke すると
-- `role "anon" does not exist` で落ちる。無ければ剥がすものも無いので飛ばしてよい。
do $$
declare r text;
begin
  foreach r in array array['anon', 'authenticated'] loop
    if not exists (select 1 from pg_roles where rolname = r) then
      continue;
    end if;

    -- 2. 権限を剥がす。
    --
    -- RLS だけでも読めなくなるが、権限も剥がしておく。RLS の設定漏れが
    -- 1テーブルでもあったときに、そこだけ開くのを防ぐ（二重に守る）。
    execute format('revoke all on all tables in schema public from %I', r);
    execute format('revoke all on all sequences in schema public from %I', r);

    -- 3. **今後作るテーブルに自動で権限が付かないようにする。**
    --
    -- これが無いと、次のマイグレーションでテーブルを足した瞬間にまた開く。
    -- 1 と 2 は今あるものを閉じるだけで、再発は防げない。
    --
    -- 対象は「このロール（postgres）がこれから作るもの」。
    execute format(
      'alter default privileges in schema public revoke all on tables from %I', r);
    execute format(
      'alter default privileges in schema public revoke all on sequences from %I', r);
  end loop;
end $$;
