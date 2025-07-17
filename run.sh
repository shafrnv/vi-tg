#!/bin/bash

echo "🚀 Запуск vi-tg - Telegram TUI клиент"
echo "📱 Демонстрационная версия с тестовыми данными"
echo ""
echo "Управление:"
echo "  ↑/↓  - Навигация по чатам"
echo "  Enter - Загрузить сообщения"
echo "  Ctrl+Q - Выход"
echo ""
echo "Для подключения к реальному Telegram API:"
echo "  📖 Смотрите TELEGRAM_SETUP.md"
echo ""
echo "Запуск..."
echo ""

RUST_LOG=info cargo run 