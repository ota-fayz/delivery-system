-- Удаляем таблицу отзывов (индексы удалятся автоматически)
DROP TABLE IF EXISTS reviews;

-- Удаляем добавленные колонки из couriers
ALTER TABLE couriers 
    DROP COLUMN IF EXISTS rating,
    DROP COLUMN IF EXISTS total_reviews;
