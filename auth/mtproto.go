package auth

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gotd/td/telegram"
	gotdauth "github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/tg"
	"golang.org/x/crypto/ssh/terminal"
)

type MTProtoClient struct {
	client *telegram.Client
	api    *tg.Client
}

type Dialog struct {
	ID       int64
	Title    string
	Type     string
	Unread   int
	LastMsg  string
}

type Message struct {
	ID        int
	Text      string
	From      string
	Timestamp time.Time
	ChatID    int64
}

// --- Кастомный UserAuthenticator для авторизации ---
type ConsoleAuth struct {
	PhoneNumber string
}

func (a *ConsoleAuth) Phone(ctx context.Context) (string, error) {
	return a.PhoneNumber, nil
}

func (a *ConsoleAuth) Password(ctx context.Context) (string, error) {
	fmt.Print("Введите пароль двухфакторной аутентификации: ")
	pw, err := terminal.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	return string(pw), err
}

func (a *ConsoleAuth) Code(ctx context.Context, sentCode *tg.AuthSentCode) (string, error) {
	fmt.Print("Введите код подтверждения: ")
	r := bufio.NewReader(os.Stdin)
	code, _ := r.ReadString('\n')
	return strings.TrimSpace(code), nil
}

func (a *ConsoleAuth) SignUp(ctx context.Context) (gotdauth.UserInfo, error) {
	fmt.Print("Введите имя: ")
	r := bufio.NewReader(os.Stdin)
	first, _ := r.ReadString('\n')
	fmt.Print("Введите фамилию: ")
	last, _ := r.ReadString('\n')
	return gotdauth.UserInfo{
		FirstName: strings.TrimSpace(first),
		LastName:  strings.TrimSpace(last),
	}, nil
}

func (a *ConsoleAuth) AcceptTermsOfService(ctx context.Context, tos tg.HelpTermsOfService) error {
	fmt.Println("Примите условия использования Telegram (Y/n): ")
	r := bufio.NewReader(os.Stdin)
	resp, _ := r.ReadString('\n')
	resp = strings.ToLower(strings.TrimSpace(resp))
	if resp == "n" {
		return fmt.Errorf("terms not accepted")
	}
	return nil
}

// --- Основная логика ---

func NewMTProtoClient() *MTProtoClient {
	return &MTProtoClient{}
}

func (m *MTProtoClient) AuthAndConnect(ctx context.Context, phone string) error {
	sessionPath := getSessionPath()
	client := telegram.NewClient(17349, "3446e2d45b85344a2a4f52c4f91d3659", telegram.Options{
		SessionStorage: &telegram.FileSessionStorage{Path: sessionPath},
	})
	
	userAuth := &ConsoleAuth{PhoneNumber: phone}
	authFlow := gotdauth.NewFlow(userAuth, gotdauth.SendCodeOptions{})

	err := client.Run(ctx, func(ctx context.Context) error {
		if err := client.Auth().IfNecessary(ctx, authFlow); err != nil {
			return fmt.Errorf("ошибка авторизации: %w", err)
		}
		m.api = client.API()
		return nil
	})
	if err != nil {
		return err
	}
	m.client = client
	return nil
}

func (m *MTProtoClient) GetDialogs(ctx context.Context) ([]Dialog, error) {
	if m.api == nil {
		return nil, fmt.Errorf("клиент не инициализирован")
	}

	dialogs, err := m.api.MessagesGetDialogs(ctx, &tg.MessagesGetDialogsRequest{
		Limit: 100,
	})
	if err != nil {
		return nil, fmt.Errorf("ошибка получения диалогов: %w", err)
	}

	var result []Dialog

	switch d := dialogs.(type) {
	case *tg.MessagesDialogs:
		for i, dialogRaw := range d.Dialogs {
			dialog, ok := dialogRaw.(*tg.Dialog)
			if !ok {
				continue
			}
			var title, typ string
			var id int64
			// Определяем тип и название
			switch peer := dialog.Peer.(type) {
			case *tg.PeerUser:
				id = int64(peer.UserID)
				for _, userRaw := range d.Users {
					if u, ok := userRaw.(*tg.User); ok && u.ID == peer.UserID {
						title = u.Username
						if title == "" {
							title = strings.TrimSpace(u.FirstName + " " + u.LastName)
						}
						break
					}
				}
				typ = "user"
			case *tg.PeerChat:
				id = int64(peer.ChatID)
				for _, chatRaw := range d.Chats {
					if c, ok := chatRaw.(*tg.Chat); ok && c.ID == peer.ChatID {
						title = c.Title
						break
					}
				}
				typ = "group"
			case *tg.PeerChannel:
				id = int64(peer.ChannelID)
				for _, chRaw := range d.Chats {
					if c, ok := chRaw.(*tg.Channel); ok && c.ID == peer.ChannelID {
						title = c.Title
						break
					}
				}
				typ = "channel"
			}
			if title == "" {
				title = "Неизвестный чат"
			}
			unread := dialog.UnreadCount // int, не указатель
			result = append(result, Dialog{
				ID:      id,
				Title:   title,
				Type:    typ,
				Unread:  unread,
				LastMsg: fmt.Sprintf("%d", i),
			})
		}
	default:
		return nil, fmt.Errorf("неизвестный тип диалогов")
	}
	return result, nil
}

func (m *MTProtoClient) GetMessages(ctx context.Context, peerID int64, limit int) ([]Message, error) {
	if m.api == nil {
		return nil, fmt.Errorf("клиент не инициализирован")
	}

	messagesRaw, err := m.api.MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{
		Peer:  &tg.InputPeerUser{UserID: peerID},
		Limit: limit, // исправлено на int
	})
	if err != nil {
		return nil, fmt.Errorf("ошибка получения сообщений: %w", err)
	}

	var result []Message
	if history, ok := messagesRaw.(*tg.MessagesMessages); ok {
		for _, msgRaw := range history.Messages {
			if message, ok := msgRaw.(*tg.Message); ok {
				ts := time.Unix(int64(message.Date), 0)
				result = append(result, Message{
					ID:        int(message.ID),
					Text:      message.Message,
					From:      fmt.Sprintf("%d", message.FromID),
					Timestamp: ts,
					ChatID:    peerID,
				})
			}
		}
	}
	return result, nil
}

func (m *MTProtoClient) SendMessage(ctx context.Context, peerID int64, text string) error {
	if m.api == nil {
		return fmt.Errorf("клиент не инициализирован")
	}

	// Отправляем сообщение
	_, err := m.api.MessagesSendMessage(ctx, &tg.MessagesSendMessageRequest{
		Peer: &tg.InputPeerUser{
			UserID: peerID,
		},
		Message: text,
	})
	
	return err
}

func getSessionPath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = "."
	}
	return filepath.Join(homeDir, ".vi-tg", "session.json")
} 