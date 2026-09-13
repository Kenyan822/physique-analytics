-- 大会（要件 P-04）。
--
-- カウントダウンと必要ペース判定（要件 A-10）の入力になる。
-- private/config.json の contests を移す。

create table contests (
  id          uuid        primary key default gen_random_uuid(),
  -- 開催月。日付が決まる前から登録したいので YYYY-MM で持ちたいが、
  -- date 型にして月末を入れる。**月内なら最終日が一番遠い＝安全側**の
  -- 見積もりになり、必要ペースを過小評価しない
  held_on     date        not null,
  category    text        not null check (length(category) between 1 and 200),
  -- ステージ体脂肪率の目標
  target_bf_pct numeric(4,1) not null check (target_bf_pct > 0 and target_bf_pct < 50),
  -- 「完走・経験」「入賞」など、その大会での位置づけ
  goal        text        check (goal is null or length(goal) <= 200),

  created_at  timestamptz not null default now(),
  updated_at  timestamptz not null default now(),
  deleted_at  timestamptz
);

-- 次の大会を引くのが主な用途
create index contests_held_on_idx on contests (held_on) where deleted_at is null;
