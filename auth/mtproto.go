package auth

import (
	"bufio"
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/gotd/td/telegram"
	gotdauth "github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/tg"
	"golang.org/x/crypto/ssh/terminal"
)

// debugLog записывает отладочные сообщения в файл
func debugLog(format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)
	timestamp := time.Now().Format("2006-01-02 15:04:05.000")
	logMessage := fmt.Sprintf("[%s] %s\n", timestamp, message)

	if file, err := os.OpenFile("/tmp/vi-tg-debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
		defer file.Close()
		file.WriteString(logMessage)
	}
}

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
	ID               int
	Text             string
	From             string
	Timestamp        time.Time
	ChatID           int64
	Type             string // "text", "sticker", "photo", "video", "voice", etc.
	StickerID        int64  // ID стикера если Type == "sticker"
	StickerEmoji     string // Эмодзи стикера
	StickerPath      string // Путь к файлу стикера (если скачан)
	ImagePath        string // Путь к файлу изображения (если скачан)
	VideoPath        string // Путь к файлу видео (если скачан)
	VideoPreviewPath string // Путь к превью видео (если сгенерировано)
	VideoIsRound     bool   // Флаг для круглого видео
	VoiceID          int64  // ID голосового сообщения если Type == "voice"
	VoicePath        string // Путь к файлу голосового сообщения (если скачан)
	VoiceDuration    int    // Длительность голосового сообщения в секундах
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
	imagePath := ""
	videoPath := ""
	videoPreviewPath := ""
	videoIsRound := false
	voiceID := int64(0)
	voicePath := ""
	voiceDuration := 0

	debugLog("Обрабатываем сообщение %d, медиа: %v", message.ID, message.Media != nil)

	if message.Media != nil {
		switch media := message.Media.(type) {
		case *tg.MessageMediaDocument:
			debugLog("Сообщение %d содержит документ", message.ID)
			if media.Document != nil {
				if doc, ok := media.Document.(*tg.Document); ok {
					isVideo := false
					isVoice := false
					for _, attr := range doc.Attributes {
						if stickerAttr, ok := attr.(*tg.DocumentAttributeSticker); ok {
							msgType = "sticker"
							stickerID = doc.ID
							stickerEmoji = stickerAttr.Alt
							debugLog("Начинаем скачивание стикера для сообщения %d, Document ID: %d", message.ID, doc.ID)
							stickerPath = downloadStickerFile(m.api, doc)
							if stickerPath == "" {
								debugLog("Не удалось скачать стикер для сообщения %d", message.ID)
							} else {
								debugLog("Стикер для сообщения %d скачан: %s", message.ID, stickerPath)
							}
							break
						}
						if videoAttr, ok := attr.(*tg.DocumentAttributeVideo); ok {
							isVideo = true
							msgType = "video"
							// Проверяем, является ли видео круглым
							if videoAttr.RoundMessage {
								videoIsRound = true
								debugLog("Видео для сообщения %d является круглым", message.ID)
							}
							videoPath = downloadVideoFile(m.api, doc, message.ID)
							if videoPath == "" {
								debugLog("Не удалось скачать видео для сообщения %d", message.ID)
							} else {
								debugLog("Видео скачано: %s", videoPath)
								// Генерируем превью для видео
								videoPreviewPath = generateVideoPreview(videoPath, message.ID)
							}
							break
						}
						if audioAttr, ok := attr.(*tg.DocumentAttributeAudio); ok {
							if audioAttr.Voice {
								isVoice = true
								msgType = "voice"
								voiceID = doc.ID
								voiceDuration = int(audioAttr.Duration)
								debugLog("Начинаем скачивание голосового сообщения для сообщения %d, Document ID: %d", message.ID, doc.ID)
								voicePath = downloadVoiceFile(m.api, doc, message.ID)
								if voicePath == "" {
									debugLog("Не удалось скачать голосовое сообщение для сообщения %d", message.ID)
								} else {
									debugLog("Голосовое сообщение для сообщения %d скачано: %s", message.ID, voicePath)
								}
								break
							}
						}
					}
					if !isVideo && !isVoice && msgType != "sticker" {
						debugLog("Сообщение %d содержит документ неизвестного типа", message.ID)
					}
				}
			}
		case *tg.MessageMediaPhoto:
			debugLog("Сообщение %d содержит фото", message.ID)
			if photo, ok := media.Photo.(*tg.Photo); ok {
				msgType = "photo"
				imagePath = downloadPhotoFile(m.api, photo, message.ID)
				if imagePath == "" {
					debugLog("Не удалось скачать фото для сообщения %d", message.ID)
				} else {
					debugLog("Фото скачано: %s", imagePath)
				}
			}
		default:
			debugLog("Сообщение %d содержит неизвестный тип медиа: %T", message.ID, media)
		}
	} else {
		debugLog("Сообщение %d не содержит медиа", message.ID)
	}

	return Message{
		ID:               int(message.ID),
		Text:             message.Message,
		From:             fromName,
		Timestamp:        ts,
		ChatID:           peerID,
		Type:             msgType,
		StickerID:        stickerID,
		StickerEmoji:     stickerEmoji,
		StickerPath:      stickerPath,
		ImagePath:        imagePath,
		VideoPath:        videoPath,
		VideoPreviewPath: videoPreviewPath,
		VideoIsRound:     videoIsRound,
		VoiceID:          voiceID,
		VoicePath:        voicePath,
		VoiceDuration:    voiceDuration,
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
		debugLog("API или документ nil для стикера")
		return ""
	}

	// Проверяем, не скачан ли уже файл с любым расширением
	possibleExtensions := []string{".webp", ".png", ".jpg", ".jpeg"}
	for _, ext := range possibleExtensions {
		existingFileName := fmt.Sprintf("/tmp/vi-tg_sticker_%d%s", doc.ID, ext)
		if info, err := os.Stat(existingFileName); err == nil && info.Size() > 0 {
			debugLog("Стикер уже существует: %s", existingFileName)
			return existingFileName
		}
	}

	// Определяем предпочтительное расширение на основе атрибутов
	// Стикеры обычно приходят в формате WebP
	preferredExt := ".webp"
	for _, attr := range doc.Attributes {
		if _, ok := attr.(*tg.DocumentAttributeImageSize); ok {
			// Если есть атрибут размера изображения, может быть PNG, но проверим позже
			preferredExt = ".png"
			break
		}
	}

	// Временный файл для скачивания
	tempFileName := fmt.Sprintf("/tmp/vi-tg_sticker_%d_temp", doc.ID)

	// Создаем временный файл
	f, err := os.Create(tempFileName)
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
			os.Remove(tempFileName) // Удаляем временный файл
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
					os.Remove(tempFileName)
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
				os.Remove(tempFileName)
				return ""
			}

			switch cdnData := cdnResp.(type) {
			case *tg.UploadCDNFile:
				if len(cdnData.Bytes) == 0 {
					finished = true
				} else {
					// Записываем чанк в файл
					if _, err := f.Write(cdnData.Bytes); err != nil {
						os.Remove(tempFileName)
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
				os.Remove(tempFileName)
				return ""
			}
		default:
			os.Remove(tempFileName)
			return ""
		}

		if finished {
			break
		}
	}

	// Проверяем, что временный файл не пустой
	if info, err := os.Stat(tempFileName); err != nil || info.Size() == 0 {
		os.Remove(tempFileName)
		return ""
	}

	// Определяем реальный формат файла по его содержимому
	detectedExt := detectImageFormat(tempFileName)
	if detectedExt == "" {
		// Если формат не определен, используем предпочтительное расширение
		detectedExt = preferredExt
		debugLog("Формат не определен, используем предпочтительное расширение: %s", detectedExt)
	} else {
		debugLog("Определен формат стикера: %s", detectedExt)
	}

	// Финальный файл с правильным расширением
	finalFileName := fmt.Sprintf("/tmp/vi-tg_sticker_%d%s", doc.ID, detectedExt)

	// Переименовываем файл с правильным расширением
	if err := os.Rename(tempFileName, finalFileName); err != nil {
		debugLog("Ошибка переименования файла %s в %s: %v", tempFileName, finalFileName, err)
		os.Remove(tempFileName)
		return ""
	}

	debugLog("Стикер успешно скачан и сохранен как: %s", finalFileName)
	return finalFileName
}

