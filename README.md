# Highload

Aгрегатор доставки еды по модели Яндекс.Еды для России и СНГ.

## Команда

Андреев Кирилл, Сиденко Олег, Крамарь Данил, Пашкин Андрей, Войтенко Денис

## Как запустить

Из корня репозитория:

```bash
docker-compose up --build
```

Дождаться зелёных healthcheck.

Остановка: `docker-compose down` (с томом БД: `docker-compose down -v`).

Скрипты из `deploy/init-db/` применяются Postgres автоматически **только при первой инициализации volume**.
Если меняли `002_mock_data.sql`, поднимайте с пересозданием тома:

```bash
docker-compose down -v && docker-compose up --build
```

## Генерация большого mock data

По умолчанию генерируется:
- ~1000 ресторанов
- 12 категорий на ресторан
- 100 позиций меню на ресторан

Команда:

```bash
python3 scripts/generate_mock_data.py
```

Файл назначения по умолчанию: `deploy/init-db/002_mock_data.sql`.
Параметры можно менять, например:

```bash
python3 scripts/generate_mock_data.py --restaurants 1500 --menu-items-per-restaurant 120 --seed 2026
```

## Как проверить (curl)

Свой хост вместо `127.0.0.1` при проверке с другой машины.

**Поиск ресторанов** (`lat`/`lon` обязательны):

```bash
curl -sS "http://127.0.0.1:8080/api/v1/restaurants?lat=55.75&lon=37.61&radius=5000"
```

Ожидается `200`, JSON с полями `restaurants`, `total`, `page`, `limit`.

**Меню ресторана**:

```bash
curl -sS "http://127.0.0.1:8080/api/v1/restaurants/f47ac10b-58cc-4372-a567-0e02b2c3d479/menu"
```

Ожидается `200`, JSON с `restaurant_id`, `categories[]`.

**Создание заказа**:

```bash
curl -sS -X POST "http://127.0.0.1:8080/api/v1/orders" \
  -H "Content-Type: application/json" \
  -d '{
    "restaurant_id": "f47ac10b-58cc-4372-a567-0e02b2c3d479",
    "items": [
      {"menu_item_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890", "quantity": 2},
      {"menu_item_id": "e3f4a5b6-c7d8-9012-cdef-123456789012", "quantity": 1}
    ],
    "delivery_address": {"lat": 55.7558, "lon": 37.6173, "address_text": "ул. Тверская, д. 1"},
    "comment": "Не звонить в дверь"
  }'
```

Ожидается `201`, в теле `order_id`, `status: "created"`, `items`, `total_amount`, `estimated_delivery`, `created_at`.

**Оплата (асинхронно)** - заголовок `Idempotency-Key` обязателен:

```bash
export OID="<order_id из предыдущего шага>"
curl -sS -X POST "http://127.0.0.1:8080/api/v1/orders/${OID}/pay" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: 550e8400-e29b-41d4-a716-446655440000" \
  -d '{"payment_method":"card","card_token":"tok_visa_4242"}'
```

Ожидается `202`, `status: "payment_pending"`.

**Трекинг** (после короткой паузы, пока payment-service обработает событие):

```bash
sleep 1
curl -sS "http://127.0.0.1:8080/api/v1/orders/${OID}/tracking"
```

Ожидается `200`, `status` в итоге `paid`, поля `status_history`, `estimated_delivery`, `updated_at`.

## Load-тест (k6)

Профиль сценария: **read-heavy** (частые списки ресторанов и меню, реже цепочка заказ->оплата->трекинг).

```bash
k6 run loadtest/smoke.k6
```

Для удалённой VM:

```bash
BASE_URL=http://<vm-ip>:8080 k6 run loadtest/smoke.k6
```

## Паттерны (проектирование + устойчивость)

| Паттерн | Зачем | Где в коде |
|---------|--------|------------|
| **API Gateway** | Единая точка входа, маршрутизация к catalog/order | [nginx/nginx.conf](nginx/nginx.conf) - `upstream` + `location` для `/api/v1/restaurants` и `/api/v1/orders` |
| **EDA (асинхронная оплата)** | Развязать order и payment, соответствие ADR-003 | [services/order-service/internal/transport/http/handlers.go](services/order-service/internal/transport/http/handlers.go) - `Pay` -> Kafka; [services/order-service/internal/events/kafka.go](services/order-service/internal/events/kafka.go) - `RunPaymentResultConsumer`; [services/payment-service/internal/worker/consumer.go](services/payment-service/internal/worker/consumer.go) - обработка `payment.requests` и публикация в `payment.results` |
| **Health check** | Readiness для compose и nginx | [services/catalog-service/internal/transport/http/health.go](services/catalog-service/internal/transport/http/health.go) и аналоги в order/payment `internal/transport/http`; [docker-compose.yml](docker-compose.yml) - `healthcheck`; gateway - `/health` |
| **Timeout (HTTP)** | Ограничить блокировку при недоступном catalog | [services/order-service/internal/catalogclient/client.go](services/order-service/internal/catalogclient/client.go) - `http.Client{Timeout: 5 * time.Second}`; [services/order-service/internal/transport/http/handlers.go](services/order-service/internal/transport/http/handlers.go) - `context.WithTimeout` на загрузку меню при создании заказа |
| **Идемпотентность оплаты** | Повтор `pay` с тем же ключом безопасен | [services/order-service/internal/store/order_store.go](services/order-service/internal/store/order_store.go) - метод `Pay` (ключ `Idempotency-Key` + `payments.idempotency_key`) |

## Итерации оптимизации

Таблица и шаблон замеров: [docs/optimization-log.md](docs/optimization-log.md)

## Документация

- [Архитектура](docs/architecture.md)
- [Анализ требования](docs/requirements.md)
- [ADR](docs/adr/)

