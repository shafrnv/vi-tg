package auth

import (
	"bufio"
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
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
	ID      int64
	Title   string
	Type    string
	Unread  int
	LastMsg string
}

type Message struct {
	ID           int
	Text         string
	From         string
	Timestamp    time.Time
	ChatID       int64
	Type         string // "text", "sticker", "photo", "video", etc.
	StickerID    int64  // ID стикера если Type == "sticker"
	StickerEmoji string // Эмодзи стикера
	StickerPath  string // Путь к файлу стикера (если скачан)
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
	client := telegram.NewClient(19936415, "2721a01cc1e880707e42f3f56fee3448", telegram.Options{
		SessionStorage: &telegram.FileSessionStorage{Path: sessionPath},
	})

	userAuth := &ConsoleAuth{PhoneNumber: phone}
	authFlow := gotdauth.NewFlow(userAuth, gotdauth.SendCodeOptions{})

	// Создаем канал для сигнализации о завершении авторизации
	authDone := make(chan error, 1)

	// Запускаем клиент в горутине
	go func() {
		err := client.Run(ctx, func(ctx context.Context) error {
			// Авторизуемся
			if err := client.Auth().IfNecessary(ctx, authFlow); err != nil {
				return fmt.Errorf("ошибка авторизации: %w", err)
			}

			// Сохраняем API клиент
			m.api = client.API()
			m.client = client

			// Сигнализируем об успешной авторизации
			authDone <- nil

			// Держим соединение активным
			<-ctx.Done()
			return nil
		})

		// Если авторизация не прошла, отправляем ошибку
		select {
		case authDone <- err:
		default:
		}
	}()

	// Ждем успешной авторизации (но не закрытия соединения)
	select {
	case err := <-authDone:
		if err != nil {
			return err
		}
		return nil
	case <-time.After(60 * time.Second):
		return fmt.Errorf("таймаут авторизации")
	}
}

func (m *MTProtoClient) GetDialogs(ctx context.Context) ([]Dialog, error) {
	if m.api == nil {
		return nil, fmt.Errorf("клиент не инициализирован")
	}

	// Создаем новый контекст для получения диалогов
	dialogsCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	dialogs, err := m.api.MessagesGetDialogs(dialogsCtx, &tg.MessagesGetDialogsRequest{
		Limit:      100,
		OffsetPeer: &tg.InputPeerEmpty{}, // Добавляем обязательное поле
	})
	if err != nil {
		return nil, fmt.Errorf("ошибка получения диалогов: %w", err)
	}

	// Добавляем отладочную информацию

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
			default:
				fmt.Printf("Неизвестный тип peer: %T\n", dialog.Peer)
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
	case *tg.MessagesDialogsSlice: // Обрабатываем MessagesDialogsSlice аналогично
		for i, dialogRaw := range d.Dialogs {
			dialog, ok := dialogRaw.(*tg.Dialog)
			if !ok {
				continue
			}
			var title, typ string
			var id int64
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
			unread := dialog.UnreadCount
			result = append(result, Dialog{
				ID:      id,
				Title:   title,
				Type:    typ,
				Unread:  unread,
				LastMsg: fmt.Sprintf("%d", i),
			})
		}
	default:
		return nil, fmt.Errorf("неизвестный тип диалогов: %T", dialogs)
	}
	return result, nil
}

