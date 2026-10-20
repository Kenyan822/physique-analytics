-- daily_metrics の kcal / PFC を使わないことを列コメントに残す（#316）。
--
-- **摂取は meals が正**（#325 で分析側もそちらを見るようにした）。
-- この4列は誰も書かないまま残っていて、API からも外した。
--
-- **列はまだ落とさない。** 落とすのは戻せない操作なので、
-- API から消えた状態でしばらく様子を見てから別で判断する。
-- MCP の query は Claude がスキーマを見て SQL を書くので、
-- コメントで「使っていない」と分かるようにしておく。
comment on column daily_metrics.kcal is
  '使っていない。摂取は meals を日付で合計する（#316 / #325）';
comment on column daily_metrics.protein_g is
  '使っていない。摂取は meals を日付で合計する（#316 / #325）';
comment on column daily_metrics.fat_g is
  '使っていない。摂取は meals を日付で合計する（#316 / #325）';
comment on column daily_metrics.carb_g is
  '使っていない。摂取は meals を日付で合計する（#316 / #325）';
