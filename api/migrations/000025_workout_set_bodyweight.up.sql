-- 自重の種目を記録できるようにする（#335）。
--
-- 懸垂・ディップス・腕立ては weight_kg に入れる数字が無い。
-- **0 を入れてはいけない。** トン数も e1RM も 0 になり、何セットやっても
-- ボリュームに乗らない。分析が壊れる。
--
-- 「自重でやった」という事実と「その日の体重」は別の情報なので分けて持つ。
-- 後から体重を直したら挙上重量も直るのが正しい。
--
-- 加重（懸垂＋10kg）は bodyweight = true かつ weight_kg = 10 で表す。
-- weight_kg は **体重に足す追加分**になる。
alter table workout_sets
  add column bodyweight boolean not null default false;

comment on column workout_sets.bodyweight is
  '自重の種目か。立っているときの挙上重量は「その日の体重 + weight_kg」（#335）';
comment on column workout_sets.weight_kg is
  'kg。bodyweight が立っているときは体重に足す追加分（加重）で、自重だけなら 0';