// processMessage обрабатывает сообщение и определяет его тип
func (m *MTProtoClient) processMessage(message *tg.Message, users []tg.UserClass, peerID int64) Message {
	ts := time.Unix(int64(message.Date), 0)
	fromName := "Неизвестный"
	if message.FromID != nil {
		switch fromPeer := message.FromID.(type) {
		case *tg.PeerUser:
			for _, userRaw := range users {
				if u, ok := userRaw.(*tg.User); ok && u.ID == fromPeer.UserID {
					fromName = u.Username
					if fromName == "" {
						fromName = strings.TrimSpace(u.FirstName + " " + u.LastName)
					}
					break
				}
			}
		}
	}

	// Определяем тип сообщения и обрабатываем стикеры
	msgType := "text"
	stickerID := int64(0)
	stickerEmoji := ""
	stickerPath := ""

	// Проверяем, есть ли медиа в сообщении
	if message.Media != nil {
		switch media := message.Media.(type) {
		case *tg.MessageMediaDocument:
			if media.Document != nil {
				if doc, ok := media.Document.(*tg.Document); ok {
					// Проверяем, является ли документ стикером
					for _, attr := range doc.Attributes {
						if stickerAttr, ok := attr.(*tg.DocumentAttributeSticker); ok {
							msgType = "sticker"
							stickerID = doc.ID
							stickerEmoji = stickerAttr.Alt
							// Скачиваем стикер
							stickerPath = downloadStickerFile(m.api, doc)
							break
						}
					}
				}
			}
		}
	}

	return Message{
		ID:           int(message.ID),
		Text:         message.Message,
		From:         fromName,
		Timestamp:    ts,
		ChatID:       peerID,
		Type:         msgType,
		StickerID:    stickerID,
		StickerEmoji: stickerEmoji,
		StickerPath:  stickerPath,
	}
}

func (m *MTProtoClient) GetMessages(ctx context.Context, peerID int64, limit int) ([]Message, error) {
	if m.api == nil {
		return nil, fmt.Errorf("клиент не инициализирован")
	}

	// Создаем новый контекст для получения сообщений
	messagesCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Определяем тип peer по ID
	var peer tg.InputPeerClass

	// Для простоты пока используем InputPeerUser, но можно доработать
	// для определения типа peer по ID
	peer = &tg.InputPeerUser{
		UserID: peerID,
	}

	messagesRaw, err := m.api.MessagesGetHistory(messagesCtx, &tg.MessagesGetHistoryRequest{
		Peer:  peer,
		Limit: limit,
	})
	if err != nil {
		return nil, fmt.Errorf("ошибка получения сообщений: %w", err)
	}

	var result []Message

	switch messages := messagesRaw.(type) {
	case *tg.MessagesMessages:
		for _, msgRaw := range messages.Messages {
			if message, ok := msgRaw.(*tg.Message); ok {
				result = append(result, m.processMessage(message, messages.Users, peerID))
			}
		}
	case *tg.MessagesMessagesSlice:
		for _, msgRaw := range messages.Messages {
			if message, ok := msgRaw.(*tg.Message); ok {
				result = append(result, m.processMessage(message, messages.Users, peerID))
			}
		}
	case *tg.MessagesChannelMessages:
		for _, msgRaw := range messages.Messages {
			if message, ok := msgRaw.(*tg.Message); ok {
				result = append(result, m.processMessage(message, messages.Users, peerID))
			}
		}
	default:
	}

	return result, nil
}

func (m *MTProtoClient) SendMessage(ctx context.Context, peerID int64, text string) error {
	if m.api == nil {
		return fmt.Errorf("клиент не инициализирован")
	}

	// Генерируем случайный ID для сообщения
	randomID, err := generateRandomID()
	if err != nil {
		return fmt.Errorf("ошибка генерации random_id: %w", err)
	}

	// Отправляем сообщение
	_, err = m.api.MessagesSendMessage(ctx, &tg.MessagesSendMessageRequest{
		Peer: &tg.InputPeerUser{
			UserID: peerID,
		},
		Message:  text,
		RandomID: randomID,
	})

	return err
}

// generateRandomID генерирует случайный 64-битный ID для сообщения
func generateRandomID() (int64, error) {
	// Генерируем случайное число от 1 до 2^63-1
	max := big.NewInt(0)
	max.SetBit(max, 63, 1)      // 2^63
	max.Sub(max, big.NewInt(1)) // 2^63 - 1

	n, err := rand.Int(rand.Reader, max)
	if err != nil {
		return 0, err
	}

	// Убеждаемся, что число положительное
	result := n.Int64()
	if result <= 0 {
		result = 1
	}

	return result, nil
}

