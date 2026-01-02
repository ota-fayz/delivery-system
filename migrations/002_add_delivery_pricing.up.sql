ALTER TABLE orders ADD COLUMN pickup_address TEXT;
ALTER TABLE orders ADD COLUMN delivery_cost DECIMAL(10, 2) DEFAULT 0 CHECK (delivery_cost >= 0);