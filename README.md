# Trip Service

Сервис поездок TripGo: создаёт поездку, отдаёт её и завершает. Данные в
PostgreSQL, схема накатывается миграциями, создание и завершение поездки
атомарно пишут в две таблицы.

Лабораторная работа 2: разделение кода на слои, тесты, линтер, логи, метрики и
трейсы. API и схема БД остались как в первой работе. Задание со звёздочкой —
мутационное тестирование домена и usecase — выполнено.

## Запуск

Нужны Go 1.27.1 или новее и `tripgoctl` — он поднимает PostgreSQL, окружение
телеметрии и генерирует `.env`.

```bash
tripgoctl cluster start
tripgoctl environment start
tripgoctl connect

make migrate
make run
```

Конфигурация только из окружения. `.env` дополняет уже заданные переменные и
не публикуется. Без `DATABASE_URL` или `OTEL_EXPORTER_OTLP_ENDPOINT` сервис
падает на старте. При запуске выполняется `Ping` PostgreSQL; если БД недоступна,
сервис не стартует. Недоступность коллектора не мешает обрабатывать запросы.

Адрес по умолчанию — `http://localhost:8080`. Для остановки — Ctrl+C: сервис
дожидается текущих запросов и закрывает ресурсы в пределах `SHUTDOWN_TIMEOUT`.

## Переменные окружения

| Переменная | По умолчанию | Что настраивает |
|---|---|---|
| `HTTP_ADDR` | `:8080` | адрес HTTP-сервера |
| `HTTP_READ_TIMEOUT` | `5s` | таймаут чтения запроса |
| `HTTP_READ_HEADER_TIMEOUT` | `3s` | таймаут чтения заголовков |
| `HTTP_WRITE_TIMEOUT` | `10s` | таймаут записи ответа |
| `HTTP_IDLE_TIMEOUT` | `60s` | время жизни keep-alive соединения |
| `SHUTDOWN_TIMEOUT` | `10s` | бюджет на graceful shutdown |
| `LOG_LEVEL` | `info` | уровень логов |
| `DATABASE_URL` | — | **обязательная**, строка подключения к PostgreSQL |
| `DATABASE_MAX_CONNS` | `10` | максимум соединений в пуле |
| `DATABASE_MIN_CONNS` | `2` | минимум соединений в пуле |
| `DATABASE_MAX_CONN_LIFETIME` | `30m` | время жизни соединения |
| `DATABASE_CONNECT_TIMEOUT` | `5s` | таймаут подключения к БД на старте |
| `DATABASE_QUERY_TIMEOUT` | `3s` | таймаут запроса, BEGIN, COMMIT и ROLLBACK |
| `OTEL_SERVICE_NAME` | `trip-service` | имя сервиса в телеметрии |
| `OTEL_RESOURCE_ATTRIBUTES` | из `.env` | namespace и окружение сервиса |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | — | **обязательная**, адрес коллектора |
| `OTEL_EXPORTER_OTLP_PROTOCOL` | `http/protobuf` | протокол OTLP: HTTP/protobuf или gRPC |
| `OTEL_TRACES_SAMPLER` | `parentbased_always_on` | стратегия сэмплирования трейсов |
| `OTEL_TRACES_SAMPLER_ARG` | `1` | доля запросов для стратегий traceidratio |
| `OTEL_METRIC_EXPORT_INTERVAL_DURATION` | `5s` | период отправки метрик |
| `SERVICE_VERSION` | `local` | версия сборки в телеметрии |

Пример — в [`.env.example`](.env.example). Адреса БД и коллектора берутся из
рабочего `.env`, созданного `tripgoctl`. Некорректные таймауты, лимиты пула,
уровень логов и настройки OTel проверяются при старте.

## Makefile

| Цель | Что делает |
|---|---|
| `make generate` | генерация OpenAPI-кода и gomock-моков |
| `make migrate` | накатить миграции (`goose up`) |
| `make migrate-down` | откатить последнюю миграцию |
| `make migrate-status` | состояние миграций |
| `make run` | запустить сервис |
| `make build` | собрать бинарь в `bin/trip-service` |
| `make lint` | проверить код через golangci-lint |
| `make test` | unit- и HTTP-тесты с `-race -count=2`, отчёт о покрытии |
| `make test-integration` | тесты PostgreSQL с `-race -count=2` |
| `make test-mutation` | мутационные тесты домена и usecase |

Для `make lint` нужен `golangci-lint` v2.15.0:

```bash
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0
```

OpenAPI генерируется через `oapi-codegen`, моки — через `go:generate`.
Инструменты генерации закреплены в `go.mod`.

## API

| Метод | Путь | Ответы |
|---|---|---|
| `POST` | `/api/v1/trips` | `201`, `400`, `409`, `500` |
| `GET` | `/api/v1/trips/{tripId}` | `200`, `400`, `404`, `500` |
| `POST` | `/api/v1/trips/{tripId}/finish` | `200`, `400`, `404`, `409`, `500` |
| `GET` | `/health` | `200` |
| `GET` | `/ready` | `200`, `503` |

Источник истины — [OpenAPI-контракт](contracts/openapi/trip-service.openapi.yaml).
Типы и серверные интерфейсы генерируются из него и руками не правятся.

Ошибки — по RFC 9457 в `application/problem+json`, клиент смотрит на `code`:

| HTTP | `code` |
|---|---|
| `400` | `invalid_request` |
| `404` | `trip_not_found` |
| `409` | `trip_completed` |
| `409` | `driver_busy` |
| `500` | `internal_error` |

Успешные ответы — `application/json`. `/health` и `/ready` отдают `HealthResponse`
и в успешном, и в ошибочном случае.

