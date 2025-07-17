# Настройка Telegram API

Данный документ описывает варианты интеграции с реальным Telegram API для vi-tg.

## Вариант 1: rust-tdlib (Рекомендуется)

### Описание
`rust-tdlib` - это Rust обертка для официальной библиотеки TDLib от Telegram. Предоставляет полный доступ к Telegram API.

### Преимущества
- Полный доступ к Telegram API
- Официальная поддержка от Telegram
- Поддержка всех типов сообщений
- Локальная база данных для кэширования
- Автоматическое управление сессиями

### Установка TDLib

#### Arch Linux
```bash
sudo pacman -S tdlib
```

#### Ubuntu/Debian
```bash
sudo apt-get update
sudo apt-get install libtdjson-dev
```

#### Из исходников
```bash
git clone https://github.com/tdlib/td.git
cd td
mkdir build
cd build
cmake -DCMAKE_BUILD_TYPE=Release ..
make -j$(nproc)
sudo make install
```

### Настройка

1. Получите API ID и API Hash:
   - Перейдите на https://my.telegram.org/auth
   - Войдите в свой аккаунт Telegram
   - Перейдите в "API development tools"
   - Создайте новое приложение
   - Сохраните `api_id` и `api_hash`

2. Обновите конфигурацию в `~/.vi-tg/config.json`:
```json
{
  "api_id": 12345678,
  "api_hash": "your_api_hash_here",
  "phone_number": "+1234567890",
  "use_tdlib": true,
  "theme": "default",
  "auto_save": true
}
```

### Текущая реализация

Проект уже настроен для использования `rust-tdlib`:
- Добавлена зависимость `rust-tdlib = "0.4.3"` в `Cargo.toml`
- Реализован `TdlibClient` в `src/telegram.rs`
- Обновлен `AuthManager` для работы с TDLib в `src/auth.rs`
- Конфигурация поддерживает параметры TDLib

### Использование

```rust
// Создание клиента
let client = TdlibClient::new();

// Инициализация с API параметрами
client.initialize(api_id, api_hash).await?;

// Получение чатов
let chats = client.get_chats().await?;

// Получение сообщений
let messages = client.get_messages(chat_id, 50).await?;

// Отправка сообщения
client.send_message(chat_id, "Привет!").await?;
```

## Вариант 2: TDLib напрямую

### Описание
Использование TDLib напрямую через C API с помощью биндингов.

### Установка
```bash
# Добавьте в Cargo.toml
[dependencies]
tdlib = "0.3"
```

### Пример использования
```rust
use tdlib::client::Client;
use tdlib::types::*;

let client = Client::new();
let auth_state = client.get_authorization_state().await?;
// Обработка авторизации...
```

## Вариант 3: Bot API (Ограниченный)

### Описание
Использование Telegram Bot API для ботов. Ограниченная функциональность.

### Настройка
1. Создайте бота через @BotFather
2. Получите токен бота
3. Обновите конфигурацию:
```json
{
  "telegram_token": "your_bot_token_here",
  "use_tdlib": false
}
```

### Ограничения
- Работает только с ботами
- Нет доступа к личным сообщениям
- Ограниченный набор API методов

## Вариант 4: Python FFI

### Описание
Использование Python библиотек (Pyrogram, Telethon) через FFI.

### Установка
```bash
pip install pyrogram telethon
```

### Пример интеграции
```rust
use pyo3::prelude::*;

#[pyfunction]
fn get_messages(chat_id: i64) -> PyResult<Vec<String>> {
    // Вызов Python кода
}
```

## Рекомендации

1. **Для полноценного клиента**: Используйте rust-tdlib (Вариант 1)
2. **Для ботов**: Используйте Bot API (Вариант 3)
3. **Для прототипирования**: Используйте Mock клиент (текущая реализация)

## Безопасность

- Никогда не коммитьте API ключи в репозиторий
- Используйте переменные окружения для чувствительных данных
- Регулярно обновляйте зависимости

## Отладка

Для включения логирования TDLib:
```bash
export RUST_LOG=debug
export TDLIB_LOG_LEVEL=2
```

## Поддержка

- [Документация TDLib](https://core.telegram.org/tdlib)
- [rust-tdlib на GitHub](https://github.com/antonio-antuan/rust-tdlib)
- [Telegram API документация](https://core.telegram.org/api) 