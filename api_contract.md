# API Контракт для vi-tg

## Архитектура

- **Go Backend**: HTTP сервер на порту 8080, отвечает за авторизацию, синхронизацию с Telegram API
- **Rust Frontend**: TUI клиент, отправляет HTTP запросы к Go бэкенду

## Endpoints

### 1. Статус авторизации
```
GET /api/auth/status
Response: {
  "authorized": boolean,
  "phone_number": string|null,
  "needs_code": boolean
}
```

### 2. Установка номера телефона
```
POST /api/auth/phone
Body: {
  "phone": string
}
Response: {
  "success": boolean,
  "message": string,
  "needs_code": boolean
}
```

### 3. Отправка кода подтверждения
```
POST /api/auth/code
Body: {
  "code": string
}
Response: {
  "success": boolean,
  "message": string,
  "authorized": boolean
}
```

### 4. Получение списка чатов
```
GET /api/chats
Response: {
  "chats": [
    {
      "id": int64,
      "title": string,
      "type": string,
      "unread": int,
      "last_message": string|null
    }
  ]
}
```

### 5. Получение сообщений из чата
```
GET /api/chats/{chat_id}/messages?limit=50
Response: {
  "messages": [
    {
      "id": int,
      "text": string,
      "from": string,
      "timestamp": string,
      "chat_id": int64,
      "type": string,
      "sticker_id": int64|null,
      "sticker_emoji": string|null,
      "sticker_path": string|null
    }
  ]
}
```

### 6. Отправка сообщения
```
POST /api/chats/{chat_id}/messages
Body: {
  "text": string
}
Response: {
  "success": boolean,
  "message": string,
  "message_id": int|null
}
```

### 7. Скачивание стикера
```
GET /api/stickers/{sticker_id}
Response: Binary data (image file)
```

## Структуры данных

### Chat
```json
{
  "id": int64,
  "title": string,
  "type": "user|group|channel",
  "unread": int,
  "last_message": string|null
}
```

### Message
```json
{
  "id": int,
  "text": string,
  "from": string,
  "timestamp": "2024-01-01T12:00:00Z",
  "chat_id": int64,
  "type": "text|sticker|photo|video|document",
  "sticker_id": int64|null,
  "sticker_emoji": string|null,
  "sticker_path": string|null
}
```

### AuthStatus
```json
{
  "authorized": boolean,
  "phone_number": string|null,
  "needs_code": boolean
}
```

## Ошибки

Все ошибки возвращаются в формате:
```json
{
  "error": string,
  "code": int
}
```

HTTP статус коды:
- 200: Успех
- 400: Неверный запрос
- 401: Не авторизован
- 404: Не найдено
- 500: Внутренняя ошибка сервера

## Особенности

1. **Авторизация**: Состояние авторизации хранится в Go бэкенде
2. **Стикеры**: Скачиваются и кешируются в Go бэкенде, путь возвращается в API
3. **Polling**: Rust фронтенд периодически опрашивает API для получения новых сообщений
4. **Конфигурация**: Настройки хранятся в Go бэкенде 