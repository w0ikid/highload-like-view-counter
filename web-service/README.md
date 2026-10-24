# Web service

Минимальный frontend: React + TypeScript + Vite. Стартовая страница и проверка `/readyz` работают; тестовые посты и действия подключим после реализации backend API.

```sh
cd web-service
npm ci
cp .env.example .env
npm run dev
```

Открыть http://localhost:5173. Нужен Node.js 22.12+ (или совместимая более новая версия). Go API запускается отдельно по корневому README.

Vite перенаправляет `/api/*` в Go API на `http://localhost:8080`, удаляя префикс `/api`. Другой адрес задаётся через `API_PROXY_TARGET` в `.env`; после изменения перезапустите dev-сервер. Секреты в переменные `VITE_*` помещать нельзя: они доступны браузеру.

```sh
npm run typecheck
npm run build
npm run preview
```

Сборка сохраняется в `dist/`. `preview` служит для просмотра сборки; proxy подключения к API настроен только для dev-сервера. При публикации нужно настроить `/api/*` на стороне reverse proxy. Docker и production hosting frontend пока не подключены. UI не участвует в измерении протоколов.

Страница `/reset-password` обрабатывает ссылку из MailHog, устанавливает новый пароль и не сохраняет токен в localStorage. Формы регистрации и входа пока не добавлены; auth API описан в `../docs/auth.md`.