// downloadPhotoFile скачивает фото и сохраняет как PNG
func downloadPhotoFile(api *tg.Client, photo *tg.Photo, messageID int) string {
	if api == nil || photo == nil {
		debugLog("API или фото nil для сообщения %d", messageID)
		return ""
	}

	debugLog("Начинаем скачивание фото для сообщения %d, Photo ID: %d", messageID, photo.ID)

	// Проверяем, не скачан ли уже файл
	possibleExtensions := []string{".jpg", ".jpeg", ".png", ".webp", ".gif"}
	for _, ext := range possibleExtensions {
		existingPath := fmt.Sprintf("/tmp/vi-tg_image_%d%s", messageID, ext)
		if _, err := os.Stat(existingPath); err == nil {
			debugLog("Файл уже существует: %s", existingPath)
			return existingPath
		}
	}

	// Собираем все доступные размеры
	var sizes []struct {
		width    int
		location tg.InputFileLocationClass
		desc     string
	}

	debugLog("Количество размеров фото: %d", len(photo.Sizes))

	for i, sizeRaw := range photo.Sizes {
		var width int
		var location tg.InputFileLocationClass
		var desc string

		switch size := sizeRaw.(type) {
		case *tg.PhotoSize:
			width = size.W
			desc = fmt.Sprintf("PhotoSize(%s)", size.Type)
			debugLog("Размер %d: PhotoSize, ширина: %d, тип: %s", i, width, size.Type)
			// Для PhotoSize используем InputPhotoFileLocation с ThumbSize
			location = &tg.InputPhotoFileLocation{
				ID:            photo.ID,
				AccessHash:    photo.AccessHash,
				FileReference: photo.FileReference,
				ThumbSize:     size.Type,
			}
		case *tg.PhotoSizeProgressive:
			width = size.W
			desc = "PhotoSizeProgressive"
			debugLog("Размер %d: PhotoSizeProgressive, ширина: %d", i, width)
			// Для PhotoSizeProgressive используем InputPhotoFileLocation без ThumbSize
			location = &tg.InputPhotoFileLocation{
				ID:            photo.ID,
				AccessHash:    photo.AccessHash,
				FileReference: photo.FileReference,
				ThumbSize:     "", // Пустая строка для полного размера
			}
		case *tg.PhotoSizeEmpty:
			debugLog("Размер %d: PhotoSizeEmpty", i)
			continue
		case *tg.PhotoStrippedSize:
			desc = fmt.Sprintf("PhotoStrippedSize(%s)", size.Type)
			debugLog("Размер %d: PhotoStrippedSize, тип: %s", i, size.Type)
			// Для PhotoStrippedSize используем InputPhotoFileLocation с ThumbSize
			location = &tg.InputPhotoFileLocation{
				ID:            photo.ID,
				AccessHash:    photo.AccessHash,
				FileReference: photo.FileReference,
				ThumbSize:     size.Type,
			}
			width = 0 // PhotoStrippedSize не имеет ширины
		default:
			debugLog("Размер %d: неизвестный тип %T", i, sizeRaw)
			continue
		}

		sizes = append(sizes, struct {
			width    int
			location tg.InputFileLocationClass
			desc     string
		}{width, location, desc})
	}

	// Сортируем размеры по убыванию (от большего к меньшему)
	for i := 0; i < len(sizes); i++ {
		for j := i + 1; j < len(sizes); j++ {
			if sizes[i].width < sizes[j].width {
				sizes[i], sizes[j] = sizes[j], sizes[i]
			}
		}
	}

	debugLog("Найдено %d размеров для скачивания", len(sizes))

	// Пробуем скачать с каждого размера, начиная с наибольшего
	for i, size := range sizes {
		debugLog("Пробуем скачать размер %d/%d: %s (ширина: %d)", i+1, len(sizes), size.desc, size.width)
		// Передаем пустую строку, чтобы функция сама определила формат
		result := downloadFileWithLocation(api, size.location, messageID, "")
		if result != "" {
			debugLog("Успешно скачан размер %s: %s", size.desc, result)
			return result
		}
		debugLog("Не удалось скачать размер %s", size.desc)
	}

	debugLog("Не удалось скачать ни один размер для фото сообщения %d", messageID)
	return ""
}

