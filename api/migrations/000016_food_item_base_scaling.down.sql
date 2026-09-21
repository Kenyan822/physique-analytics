alter table food_items drop constraint if exists food_items_scaling_needs_basis;

alter table food_items
  drop column if exists scales_with_amount,
  drop column if exists base_unit,
  drop column if exists base_amount;