func getSessionPath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = "."
	}
	return filepath.Join(homeDir, ".vi-tg", "session.json")
}

// downloadStickerFile скачивает файл стикера и возвращает путь к нему
func downloadStickerFile(api *tg.Client, doc *tg.Document) string {
	if api == nil || doc == nil {
		return ""
	}

	// Определяем расширение
	ext := ".webp"
	for _, attr := range doc.Attributes {
		if _, ok := attr.(*tg.DocumentAttributeImageSize); ok {
			ext = ".png"
		}
	}

	// Путь для сохранения
	fileName := fmt.Sprintf("/tmp/vi-tg_sticker_%d%s", doc.ID, ext)

	// Проверяем, не скачан ли уже файл
	if info, err := os.Stat(fileName); err == nil && info.Size() > 0 {
		return fileName
	}

	// Создаем файл
	f, err := os.Create(fileName)
	if err != nil {
		return ""
	}
	defer f.Close()

	// Скачиваем файл по частям
	offset := int64(0)
	chunkSize := int(512 * 1024) // 512KB чанки
	totalBytes := int64(0)

	for {

		resp, err := api.UploadGetFile(context.Background(), &tg.UploadGetFileRequest{
			Precise:      true,
			CDNSupported: false, // Отключаем CDN поддержку
			Location: &tg.InputDocumentFileLocation{
				ID:            doc.ID,
				AccessHash:    doc.AccessHash,
				FileReference: doc.FileReference,
			},
			Offset: offset,
			Limit:  chunkSize,
		})
		if err != nil {
			// Если файл не скачивается, возвращаем пустую строку
			os.Remove(fileName) // Удаляем пустой файл
			return ""
		}

		finished := false

		// Проверяем тип ответа и записываем данные
		switch data := resp.(type) {
		case *tg.UploadFile:
			if len(data.Bytes) == 0 {
				// Файл скачан полностью
				finished = true
			} else {
				// Записываем чанк в файл
				if _, err := f.Write(data.Bytes); err != nil {
					os.Remove(fileName)
					return ""
				}
				offset += int64(len(data.Bytes))
				totalBytes += int64(len(data.Bytes))

				// Если получили меньше данных чем запросили, значит файл закончился
				if len(data.Bytes) < chunkSize {
					finished = true
				}
			}
		case *tg.UploadFileCDNRedirect:
			// Скачиваем файл через CDN
			cdnResp, err := api.UploadGetCDNFile(context.Background(), &tg.UploadGetCDNFileRequest{
				FileToken: data.FileToken,
				Offset:    offset,
				Limit:     chunkSize,
			})
			if err != nil {
				os.Remove(fileName)
				return ""
			}

			switch cdnData := cdnResp.(type) {
			case *tg.UploadCDNFile:
				if len(cdnData.Bytes) == 0 {
					finished = true
				} else {
					// Записываем чанк в файл
					if _, err := f.Write(cdnData.Bytes); err != nil {
						os.Remove(fileName)
						return ""
					}
					offset += int64(len(cdnData.Bytes))
					totalBytes += int64(len(cdnData.Bytes))

					// Если получили меньше данных чем запросили, значит файл закончился
					if len(cdnData.Bytes) < chunkSize {
						finished = true
					}
				}
			default:
				os.Remove(fileName)
				return ""
			}
		default:
			// Неожиданный тип ответа
			os.Remove(fileName)
			return ""
		}

		if finished {
			break
		}
	}

	// Проверяем, что файл не пустой
	if info, err := os.Stat(fileName); err != nil || info.Size() == 0 {
		os.Remove(fileName)
		return ""
	}

	// Получаем информацию о файле для отладки
	return fileName
}