// downloadFileWithLocation скачивает файл по заданному location и сохраняет с правильным расширением
func downloadFileWithLocation(api *tg.Client, location tg.InputFileLocationClass, messageID int, ext string) string {
	// Сначала скачиваем во временный файл
	tempFileName := fmt.Sprintf("/tmp/vi-tg_image_%d_temp", messageID)

	debugLog("Начинаем скачивание во временный файл: %s", tempFileName)

	// Создаем временный файл
	f, err := os.Create(tempFileName)
	if err != nil {
		debugLog("Ошибка создания временного файла %s: %v", tempFileName, err)
		return ""
	}
	defer f.Close()

	// Скачиваем файл по частям
	offset := int64(0)
	chunkSize := int(512 * 1024) // 512KB чанки
	totalBytes := int64(0)
	finished := false
	chunkCount := 0

	debugLog("Начинаем скачивание файла по частям")

	for !finished {
		chunkCount++
		debugLog("Скачиваем чанк %d, offset: %d", chunkCount, offset)

		resp, err := api.UploadGetFile(context.Background(), &tg.UploadGetFileRequest{
			Precise:      true,
			CDNSupported: false, // Отключаем CDN поддержку
			Location:     location,
			Offset:       offset,
			Limit:        chunkSize,
		})

		if err != nil {
			// Проверяем, является ли ошибка связанной с истекшим file reference
			if strings.Contains(err.Error(), "FILE_REFERENCE_EXPIRED") {
				debugLog("File reference expired для сообщения %d", messageID)
				os.Remove(tempFileName)
				return ""
			}

			debugLog("Ошибка скачивания файла для сообщения %d: %v", messageID, err)
			os.Remove(tempFileName)
			return ""
		}

		// Обработка ответа
		switch file := resp.(type) {
		case *tg.UploadFile:
			if len(file.Bytes) == 0 {
				// Файл скачан полностью
				debugLog("Получен пустой чанк, файл скачан полностью")
				finished = true
			} else {
				// Записываем чанк в файл
				if _, err := f.Write(file.Bytes); err != nil {
					debugLog("Ошибка записи чанка в файл: %v", err)
					os.Remove(tempFileName)
					return ""
				}
				offset += int64(len(file.Bytes))
				totalBytes += int64(len(file.Bytes))
				debugLog("Записан чанк %d, размер: %d байт, общий размер: %d байт", chunkCount, len(file.Bytes), totalBytes)

				// Если получили меньше данных чем запросили, значит файл закончился
				if len(file.Bytes) < chunkSize {
					debugLog("Получен последний чанк, файл закончен")
					finished = true
				}
			}
		case *tg.UploadFileCDNRedirect:
			debugLog("Получен CDN редирект")
			// Скачиваем файл через CDN
			cdnResp, err := api.UploadGetCDNFile(context.Background(), &tg.UploadGetCDNFileRequest{
				FileToken: file.FileToken,
				Offset:    offset,
				Limit:     chunkSize,
			})
			if err != nil {
				debugLog("Ошибка скачивания через CDN: %v", err)
				os.Remove(tempFileName)
				return ""
			}

			switch cdnData := cdnResp.(type) {
			case *tg.UploadCDNFile:
				if len(cdnData.Bytes) == 0 {
					debugLog("Получен пустой CDN чанк, файл скачан полностью")
					finished = true
				} else {
					// Записываем чанк в файл
					if _, err := f.Write(cdnData.Bytes); err != nil {
						debugLog("Ошибка записи CDN чанка в файл: %v", err)
						os.Remove(tempFileName)
						return ""
					}
					offset += int64(len(cdnData.Bytes))
					totalBytes += int64(len(cdnData.Bytes))
					debugLog("Записан CDN чанк %d, размер: %d байт, общий размер: %d байт", chunkCount, len(cdnData.Bytes), totalBytes)

					// Если получили меньше данных чем запросили, значит файл закончился
					if len(cdnData.Bytes) < chunkSize {
						debugLog("Получен последний CDN чанк, файл закончен")
						finished = true
					}
				}
			default:
				debugLog("Неожиданный тип CDN ответа: %T", cdnResp)
				os.Remove(tempFileName)
				return ""
			}
		default:
			debugLog("Неожиданный тип ответа: %T", resp)
			os.Remove(tempFileName)
			return ""
		}
	}

	debugLog("Скачивание завершено, общий размер: %d байт", totalBytes)

	// Проверяем, что файл не пустой
	if info, err := os.Stat(tempFileName); err != nil || info.Size() == 0 {
		debugLog("Файл пустой или не существует: %v", err)
		os.Remove(tempFileName)
		return ""
	}

	// Определяем формат изображения
	detectedExt := detectImageFormat(tempFileName)
	if detectedExt == "" {
		// Если формат не определен, используем PNG как fallback
		detectedExt = ".png"
		debugLog("Формат не определен, используем PNG")
	} else {
		debugLog("Определен формат: %s", detectedExt)
	}

	// Переименовываем файл с правильным расширением
	finalFileName := fmt.Sprintf("/tmp/vi-tg_image_%d%s", messageID, detectedExt)

	if err := os.Rename(tempFileName, finalFileName); err != nil {
		debugLog("Ошибка переименования файла %s в %s: %v", tempFileName, finalFileName, err)
		os.Remove(tempFileName)
		return ""
	}

	debugLog("Файл успешно сохранен как %s", finalFileName)
	return finalFileName
}

