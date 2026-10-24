# Базовый auth и доменная модель

Это расширение SIS2. За образец взяты разделение domain/usecase/infrastructure/transport, Credential, Session и Argon2id из qrgo-back. Приватные библиотеки, роли, SMS/OTP, KYC и связанные бизнес-сущности не переносились.

## Сущности

| Модель | Назначение |
| --- | --- |
| User | Положительный int64 ID, уникальный нормализованный email, имя, дата создания |
| Credential | Argon2id-хеш пароля, отдельный от профиля |
| Session | UUID сессии, пользователь, SHA-256 refresh-токена, срок, отзыв |
| PasswordReset | SHA-256 одноразового токена, пользователь, срок, дата использования |
| Post | Тестовый пост с ID и заголовком; создаётся seed-командой |
| Like | Пара post_id/user_id и дата; отдельного ID не нужно |
| PostCounter | Сохранённые likes/views |
| Counter | Проекция PostCounter с likedByMe |
| ViewEvent | post_id/viewer_id; viewer не обязан быть User |
| FlushBatch / ViewDelta / FlushLog | Стабильный UUID пачки, изменения просмотров и журнал применения |

Полные контракты auth-репозиториев находятся в domain, зависимости сервиса — в usecase/auth/interfaces.go, wiring — в bootstrap. У ещё не реализованных счётчиков пока только модели; искусственные пустые repository/service не добавлены.

## Правила

- Email приводится к нижнему регистру и обрезается по краям; уникальность обеспечивает PostgreSQL. Пароль: 8–128 Unicode-символов, без обрезки пробелов. Имя: 1–100 символов.
- Argon2id: 64 MiB памяти, одна итерация, четыре потока, случайная соль 16 байт. Верификатор принимает только поддерживаемый формат и параметры.
- Access JWT HS256: issuer/audience highload-counters, обязательный exp, TTL 15 минут. JWT содержит ID пользователя и сессии. Текущая активность сессии проверяется в PostgreSQL: logout/reset действуют и на access-токены.
- Refresh: 32 случайных байта, TTL 30 дней. В базе только SHA-256. Ротация отзывает старую сессию и создаёт новую в одной транзакции. Одновременный повтор выигрывает один раз; прежний access также перестаёт действовать. Клиент должен сериализовать refresh; при потере ответа требуется новый вход. Автоматических ретраев refresh нет.
- Reset: 32 случайных байта, TTL 15 минут. Новый запрос отменяет предыдущие reset-токены. Использование токена, изменение пароля и отзыв всех сессий атомарны. Email/пароль не подтверждают владение адресом при регистрации: email verification пока не реализована.
- Для login/refresh/reset используется единый порядок блокировки строки users. Login повторно проверяет хеш под блокировкой, поэтому вход со старым паролем не обойдёт конкурентный reset.
- Forgot-password возвращает одинаковый 202 для существующего/неизвестного email и сбоя SMTP. Тело не подтверждает доставку; сбой доставки фиксируется в логе без токена/email. Отправка синхронная, гарантированной доставки/outbox пока нет; временные различия ответов не маскируются.
- Auth POST ограничен 30 запросами в минуту на прямой IP через Redis, без доверия X-Forwarded-For. При недоступном limiter — 503. Общий лимит включает register/login/refresh/logout/reset. При reverse proxy нужно отдельно настроить доверенные адреса; сейчас пользователи за proxy делят лимит.
- Одновременно выполняется до четырёх auth POST на экземпляр API, чтобы ограничить память Argon2id. Общий auth budget 5 секунд; PostgreSQL-транзакция 500 мс, SMTP 2 секунды. Хеширование Argon2id уже начатой операции не прерывается, но дальнейшая запись получает отменённый context.
- Пароли, токены и SQL не попадают в JSON ошибок. Auth-ответы имеют Cache-Control: no-store. Токены возвращаются JSON, cookie-сессии не используются. Полный login/register UI пока не сделан; access/refresh не сохраняются frontend в localStorage.

SMTP-адаптер предназначен для локального MailHog: без внешней доставки, TLS и SMTP AUTH. Production auth и почтовая доставка потребуют отдельного этапа. Автоматической очистки истёкших sessions/password_resets пока нет.

## HTTP

| Метод | Путь | JSON |
| --- | --- | --- |
| POST | /v1/auth/register | email, password, name |
| POST | /v1/auth/login | email, password |
| POST | /v1/auth/refresh | refreshToken |
| POST | /v1/auth/logout | refreshToken |
| POST | /v1/auth/forgot-password | email |
| POST | /v1/auth/reset-password | token, password |
| GET | /v1/users/me | Authorization: Bearer accessToken |

Register/login/refresh возвращают accessToken, refreshToken, expiresAt. User ID в JSON передаётся десятичной строкой для сохранения int64 в браузере. Ошибки: code, message, requestId и необязательный field. Как в qrgo-back, domain.Error хранит Code/Message/Field и общий kind; transport определяет HTTP-статус по kind. Конкретные ошибки находятся рядом с сущностями: email_invalid, email_taken, user_name_invalid, password_invalid, invalid_credentials, token_invalid, token_expired. Сверх кодов SIS2 добавлены 409 для занятого email и 429 для лимита auth.

```sh
curl -X POST http://localhost:8080/v1/auth/register \
  -H 'Content-Type: application/json' \
  -d '{"email":"student@example.com","password":"example-password","name":"Student"}'

curl -X POST http://localhost:8080/v1/auth/forgot-password \
  -H 'Content-Type: application/json' \
  -d '{"email":"student@example.com"}'
```

Открыть MailHog http://localhost:8025, затем ссылку в письме. Для страницы reset запустить `cd web-service && npm run dev`. Секрет передаётся во fragment ссылки и удаляется из адресной строки после загрузки страницы; после перезагрузки откройте письмо повторно.

## Соотношение с SIS2

Auth доступен только через REST и не участвует в сравнении REST/gRPC счётчиков. Адаптеры счётчиков используют общий режим COUNTER_AUTH_MODE: test принимает X-User-ID / x-user-id; bearer принимает Authorization / authorization и проверяет активную сессию. По умолчанию без env — bearer, в учебном .env.example — test. `likes.user_id` остаётся положительным int64 без FK ради benchmark-ID; viewer_id независим от users. Auth endpoints никогда не доверяют X-User-ID. Не включайте test-режим на публичном API.

## Проверки

```sh
docker compose --env-file .env.example up -d postgres redis mailhog
TEST_POSTGRES_DSN='postgres://counters:counters_local@localhost:5432/counters?sslmode=disable' \
  go test ./... -race -count=1
```

Интеграционный тест создаёт случайную схему в тестовой БД, применяет обе миграции и удаляет только эту схему после проверки. Без TEST_POSTGRES_DSN он пропускается. Проверяет уникальность email, неправильный пароль, параллельный refresh/reset, отзыв access/refresh, одноразовость/истечение reset, logout и отмену context. Unit-тесты проверяют Argon2id и JWT.

HTTP + Redis + MailHog smoke (создаёт уникального тестового пользователя и письмо, без очистки общей БД):

```sh
docker compose --env-file .env.example up --build -d api
python3 test/integration/auth/smoke.py
```
