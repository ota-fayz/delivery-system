-- Создание таблицы промокодов
CREATE TABLE promo_codes (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    code VARCHAR(50) NOT NULL UNIQUE,
    discount_type VARCHAR(20) NOT NULL CHECK (discount_type IN ('fixed_amount', 'percentage', 'free_delivery')),
    discount_value DECIMAL(10, 2) NOT NULL CHECK (discount_value >= 0),
    valid_from TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    valid_until TIMESTAMP WITH TIME ZONE NOT NULL,
    usage_limit INTEGER CHECK (usage_limit IS NULL OR usage_limit > 0),
    used_count INTEGER NOT NULL DEFAULT 0 CHECK (used_count >= 0),
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    CONSTRAINT valid_date_range CHECK (valid_until > valid_from)
);

-- Добавление полей для промокодов в таблицу заказов
ALTER TABLE orders
    ADD COLUMN promo_code VARCHAR(50),
    ADD COLUMN discount_amount DECIMAL(10, 2) NOT NULL DEFAULT 0 CHECK (discount_amount >= 0),
    ADD COLUMN original_amount DECIMAL(10, 2);

-- Обновляем существующие записи: устанавливаем original_amount равным total_amount
UPDATE orders SET original_amount = total_amount WHERE original_amount IS NULL;

-- Делаем original_amount NOT NULL после обновления существующих записей
ALTER TABLE orders ALTER COLUMN original_amount SET NOT NULL;

-- Индексы для оптимизации запросов
CREATE INDEX idx_promo_codes_code ON promo_codes(code);
CREATE INDEX idx_promo_codes_is_active ON promo_codes(is_active);
CREATE INDEX idx_promo_codes_valid_until ON promo_codes(valid_until);
CREATE INDEX idx_orders_promo_code ON orders(promo_code);

-- Триггер для автоматического обновления updated_at в promo_codes
CREATE TRIGGER update_promo_codes_updated_at
    BEFORE UPDATE ON promo_codes
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- Комментарии к таблице и колонкам
COMMENT ON TABLE promo_codes IS 'Таблица промокодов для скидок';
COMMENT ON COLUMN promo_codes.code IS 'Уникальный код промокода';
COMMENT ON COLUMN promo_codes.discount_type IS 'Тип скидки: fixed_amount (фиксированная сумма), percentage (процент), free_delivery (бесплатная доставка)';
COMMENT ON COLUMN promo_codes.discount_value IS 'Значение скидки (сумма для fixed_amount, процент для percentage, 0 для free_delivery)';
COMMENT ON COLUMN promo_codes.usage_limit IS 'Лимит использований (NULL = неограничено)';
COMMENT ON COLUMN promo_codes.used_count IS 'Количество использований промокода';
COMMENT ON COLUMN orders.promo_code IS 'Применённый промокод';
COMMENT ON COLUMN orders.discount_amount IS 'Сумма скидки';
COMMENT ON COLUMN orders.original_amount IS 'Оригинальная сумма заказа до применения скидки';