// detectImageFormat определяет формат изображения по первым байтам файла
func detectImageFormat(filePath string) string {
	file, err := os.Open(filePath)
	if err != nil {
		return ""
	}
	defer file.Close()

	// Читаем первые 12 байт для определения формата
	header := make([]byte, 12)
	n, err := file.Read(header)
	if err != nil || n < 8 {
		return ""
	}

	// Проверяем различные форматы изображений
	if len(header) >= 2 {
		// JPEG: начинается с 0xFF 0xD8
		if header[0] == 0xFF && header[1] == 0xD8 {
			return ".jpg"
		}
	}

	if len(header) >= 8 {
		// PNG: начинается с 0x89 0x50 0x4E 0x47 0x0D 0x0A 0x1A 0x0A
		if header[0] == 0x89 && header[1] == 0x50 && header[2] == 0x4E && header[3] == 0x47 &&
			header[4] == 0x0D && header[5] == 0x0A && header[6] == 0x1A && header[7] == 0x0A {
			return ".png"
		}
	}

	if len(header) >= 4 {
		// GIF: начинается с "GIF8"
		if header[0] == 0x47 && header[1] == 0x49 && header[2] == 0x46 && header[3] == 0x38 {
			return ".gif"
		}
	}

	if len(header) >= 12 {
		// WebP: начинается с "RIFF" и содержит "WEBP"
		if header[0] == 0x52 && header[1] == 0x49 && header[2] == 0x46 && header[3] == 0x46 &&
			header[8] == 0x57 && header[9] == 0x45 && header[10] == 0x42 && header[11] == 0x50 {
			return ".webp"
		}
	}

	// Если формат не определен, возвращаем пустую строку
	return ""
}

