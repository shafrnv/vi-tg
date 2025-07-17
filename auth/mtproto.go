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
	client   *telegram.Client
	api      *tg.Client
	authCode string // Код подтверждения для авторизации
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
	// Создаем файл-сигнал для TUI
	signalFile := "/tmp/vi-tg-needs-code"
	os.WriteFile(signalFile, []byte("1"), 0644)

	// Ждем пока код не будет установлен через TUI
	for {
		time.Sleep(100 * time.Millisecond)
		// Проверяем файл с кодом
		codeFile := "/tmp/vi-tg-auth-code"
		if data, err := os.ReadFile(codeFile); err == nil {
			code := strings.TrimSpace(string(data))
			os.Remove(codeFile)   // Удаляем файл после чтения
			os.Remove(signalFile) // Удаляем сигнальный файл
			return code, nil
		}
	}
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

// SetAuthCode устанавливает код подтверждения
func (m *MTProtoClient) SetAuthCode(code string) {
	m.authCode = code
}

// IsAuthorized проверяет, авторизован ли клиент
func (m *MTProtoClient) IsAuthorized() bool {
	return m.api != nil && m.client != nil
}

// InitFromSession инициализирует клиент из сохраненной сессии
func (m *MTProtoClient) InitFromSession(ctx context.Context) error {
	sessionPath := getSessionPath()

	// Проверяем, существует ли файл сессии
	if _, err := os.Stat(sessionPath); err != nil {
		return fmt.Errorf("файл сессии не найден: %w", err)
	}

	client := telegram.NewClient(19936415, "2721a01cc1e880707e42f3f56fee3448", telegram.Options{
		SessionStorage: &telegram.FileSessionStorage{Path: sessionPath},
	})

	// Запускаем клиент в горутине для проверки сессии
	authDone := make(chan error, 1)

	go func() {
		err := client.Run(ctx, func(ctx context.Context) error {
			// Проверяем, авторизован ли клиент
			if _, err := client.Auth().Status(ctx); err != nil {
				return fmt.Errorf("сессия недействительна: %w", err)
			}

			// Сохраняем API клиент
			m.api = client.API()
			m.client = client

			// Сигнализируем об успешной инициализации
			authDone <- nil

			// Держим соединение активным
			<-ctx.Done()
			return nil
		})

		// Если инициализация не прошла, отправляем ошибку
		select {
		case authDone <- err:
		default:
		}
	}()

	// Ждем успешной инициализации
	select {
	case err := <-authDone:
		if err != nil {
			return err
		}
		return nil
	case <-time.After(10 * time.Second):
		return fmt.Errorf("таймаут инициализации из сессии")
	}
}

