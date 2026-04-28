INSERT INTO restaurants (id, name, cuisine_type, address, lat, lon, rating, avg_price, is_active, image_url, delivery_time_min)
VALUES
    ('f47ac10b-58cc-4372-a567-0e02b2c3d479', 'Суши Мастер', 'japanese', 'ул. Сушная, 1', 55.751244, 37.618423, 4.7, 800, true, 'https://example.domain/img1.jpg', 35),
    ('a1b2c3d4-e5f6-4789-a012-345678901234', 'Пицца Рим', 'italian', 'ул. Пепперони, 5', 55.755800, 37.617300, 4.5, 650, true, 'https://example.domain/img2.jpg', 40),
    ('b2c3d4e5-f6a7-4890-b123-456789012345', 'Бургер Клаб', 'american', 'ул. Булочная, 10', 55.748000, 37.620000, 4.2, 450, true, 'https://example.domain/img3.jpg', 30);

INSERT INTO menu_categories (id, restaurant_id, name, sort_order)
VALUES
    ('c1d2e3f4-a5b6-4789-c012-abcdef111111', 'f47ac10b-58cc-4372-a567-0e02b2c3d479', 'Роллы', 0),
    ('d2e3f4a5-b6c7-4890-d123-bcdef1222222', 'f47ac10b-58cc-4372-a567-0e02b2c3d479', 'Супы', 1);

INSERT INTO menu_items (id, category_id, name, description, price, image_url, is_available)
VALUES
    ('a1b2c3d4-e5f6-7890-abcd-ef1234567890', 'c1d2e3f4-a5b6-4789-c012-abcdef111111', 'Филадельфия', 'Лосось, сливочный сыр, огурец, рис', 590, 'https://example.domain/susi.jpg', true),
    ('b2c3d4e5-f6a7-8901-bcde-f12345678901', 'c1d2e3f4-a5b6-4789-c012-abcdef111111', 'Калифорния', 'Краб, авокадо, огурец', 520, 'https://example.domain/cal.jpg', true),
    ('e3f4a5b6-c7d8-9012-cdef-123456789012', 'd2e3f4a5-b6c7-4890-d123-bcdef1222222', 'Мисо-суп', 'Тофу, водоросли', 250, 'https://example.domain/miso.jpg', true);