// downloadVideoFile скачивает видео файл
func downloadVideoFile(api *tg.Client, doc *tg.Document, messageID int) string {
	if api == nil || doc == nil {
		debugLog("API или документ nil для сообщения %d", messageID)
		return ""
	}

	debugLog("Начинаем скачивание видео для сообщения %d, Document ID: %d", messageID, doc.ID)

	// Определяем расширение на основе MIME типа или атрибутов
	ext := ".mp4" // По умолчанию MP4
	for _, attr := range doc.Attributes {
		if filename, ok := attr.(*tg.DocumentAttributeFilename); ok {
			// Извлекаем расширение из имени файла
			if strings.Contains(filename.FileName, ".") {
				fileExt := filepath.Ext(filename.FileName)
				if fileExt != "" {
					ext = fileExt
				}
			}
		}
	}

	// Проверяем, не скачан ли уже файл
	possibleExtensions := []string{".mp4", ".avi", ".mkv", ".mov", ".webm", ".flv"}
	for _, testExt := range possibleExtensions {
		existingPath := fmt.Sprintf("/tmp/vi-tg_video_%d%s", messageID, testExt)
		if _, err := os.Stat(existingPath); err == nil {
			debugLog("Видео файл уже существует: %s", existingPath)
			return existingPath
		}
	}

	// Путь для сохранения
	fileName := fmt.Sprintf("/tmp/vi-tg_video_%d%s", messageID, ext)
	debugLog("Сохраняем видео как: %s", fileName)

	// Создаем файл
	f, err := os.Create(fileName)
	if err != nil {
		debugLog("Ошибка создания файла %s: %v", fileName, err)
		return ""
	}
	defer f.Close()

	// Скачиваем файл по частям
	offset := int64(0)
	chunkSize := int(1024 * 1024) // 1MB чанки для видео
	totalBytes := int64(0)
	finished := false
	chunkCount := 0

	debugLog("Начинаем скачивание видео файла по частям")

	for !finished {
		chunkCount++
		debugLog("Скачиваем чанк %d, offset: %d", chunkCount, offset)

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
			debugLog("Ошибка скачивания видео для сообщения %d: %v", messageID, err)
			os.Remove(fileName)
			return ""
		}

		// Обработка ответа
		switch file := resp.(type) {
		case *tg.UploadFile:
			if len(file.Bytes) == 0 {
				// Файл скачан полностью
				debugLog("Получен пустой чанк, видео файл скачан полностью")
				finished = true
			} else {
				// Записываем чанк в файл
				if _, err := f.Write(file.Bytes); err != nil {
					debugLog("Ошибка записи чанка в видео файл: %v", err)
					os.Remove(fileName)
					return ""
				}
				offset += int64(len(file.Bytes))
				totalBytes += int64(len(file.Bytes))
				debugLog("Записан чанк %d, размер: %d байт, общий размер: %d байт", chunkCount, len(file.Bytes), totalBytes)

				// Если получили меньше данных чем запросили, значит файл закончился
				if len(file.Bytes) < chunkSize {
					debugLog("Получен последний чанк, видео файл закончен")
					finished = true
				}
			}
		case *tg.UploadFileCDNRedirect:
			debugLog("Получен CDN редирект для видео")
			// Скачиваем файл через CDN
			cdnResp, err := api.UploadGetCDNFile(context.Background(), &tg.UploadGetCDNFileRequest{
				FileToken: file.FileToken,
				Offset:    offset,
				Limit:     chunkSize,
			})
			if err != nil {
				debugLog("Ошибка скачивания видео через CDN: %v", err)
				os.Remove(fileName)
				return ""
			}

			switch cdnData := cdnResp.(type) {
			case *tg.UploadCDNFile:
				if len(cdnData.Bytes) == 0 {
					debugLog("Получен пустой CDN чанк, видео файл скачан полностью")
					finished = true
				} else {
					// Записываем чанк в файл
					if _, err := f.Write(cdnData.Bytes); err != nil {
						debugLog("Ошибка записи CDN чанка в видео файл: %v", err)
						os.Remove(fileName)
						return ""
					}
					offset += int64(len(cdnData.Bytes))
					totalBytes += int64(len(cdnData.Bytes))
					debugLog("Записан CDN чанк %d, размер: %d байт, общий размер: %d байт", chunkCount, len(cdnData.Bytes), totalBytes)

					// Если получили меньше данных чем запросили, значит файл закончился
					if len(cdnData.Bytes) < chunkSize {
						debugLog("Получен последний CDN чанк, видео файл закончен")
						finished = true
					}
				}
			default:
				debugLog("Неожиданный тип CDN ответа: %T", cdnResp)
				os.Remove(fileName)
				return ""
			}
		default:
			debugLog("Неожиданный тип ответа: %T", resp)
			os.Remove(fileName)
			return ""
		}
	}

	debugLog("Скачивание видео завершено, общий размер: %d байт", totalBytes)

	// Проверяем, что файл не пустой
	if info, err := os.Stat(fileName); err != nil || info.Size() == 0 {
		debugLog("Видео файл пустой или не существует: %v", err)
		os.Remove(fileName)
		return ""
	}

	debugLog("Видео файл успешно сохранен как %s", fileName)
	return fileName
}