```bash
curl -sS -X POST localhost:8080/api/v1/trips \
  -H 'content-type: application/json' \
  -d '{"user_id":"5cb72c04-7650-45c9-a79b-bcdba0631e0c",
       "driver_id":"8860b315-ec86-42eb-a17c-7c163d721ff5",
       "start_point":{"latitude":59.9398,"longitude":30.3146},
       "end_point":{"latitude":59.9290,"longitude":30.3626},
       "price":1450}'
```

Для следующих запросов подставь `id` из ответа:

```bash
TRIP_ID='<id из ответа>'
curl -sS localhost:8080/api/v1/trips/$TRIP_ID
curl -i -X POST localhost:8080/api/v1/trips/$TRIP_ID/finish
```

## Архитектура

| Пакет | Назначение |
|---|---|
| `cmd/trip-service` | конфигурация, запуск и обработка сигналов остановки |
| `internal/container` | создание и связывание зависимостей |
| `internal/domain/trip` | валидация, статусы, переходы и доменные ошибки |
| `internal/model` | структуры, передаваемые между слоями |
| `internal/usecase/trip/{create,get,finish}` | отдельный пакет на бизнес-операцию |
| `internal/http` | хендлеры по действиям, роутер, middleware и маппинг ошибок |
| `internal/repository/trip` | SQL-запросы поездок и истории статусов |
| `internal/infra` | PostgreSQL, менеджер транзакций, часы и телеметрия |
| `internal/generated` | сгенерированный OpenAPI-код |

Хендлеры знают только интерфейсы своих usecase. Usecase работают с доменом и
получают внешние зависимости через интерфейсы. Домен не импортирует HTTP,
драйвер БД или OpenTelemetry. Ограничения импортов проверяет `depguard`.

Интерфейсы неэкспортируемые и лежат в `contract.go` у потребителя: он определяет,
какие методы ему нужны. Моки генерируются рядом в `mocks_test.go`. Конкретные
реализации связываются в контейнере, который вызывается из `main`.

## Решения

### Домен и ошибки

В домене находятся валидация поездки и переход `active → completed`. Метод
`Finish` проверяет статус и выставляет время завершения. HTTP-хендлер разбирает
запрос и преобразует ответ; доменные ошибки переводятся в `Problem` в одном
пакете `internal/http/domainerror`. Внутренние ошибки БД в ответ не попадают.

### Транзакции и конкурентные запросы

Менеджер транзакций находится в `internal/infra/postgres/transaction`.
`Do` открывает транзакцию с уровнем `read committed` и передаёт её через контекст.
Успешный вызов заканчивается COMMIT, ошибка — ROLLBACK. При панике транзакция
откатывается, затем паника передаётся дальше. Для отката используется
`context.WithoutCancel` с таймаутом, вложенный `Do` присоединяется к текущей
транзакции.

Создание и завершение поездки записывают данные в `trips` и
`trip_status_history` внутри одного `Do`. Если запись истории падает, изменение
поездки тоже откатывается.

Две активные поездки одного водителя запрещает уникальный частичный индекс.
Его нарушение переводится в `409 driver_busy`. Два одновременных завершения
защищены условием `status = 'active'` в UPDATE: строку изменит только один запрос,
второй получит `409 trip_completed`, время завершения не перезапишется.

### `/health` и `/ready`

`/health` не ходит в БД и отвечает, пока работает процесс. `/ready` пингует БД
с таймаутом `DATABASE_QUERY_TIMEOUT` и отдаёт `503`, если она недоступна.

## Тесты

Unit-тесты проверяют доменные правила, usecase с gomock, валидацию, маппинг
ошибок и хендлеры через httptest. `make test` создаёт `coverage.out` и печатает
покрытие; у домена, usecase и health-хендлеров оно составляет 100%.

Для интеграционных тестов нужна БД из `.env` или `DATABASE_URL` с правом создавать
схемы. Тесты отделены тегом `integration`. Каждый тест создаёт отдельную схему,
применяет существующие миграции и удаляет её после проверки. Проверяются
жизненный цикл поездки, конфликты, конкурентные операции и откат транзакций.
Данные основной схемы не затрагиваются.

Для мутационных тестов нужны Python 3 и закреплённая версия `go-mutesting`.
Команда установки, результаты до/после и разбор всех выживших мутаций —
в [отчёте](reports/mutation/README.md).

## Логи, метрики и трейсы

Middleware идут в порядке `Recover → Tracing → Logging → Metrics → router`.
На HTTP-запрос пишется одна JSON-запись в stdout: метод, шаблон маршрута, статус,
длительность, `trace_id` и `span_id`. Создание и завершение поездки логируются
отдельно после commit. Recover ловит панику, пишет стек и отдаёт
`500 internal_error`.

HTTP инструментирован через `otelhttp`, пул БД — через `otelpgx`. Контекст
передаётся через W3C TraceContext. Трейсы и метрики уходят по OTLP в коллектор.

| Метрика | Тип |
|---|---|
| `http.server.request.duration` | histogram |
| `http.server.active_requests` | up-down counter |
| `db.client.operation.duration` | histogram |
| `tripgo.trips.created` | counter |
| `tripgo.trips.completed` | counter |
| `tripgo.trips.active` | gauge |

Ссылки на Jaeger и Grafana выводит `tripgoctl connect`. В Jaeger выбери
`trip-service` или найди трейс по `trace_id` из лога. В Grafana Explore выбери
Prometheus; примеры запросов — `tripgo_trips_created_total` и
`http_server_request_duration_seconds_count`. Скриншоты — в [screenshots/](screenshots/).
