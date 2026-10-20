-- 「全部」の引数を 0 に戻す。
--
-- **元の値は分からない。** 戻す先は「壊れていた状態」なので、
-- 0 に落とすのが up の逆になる。
update food_item_components
set protein_g = 0, fat_g = 0, carb_g = 0
where covers_all;
