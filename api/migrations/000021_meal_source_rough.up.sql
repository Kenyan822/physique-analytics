-- ざっくり入力（要件 N-01 / #252）。飲み会・外食は1品ずつ記録できず、
-- 記録しないとその日が丸ごと欠測になる。**精度より、記録が残ることを優先する。**
--
-- kcal だけ入れて PFC を既定比で按分した記録を `rough` で区別する。
-- `manual`（自分で計った）と混ぜると、推定の良し悪しを後から検証できなくなる。
-- 足すだけなので旧コードは影響を受けない（旧コードは rough を書かない）。
alter table meals drop constraint meals_source_check;
alter table meals add constraint meals_source_check
  check (source in ('manual', 'ai_estimated', 'rough'));