// generateVideoPreview генерирует превью для видео и возвращает путь к превью
func generateVideoPreview(videoPath string, messageID int) string {
	if videoPath == "" {
		debugLog("Пустой путь к видео для сообщения %d", messageID)
		return ""
	}

	// Проверяем, существует ли уже превью
	previewPath := fmt.Sprintf("/tmp/vi-tg_video_preview_%d.jpg", messageID)
	if _, err := os.Stat(previewPath); err == nil {
		debugLog("Превью уже существует: %s", previewPath)
		return previewPath
	}

	debugLog("Генерируем превью для видео: %s (ID: %d)", videoPath, messageID)

	// Проверяем, существует ли видео файл
	if _, err := os.Stat(videoPath); err != nil {
		debugLog("Видео файл не найден: %s", videoPath)
		return ""
	}

	// Получаем информацию о видео файле
	videoInfo, err := os.Stat(videoPath)
	if err != nil {
		debugLog("Не удалось получить информацию о видео файле: %v", err)
		return ""
	}
	debugLog("Размер видео файла: %d байт", videoInfo.Size())

	// Создаем временный файл для превью
	tempPreviewPath := fmt.Sprintf("/tmp/vi-tg_video_preview_%d_temp.jpg", messageID)

	// Используем ffmpeg для генерации превью с улучшенными параметрами
	previewCmd := fmt.Sprintf("/usr/bin/ffmpeg -i '%s' -ss 00:00:01.000 -vframes 1 -q:v 3 -vf 'scale=320:-1' -f image2 '%s' 2>&1", videoPath, tempPreviewPath)

	debugLog("Выполняем команду: %s", previewCmd)

	// Выполняем команду через sh
	cmd := exec.Command("sh", "-c", previewCmd)
	output, err := cmd.CombinedOutput()

	if err != nil {
		debugLog("Ошибка генерации превью для видео %s: %v", videoPath, err)
		debugLog("Вывод команды: %s", string(output))

		// Попробуем альтернативный подход с другой временной меткой
		previewCmd2 := fmt.Sprintf("/usr/bin/ffmpeg -i '%s' -ss 00:00:00.500 -vframes 1 -q:v 3 -vf 'scale=320:-1' -f image2 '%s' 2>&1", videoPath, tempPreviewPath)
		debugLog("Пробуем альтернативную команду: %s", previewCmd2)
		cmd2 := exec.Command("sh", "-c", previewCmd2)
		output2, err2 := cmd2.CombinedOutput()

		if err2 != nil {
			debugLog("Ошибка генерации превью (альтернативный метод) для видео %s: %v", videoPath, err2)
			debugLog("Вывод альтернативной команды: %s", string(output2))
			return ""
		}

		// Проверяем, создался ли файл после второй попытки
		if _, err := os.Stat(tempPreviewPath); err != nil {
			debugLog("Вторая попытка также не создала файл превью: %s", tempPreviewPath)
			return ""
		}
	} else {
		// Проверяем, создался ли файл после первой попытки
		if _, err := os.Stat(tempPreviewPath); err != nil {
			debugLog("Первая попытка не создала файл превью: %s", tempPreviewPath)
			return ""
		}
	}

	// Проверяем, что временный файл был создан
	if _, err := os.Stat(tempPreviewPath); err != nil {
		debugLog("Временный файл превью не был создан: %s", tempPreviewPath)
		return ""
	}

	// Проверяем размер временного файла
	if info, err := os.Stat(tempPreviewPath); err != nil || info.Size() < 100 {
		debugLog("Сгенерированный превью файл слишком мал: %s (размер: %d байт)", tempPreviewPath, info.Size())
		os.Remove(tempPreviewPath)
		return ""
	}

	// Переименовываем временный файл в постоянный
	if err := os.Rename(tempPreviewPath, previewPath); err != nil {
		debugLog("Не удалось переименовать временный файл: %v", err)
		os.Remove(tempPreviewPath)
		return ""
	}

	// Финальная проверка
	if info, err := os.Stat(previewPath); err != nil {
		debugLog("Не удалось получить информацию о финальном файле превью: %v", err)
		return ""
	} else {
		debugLog("Превью успешно сгенерировано: %s (размер: %d байт)", previewPath, info.Size())
	}

	return previewPath
}

