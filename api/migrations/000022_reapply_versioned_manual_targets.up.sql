-- **000019 が本番に当たらなかったのを直す**（#272）。
--
-- 000020（session_exercises）が 000019 より先にマージ・デプロイされ、
-- golang-migrate が 18 → 20 と進んだ。その後 000019 がマージされても
-- 「現在 20 > 19」なのでスキップされ、本番の manual_targets は古い単一行のまま、
-- schema_migrations だけが進んだ。
--
-- 結果、`/v1/targets/manual` と `/v1/targets/{date}` が
-- `column "starts_on" does not exist` で 500 を返していた。
--
-- **冪等にする。** 000019 が当たっている環境（ローカル・CI・新規構築）では
-- 何もしてはいけない。本番だけが古い形で残っている。
do $$
begin
  -- **current_schema() で見る。** 'public' を決め打ちすると、別スキーマで
  -- 検証したときに本番側のテーブルを見てしまい、テストが素通りする（実際に踏んだ）
  if exists (
    select 1 from information_schema.columns
    where table_schema = current_schema()
      and table_name = 'manual_targets'
      and column_name = 'starts_on'
  ) then
    return;   -- 000019 が当たっている。何もしない
  end if;

  -- ここから先は 000019 の up と同じ内容
  alter table manual_targets rename to manual_targets_single;
  alter index manual_targets_pkey rename to manual_targets_single_pkey;

  create table manual_targets (
    id          uuid         primary key default gen_random_uuid(),
    -- PUT（今日から）は名前を持たないので null を許す
    name        text         check (name is null or length(name) between 1 and 100),
    -- この日から適用。同じ日に2回変えたら上書き
    starts_on   date         not null unique,

    protein_g   numeric(6,1) not null check (protein_g >= 0 and protein_g <= 1000),
    fat_g       numeric(6,1) not null check (fat_g >= 0 and fat_g <= 1000),
    carb_g      numeric(6,1) not null check (carb_g >= 0 and carb_g <= 2000),

    created_at  timestamptz  not null default now()
  );

  -- 既存の1行は記録の最古日から始める。**既存の全日が今までどおり同じ目標で
  -- 評価され、移行による表示の変化がゼロになる。** profile.start_date は null
  insert into manual_targets (starts_on, protein_g, fat_g, carb_g, created_at)
  select least(
           (select min(date) from meals),
           (select min(date) from body_measurements),
           (select min(date) from workout_sessions),
           current_date),
         protein_g, fat_g, carb_g, updated_at
  from manual_targets_single;

  drop table manual_targets_single;

  comment on table manual_targets is
    '手で決めた摂取目標の履歴。starts_on 以降に適用する。あるときは自動計算（A-02）より優先する';

  -- PostgREST から見えないようにする（CLAUDE.md / #150）
  alter table manual_targets enable row level security;
end $$;