func (m *MTProtoClient) AuthAndConnect(ctx context.Context, phone string) error {
	sessionPath := getSessionPath()

	// Создаем директорию для сессии если её нет
	sessionDir := filepath.Dir(sessionPath)
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		return fmt.Errorf("ошибка создания директории сессии: %w", err)
	}

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

			fmt.Println("Соединение установлено, поддерживаем активность...")

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
		fmt.Println("Авторизация завершена, интерфейс запускается...")
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
	case *tg.MessagesDialogsSlice:
		// Обрабатываем MessagesDialogsSlice аналогично
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
func (m *MTProtoClient) processMessage(message *tg.Message, users []tg.UserClass, chats []tg.ChatClass, peerID int64) Message {
	fmt.Printf("DEBUG: Processing Message - PeerID: %d, FromID: %+v\n", peerID, message.FromID)

	fromName := ""

	// Обработка различных типов FromID
	if message.FromID != nil {
		switch fromPeer := message.FromID.(type) {
		case *tg.PeerUser:
			// Поиск пользователя по ID
			for _, userRaw := range users {
				if u, ok := userRaw.(*tg.User); ok && u.ID == fromPeer.UserID {
					// Приоритет: Username → FirstName LastName → ID
					if u.Username != "" {
						fromName = u.Username
						fmt.Println("DEBUG: Using Username")
					} else {
						fromName = strings.TrimSpace(fmt.Sprintf("%s %s", u.FirstName, u.LastName))
						if fromName == "" {
							fromName = fmt.Sprintf("User_%d", u.ID)
						}
						fmt.Println("DEBUG: Using FirstName LastName")
					}
					break
				}
			}

		case *tg.PeerChat:
			// Обработка сообщений в групповом чате
			for _, chatRaw := range chats {
				if c, ok := chatRaw.(*tg.Chat); ok && c.ID == fromPeer.ChatID {
					fromName = c.Title
					fmt.Println("DEBUG: Using Chat Title")
					break
				}
			}

			// Если название чата не найдено, используем generic идентификатор
			if fromName == "" {
				fromName = fmt.Sprintf("Chat_%d", fromPeer.ChatID)
			}

		case *tg.PeerChannel:
			// Обработка сообщений в канале
			for _, chatRaw := range chats {
				if c, ok := chatRaw.(*tg.Channel); ok && c.ID == fromPeer.ChannelID {
					fromName = c.Title
					fmt.Println("DEBUG: Using Channel Title")
					break
				}
			}

			// Если название канала не найдено, используем generic идентификатор
			if fromName == "" {
				fromName = fmt.Sprintf("Channel_%d", fromPeer.ChannelID)
			}

		default:
			fmt.Printf("DEBUG: Unexpected FromID type: %T\n", fromPeer)
			fromName = "Unknown"
		}
	} else {
		// Если FromID nil, пытаемся определить имя по PeerID
		for _, userRaw := range users {
			if u, ok := userRaw.(*tg.User); ok && u.ID == peerID {
				if u.Username != "" {
					fromName = u.Username
				} else {
					fromName = strings.TrimSpace(fmt.Sprintf("%s %s", u.FirstName, u.LastName))
					if fromName == "" {
						fromName = fmt.Sprintf("User_%d", u.ID)
					}
				}
				break
			}
		}

		// Если имя не найдено, используем generic идентификатор
		if fromName == "" {
			fromName = fmt.Sprintf("User_%d", peerID)
		}
	}

	ts := time.Unix(int64(message.Date), 0)

	// Существующая логика обработки медиа
	msgType := "text"
	stickerID := int64(0)
	stickerEmoji := ""
	stickerPath := ""

	if message.Media != nil {
		switch media := message.Media.(type) {
		case *tg.MessageMediaDocument:
			if media.Document != nil {
				if doc, ok := media.Document.(*tg.Document); ok {
					for _, attr := range doc.Attributes {
						if stickerAttr, ok := attr.(*tg.DocumentAttributeSticker); ok {
							msgType = "sticker"
							stickerID = doc.ID
							stickerEmoji = stickerAttr.Alt
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

	var messagesRaw tg.MessagesMessagesClass
	var err error

	// Последовательно пробуем различные типы peer
	peerTypes := []tg.InputPeerClass{
		&tg.InputPeerUser{UserID: peerID},
		&tg.InputPeerChat{ChatID: peerID},
	}

	// Для каналов требуется дополнительная информация об access hash
	// Попробуем получить информацию о канале перед запросом
	channelsResp, err := m.api.ChannelsGetChannels(messagesCtx, []tg.InputChannelClass{
		&tg.InputChannel{
			ChannelID: peerID,
		},
	})

	if err == nil {
		// Проверяем тип ответа и извлекаем информацию о канале
		switch resp := channelsResp.(type) {
		case *tg.MessagesChats:
			for _, chat := range resp.Chats {
				if channel, ok := chat.(*tg.Channel); ok {
					peerTypes = append(peerTypes, &tg.InputPeerChannel{
						ChannelID:  channel.ID,
						AccessHash: channel.AccessHash,
					})
					break
				}
			}
		}
	}

	// Пробуем получить сообщения для каждого типа peer
	for _, peer := range peerTypes {
		messagesRaw, err = m.api.MessagesGetHistory(messagesCtx, &tg.MessagesGetHistoryRequest{
			Peer:  peer,
			Limit: limit,
		})

		if err == nil {
			break
		}
	}

	if err != nil {
		return nil, fmt.Errorf("ошибка получения сообщений: %w", err)
	}

	var result []Message
	var users []tg.UserClass
	var chats []tg.ChatClass

	// Определяем пользователей и чаты в зависимости от типа ответа
	switch msg := messagesRaw.(type) {
	case *tg.MessagesMessagesSlice:
		users = msg.Users
		chats = msg.Chats
		for _, msgRaw := range msg.Messages {
			if message, ok := msgRaw.(*tg.Message); ok {
				result = append(result, m.processMessage(message, users, chats, peerID))
			}
		}
	case *tg.MessagesMessages:
		users = msg.Users
		chats = msg.Chats
		for _, msgRaw := range msg.Messages {
			if message, ok := msgRaw.(*tg.Message); ok {
				result = append(result, m.processMessage(message, users, chats, peerID))
			}
		}
	case *tg.MessagesChannelMessages:
		users = msg.Users
		chats = msg.Chats
		for _, msgRaw := range msg.Messages {
			if message, ok := msgRaw.(*tg.Message); ok {
				result = append(result, m.processMessage(message, users, chats, peerID))
			}
		}
	default:
		return nil, fmt.Errorf("неизвестный тип сообщений: %T", messagesRaw)
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
			fmt.Printf("DEBUG: Получено %d байт данных\n", len(data.Bytes))
			if len(data.Bytes) == 0 {
				// Файл скачан полностью
				fmt.Printf("DEBUG: Получен пустой чанк, файл скачан полностью\n")
				finished = true
			} else {
				// Записываем чанк в файл
				if _, err := f.Write(data.Bytes); err != nil {
					fmt.Printf("DEBUG: Ошибка записи в файл: %v\n", err)
					os.Remove(fileName)
					return ""
				}
				offset += int64(len(data.Bytes))
				totalBytes += int64(len(data.Bytes))

				// Если получили меньше данных чем запросили, значит файл закончился
				if len(data.Bytes) < chunkSize {
					fmt.Printf("DEBUG: Получен последний чанк, файл скачан\n")
					finished = true
				}
			}
		case *tg.UploadFileCDNRedirect:
			fmt.Printf("DEBUG: Получен CDN редирект, скачиваем через CDN\n")
			// Скачиваем файл через CDN
			cdnResp, err := api.UploadGetCDNFile(context.Background(), &tg.UploadGetCDNFileRequest{
				FileToken: data.FileToken,
				Offset:    offset,
				Limit:     chunkSize,
			})
			if err != nil {
				fmt.Printf("DEBUG: Ошибка CDN запроса: %v\n", err)
				os.Remove(fileName)
				return ""
			}

			fmt.Printf("DEBUG: CDN ответ типа: %T\n", cdnResp)

			switch cdnData := cdnResp.(type) {
			case *tg.UploadCDNFile:
				fmt.Printf("DEBUG: Получено %d байт данных через CDN\n", len(cdnData.Bytes))
				if len(cdnData.Bytes) == 0 {
					fmt.Printf("DEBUG: Получен пустой CDN чанк, файл скачан полностью\n")
					finished = true
				} else {
					// Записываем чанк в файл
					if _, err := f.Write(cdnData.Bytes); err != nil {
						fmt.Printf("DEBUG: Ошибка записи CDN данных в файл: %v\n", err)
						os.Remove(fileName)
						return ""
					}
					offset += int64(len(cdnData.Bytes))
					totalBytes += int64(len(cdnData.Bytes))

					// Если получили меньше данных чем запросили, значит файл закончился
					if len(cdnData.Bytes) < chunkSize {
						fmt.Printf("DEBUG: Получен последний CDN чанк, файл скачан\n")
						finished = true
					}
				}
			default:
				fmt.Printf("DEBUG: Неожиданный тип CDN ответа: %T\n", cdnResp)
				os.Remove(fileName)
				return ""
			}
		default:
			fmt.Printf("DEBUG: Неожиданный тип ответа: %T\n", resp)
			// Неожиданный тип ответа
			os.Remove(fileName)
			return ""
		}

		if finished {
			break
		}
	}

	fmt.Printf("DEBUG: Всего скачано байт: %d\n", totalBytes)

	// Проверяем, что файл не пустой
	if info, err := os.Stat(fileName); err != nil || info.Size() == 0 {
		fmt.Printf("DEBUG: Файл пустой или не существует, удаляем\n")
		os.Remove(fileName)
		return ""
	}

	// Получаем информацию о файле для отладки
	info, _ := os.Stat(fileName)
	fmt.Printf("DEBUG: Файл успешно скачан: %s, размер: %d\n", fileName, info.Size())
	return fileName
}
