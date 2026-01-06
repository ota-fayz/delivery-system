-- Добавляем поля для рейтинга в таблицу couriers
ALTER TABLE couriers 
    ADD COLUMN rating DECIMAL(3, 2) DEFAULT NULL,
    ADD COLUMN total_reviews INTEGER NOT NULL DEFAULT 0 CHECK (total_reviews >= 0);

-- Создаём таблицу отзывов
CREATE TABLE reviews (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    order_id UUID NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    courier_id UUID NOT NULL REFERENCES couriers(id) ON DELETE CASCADE,
    rating INTEGER NOT NULL CHECK (rating >= 1 AND rating <= 5),
    comment TEXT,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    
    -- Один отзыв на один заказ
    UNIQUE(order_id)
);

-- Индексы для оптимизации запросов
CREATE INDEX idx_reviews_courier_id ON reviews(courier_id);
CREATE INDEX idx_reviews_order_id ON reviews(order_id);
CREATE INDEX idx_reviews_rating ON reviews(rating);
CREATE INDEX idx_reviews_created_at ON reviews(created_at);