// downloadVoiceFile скачивает голосовой файл
func downloadVoiceFile(api *tg.Client, doc *tg.Document, messageID int) string {
	if api == nil || doc == nil {
		debugLog("API или документ nil для голосового сообщения %d", messageID)
		return ""
	}

	debugLog("Начинаем скачивание голосового сообщения для сообщения %d, Document ID: %d", messageID, doc.ID)

	// Определяем расширение на основе MIME типа или атрибутов
	ext := ".ogg" // Голосовые сообщения обычно в формате OGG
	for _, attr := range doc.Attributes {
		if filename, ok := attr.(*tg.DocumentAttributeFilename); ok {
			// Извлекаем расширение из имени файла
			if strings.Contains(filename.FileName, ".") {
				fileExt := filepath.Ext(filename.FileName)
				if fileExt != "" {
					ext = fileExt
				}
			}
		}
	}

	// Проверяем, не скачан ли уже файл
	possibleExtensions := []string{".ogg", ".oga", ".mp3", ".wav", ".m4a", ".aac"}
	for _, testExt := range possibleExtensions {
		existingPath := fmt.Sprintf("/tmp/vi-tg_voice_%d%s", messageID, testExt)
		if _, err := os.Stat(existingPath); err == nil {
			debugLog("Голосовой файл уже существует: %s", existingPath)
			return existingPath
		}
	}

	// Путь для сохранения
	fileName := fmt.Sprintf("/tmp/vi-tg_voice_%d%s", messageID, ext)
	debugLog("Сохраняем голосовой файл как: %s", fileName)

	// Создаем файл
	f, err := os.Create(fileName)
	if err != nil {
		debugLog("Ошибка создания файла %s: %v", fileName, err)
		return ""
	}
	defer f.Close()

	// Скачиваем файл по частям
	offset := int64(0)
	chunkSize := int(512 * 1024) // 512KB чанки для голосовых файлов
	totalBytes := int64(0)
	finished := false
	chunkCount := 0

	debugLog("Начинаем скачивание голосового файла по частям")

	for !finished {
		chunkCount++
		debugLog("Скачиваем чанк %d, offset: %d", chunkCount, offset)

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
			debugLog("Ошибка скачивания голосового файла для сообщения %d: %v", messageID, err)
			os.Remove(fileName)
			return ""
		}

		// Обработка ответа
		switch file := resp.(type) {
		case *tg.UploadFile:
			if len(file.Bytes) == 0 {
				// Файл скачан полностью
				debugLog("Получен пустой чанк, голосовой файл скачан полностью")
				finished = true
			} else {
				// Записываем чанк в файл
				if _, err := f.Write(file.Bytes); err != nil {
					debugLog("Ошибка записи чанка в голосовой файл: %v", err)
					os.Remove(fileName)
					return ""
				}
				offset += int64(len(file.Bytes))
				totalBytes += int64(len(file.Bytes))
				debugLog("Записан чанк %d, размер: %d байт, общий размер: %d байт", chunkCount, len(file.Bytes), totalBytes)

				// Если получили меньше данных чем запросили, значит файл закончился
				if len(file.Bytes) < chunkSize {
					debugLog("Получен последний чанк, голосовой файл закончен")
					finished = true
				}
			}
		case *tg.UploadFileCDNRedirect:
			debugLog("Получен CDN редирект для голосового файла")
			// Скачиваем файл через CDN
			cdnResp, err := api.UploadGetCDNFile(context.Background(), &tg.UploadGetCDNFileRequest{
				FileToken: file.FileToken,
				Offset:    offset,
				Limit:     chunkSize,
			})
			if err != nil {
				debugLog("Ошибка скачивания голосового файла через CDN: %v", err)
				os.Remove(fileName)
				return ""
			}

			switch cdnData := cdnResp.(type) {
			case *tg.UploadCDNFile:
				if len(cdnData.Bytes) == 0 {
					debugLog("Получен пустой CDN чанк, голосовой файл скачан полностью")
					finished = true
				} else {
					// Записываем чанк в файл
					if _, err := f.Write(cdnData.Bytes); err != nil {
						debugLog("Ошибка записи CDN чанка в голосовой файл: %v", err)
						os.Remove(fileName)
						return ""
					}
					offset += int64(len(cdnData.Bytes))
					totalBytes += int64(len(cdnData.Bytes))
					debugLog("Записан CDN чанк %d, размер: %d байт, общий размер: %d байт", chunkCount, len(cdnData.Bytes), totalBytes)

					// Если получили меньше данных чем запросили, значит файл закончился
					if len(cdnData.Bytes) < chunkSize {
						debugLog("Получен последний CDN чанк, голосовой файл закончен")
						finished = true
					}
				}
			default:
				debugLog("Неожиданный тип CDN ответа: %T", cdnResp)
				os.Remove(fileName)
				return ""
			}
		default:
			debugLog("Неожиданный тип ответа: %T", resp)
			os.Remove(fileName)
			return ""
		}
	}

	debugLog("Скачивание голосового файла завершено, общий размер: %d байт", totalBytes)

	// Проверяем, что файл не пустой
	if info, err := os.Stat(fileName); err != nil || info.Size() == 0 {
		debugLog("Голосовой файл пустой или не существует: %v", err)
		os.Remove(fileName)
		return ""
	}

	debugLog("Голосовой файл успешно сохранен как %s", fileName)
	return fileName
}
