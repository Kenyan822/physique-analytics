-- HealthKit から入った体脂肪率が 100 分の 1 で保存されていた（#291）。
--
-- `HKUnit.percent()` は割合を返す（18.2% なら 0.182）。それをそのまま
-- パーセントとして保存していた。bodyfat_pct は numeric(4,1) なので
-- 0.182 が 0.2 に丸められ、**元の値は復元できない。**
--
-- 推測で 100 倍すると（0.2 → 20）嘘の値が残る。15% でも 25% でも
-- 0.2 になりうるため。null にして HealthKit から取り込み直す。
--
-- 1 未満の体脂肪率はありえないので、これで壊れた行だけを選べる。
update daily_metrics
set bodyfat_pct = null
where bodyfat_pct is not null and bodyfat_pct < 1;
