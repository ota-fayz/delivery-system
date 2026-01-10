-- Удаление триггера для promo_codes
DROP TRIGGER IF EXISTS update_promo_codes_updated_at ON promo_codes;

-- Удаление индексов
DROP INDEX IF EXISTS idx_orders_promo_code;
DROP INDEX IF EXISTS idx_promo_codes_valid_until;
DROP INDEX IF EXISTS idx_promo_codes_is_active;
DROP INDEX IF EXISTS idx_promo_codes_code;

-- Удаление полей из таблицы orders
ALTER TABLE orders
    DROP COLUMN IF EXISTS original_amount,
    DROP COLUMN IF EXISTS discount_amount,
    DROP COLUMN IF EXISTS promo_code;

-- Удаление таблицы промокодов
DROP TABLE IF EXISTS promo_codes;
