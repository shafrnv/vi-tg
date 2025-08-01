# vi-tg: Гибридная архитектура Go + Rust

## Описание архитектуры

Проект использует гибридную архитектуру:
- **Go Backend** - отвечает за авторизацию, синхронизацию с Telegram API, бизнес-логику
- **Rust Frontend** - отвечает за отрисовку TUI интерфейса, пользовательский опыт

## Компоненты

### Go Backend (порт 8080)
- `backend/server.go` - HTTP API сервер
- `auth/mtproto.go` - авторизация через MTProto
- `config/config.go` - управление конфигурацией
- `telegram/client.go` - клиент для Telegram Bot API

### Rust Frontend
- `src/main.rs` - точка входа и основной цикл TUI
- `src/api.rs` - HTTP клиент для связи с Go бэкендом
- `src/app.rs` - логика приложения и управление состоянием
- `src/ui.rs` - отрисовка TUI интерфейса с помощью ratatui

## API Endpoints

### Авторизация
- `GET /api/auth/status` - проверка статуса авторизации
- `POST /api/auth/phone` - установка номера телефона
- `POST /api/auth/code` - отправка кода подтверждения

### Чаты и сообщения
- `GET /api/chats` - получение списка чатов
- `GET /api/chats/{chat_id}/messages` - получение сообщений из чата
- `POST /api/chats/{chat_id}/messages` - отправка сообщения

### Стикеры
- `GET /api/stickers/{sticker_id}` - скачивание стикера

## Установка и запуск

### Требования
- Go 1.23+
- Rust 1.70+
- Telegram API credentials (api_id, api_hash)

### Настройка
1. Создайте файл конфигурации:
   ```bash
   mkdir -p ~/.vi-tg
   ```

2. Добавьте ваши Telegram API credentials в `~/.vi-tg/config.json`:
   ```json
   {
     "telegram_token": "",
     "phone_number": "",
     "use_mtproto": true,
     "theme": "default",
     "auto_save": true
   }
   ```

### Запуск

#### Способ 1: Использование скриптов
```bash
# Терминал 1 - Go бэкенд
./run_backend.sh

# Терминал 2 - Rust фронтенд
./run_frontend.sh
```

#### Способ 2: Ручной запуск
```bash
# Терминал 1 - Go бэкенд
cd backend
go run server.go

# Терминал 2 - Rust фронтенд
cargo run
```

## Использование

### Авторизация
1. При первом запуске Rust фронтенда введите номер телефона
2. Введите код подтверждения, который придет в Telegram
3. После успешной авторизации откроется основной интерфейс

### Основной интерфейс
- **↑/↓** - навигация по чатам
- **Enter** - выбор чата
- **i** - ввод сообщения
- **r/F5** - обновление данных
- **q** - выход

### Ввод сообщений
- **Enter** - отправить сообщение
- **Esc** - отменить ввод

## Преимущества архитектуры

### Go Backend
- Отличная экосистема для работы с Telegram API
- Простая разработка HTTP API
- Горутины для конкурентной обработки
- Стабильная работа с сетью

### Rust Frontend
- Безопасность памяти
- Отличная производительность TUI
- Мощная библиотека ratatui
- Отзывчивый пользовательский интерфейс

## Разработка

### Структура проекта
```
vi-tg/
├── backend/
│   └── server.go          # HTTP API сервер
├── auth/
│   └── mtproto.go         # MTProto авторизация
├── config/
│   └── config.go          # Конфигурация
├── telegram/
│   └── client.go          # Telegram Bot API
├── src/
│   ├── main.rs            # Rust точка входа
│   ├── api.rs             # HTTP клиент
│   ├── app.rs             # Логика приложения
│   └── ui.rs              # TUI интерфейс
├── api_contract.md        # API контракт
├── Cargo.toml             # Rust зависимости
└── go.mod                 # Go зависимости
```

### Добавление новых функций

#### Go Backend
1. Добавьте новый endpoint в `backend/server.go`
2. Реализуйте бизнес-логику
3. Обновите API контракт в `api_contract.md`

#### Rust Frontend
1. Добавьте новый метод в `src/api.rs`
2. Обновите логику приложения в `src/app.rs`
3. Добавьте UI элементы в `src/ui.rs`

## Отладка

### Go Backend
```bash
# Проверка статуса сервера
curl http://localhost:8080/health

# Проверка авторизации
curl http://localhost:8080/api/auth/status
```

### Rust Frontend
```bash
# Проверка компиляции
cargo check

# Запуск с отладкой
RUST_LOG=debug cargo run
```

## Известные проблемы

1. **Авторизация**: Требует правильные API credentials
2. **Сетевые ошибки**: Rust фронтенд автоматически переподключается к Go бэкенду
3. **Стикеры**: Требуют дополнительную обработку для отображения

## Будущие улучшения

- [ ] WebSocket для real-time обновлений
- [ ] Поддержка медиа файлов
- [ ] Улучшенная обработка стикеров
- [ ] Настройки тем оформления
- [ ] Поддержка групповых чатов
- [ ] Шифрование локальных данных 