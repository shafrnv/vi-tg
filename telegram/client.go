package telegram

import (
	"fmt"
	"time"

	"gopkg.in/telebot.v3"
)

type Client struct {
	bot *telebot.Bot
}

type Chat struct {
	ID   int64
	Name string
	Type string
}

type Message struct {
	ID        int
	Text      string
	From      string
	Timestamp time.Time
	ChatID    int64
}

func NewClient(token string) (*Client, error) {
	pref := telebot.Settings{
		Token:  token,
		Poller: &telebot.LongPoller{Timeout: 10 * time.Second},
	}

	bot, err := telebot.NewBot(pref)
	if err != nil {
		return nil, fmt.Errorf("ошибка создания бота: %w", err)
	}

	return &Client{bot: bot}, nil
}

func (c *Client) SendMessage(chatID int64, text string) error {
	chat := &telebot.Chat{ID: chatID}
	_, err := c.bot.Send(chat, text)
	if err != nil {
		return fmt.Errorf("ошибка отправки сообщения: %w", err)
	}
	return nil
}

func (c *Client) GetChats() ([]Chat, error) {
	// В реальном приложении здесь нужно получить список чатов
	// Для демонстрации возвращаем тестовые данные
	chats := []Chat{
		{ID: 1, Name: "Общий чат", Type: "group"},
		{ID: 2, Name: "Тестовый чат", Type: "private"},
	}
	return chats, nil
}

func (c *Client) GetMessages(chatID int64, limit int) ([]Message, error) {
	// В реальном приложении здесь нужно получить сообщения из чата
	// Для демонстрации возвращаем тестовые данные
	messages := []Message{
		{
			ID:        1,
			Text:      "Привет! Как дела?",
			From:      "Пользователь1",
			Timestamp: time.Now().Add(-time.Hour),
			ChatID:    chatID,
		},
		{
			ID:        2,
			Text:      "Все хорошо, спасибо!",
			From:      "Пользователь2",
			Timestamp: time.Now().Add(-30 * time.Minute),
			ChatID:    chatID,
		},
	}
	return messages, nil
}

func (c *Client) StartPolling() {
	c.bot.Start()
}

func (c *Client) Stop() {
	c.bot.Stop()
} 