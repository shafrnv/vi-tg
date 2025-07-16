package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	"vi-tg/auth"
	"vi-tg/config"
	"vi-tg/telegram"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Стили для интерфейса
var (
	titleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("205")).
			Bold(true)

	selectedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("170")).
			Background(lipgloss.Color("235"))

	chatStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("86"))

	messageStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196"))

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("244"))
)

type model struct {
	telegram *telegram.Client
	mtproto  *auth.MTProtoClient
	config   *config.Config
	ctx      context.Context

	// UI состояние
	chats       []ChatItem
	messages    []MessageItem
	currentChat string
	chatIndex   int
	chatScroll  int // Индекс первого видимого чата
	input       string
	width       int
	height      int

	// Режимы
	inputMode bool
	loading   bool
	error     string

	// Режим просмотра стикера
	stickerViewMode   bool
	selectedSticker   *MessageItem
	stickerPanelIndex int // Индекс выбранного стикера в панели
}

type ChatItem struct {
	Name   string
	ID     int64
	Unread int
}

type MessageItem struct {
	From         string
	Text         string
	Timestamp    string
	Type         string // "text", "sticker", "photo", "video", etc.
	StickerID    int64  // ID стикера если Type == "sticker"
	StickerEmoji string // Эмодзи стикера
	StickerPath  string // Путь к файлу стикера (если скачан)
}

// Сообщения для обновления модели
type loadChatsMsg []ChatItem
type loadMessagesMsg []MessageItem
type errorMsg string
type reloadMessagesMsg struct {
	chatName string
	chatID   int64
}

func initialModel() model {
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatal(err)
	}

	var tgClient *telegram.Client
	var mtprotoClient *auth.MTProtoClient

	if cfg.UseMTProto {
		mtprotoClient = auth.NewMTProtoClient()
	} else if cfg.TelegramToken != "" {
		tgClient, err = telegram.NewClient(cfg.TelegramToken)
		if err != nil {
			log.Fatal(err)
		}
	}

	return model{
		telegram: tgClient,
		mtproto:  mtprotoClient,
		config:   cfg,
		ctx:      context.Background(),
		chats:    []ChatItem{},
		messages: []MessageItem{},
		loading:  true,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(
		tea.EnterAltScreen,
		m.initAuth(),
		m.loadChats(),
	)
}

func (m model) initAuth() tea.Cmd {
	return tea.Tick(time.Millisecond*100, func(time.Time) tea.Msg {
		if m.config.UseMTProto && m.mtproto != nil {
			if m.config.PhoneNumber == "" {
				fmt.Print("Введите номер телефона (с кодом страны): ")
				var phone string
				fmt.Scanln(&phone)
				m.config.PhoneNumber = phone
				config.SaveConfig(m.config)
			}

			if err := m.mtproto.AuthAndConnect(m.ctx, m.config.PhoneNumber); err != nil {
				return errorMsg(fmt.Sprintf("Ошибка авторизации: %v", err))
			}
		}
		return nil
	})
}

func (m model) loadChats() tea.Cmd {
	return tea.Tick(time.Millisecond*500, func(time.Time) tea.Msg {
		var chats []ChatItem

		if m.config.UseMTProto && m.mtproto != nil {
			dialogsCtx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
			defer cancel()

			dialogs, err := m.mtproto.GetDialogs(dialogsCtx)
			if err != nil {
				return errorMsg(fmt.Sprintf("Ошибка загрузки диалогов: %v", err))
			}

			for _, dialog := range dialogs {
				chats = append(chats, ChatItem{
					Name:   dialog.Title,
					ID:     dialog.ID,
					Unread: dialog.Unread,
				})
			}
		} else if m.telegram != nil {
			tgChats, err := m.telegram.GetChats()
			if err != nil {
				return errorMsg(fmt.Sprintf("Ошибка загрузки чатов: %v", err))
			}

			for _, chat := range tgChats {
				chats = append(chats, ChatItem{
					Name: chat.Name,
					ID:   chat.ID,
				})
			}
		} else {
			chats = append(chats, ChatItem{
				Name: "Telegram не подключен",
				ID:   0,
			})
		}

		return loadChatsMsg(chats)
	})
}

func (m model) loadMessages(chatName string, chatID int64) tea.Cmd {
	return tea.Tick(time.Millisecond*100, func(time.Time) tea.Msg {
		var messages []MessageItem

		if m.config.UseMTProto && m.mtproto != nil {
			msgs, err := m.mtproto.GetMessages(m.ctx, chatID, 50)
			if err != nil {
				return errorMsg(fmt.Sprintf("Ошибка загрузки сообщений: %v", err))
			}

			for _, msg := range msgs {
				messages = append(messages, MessageItem{
					From:         msg.From,
					Text:         msg.Text,
					Timestamp:    msg.Timestamp.Format("15:04"),
					Type:         msg.Type,
					StickerID:    msg.StickerID,
					StickerEmoji: msg.StickerEmoji,
					StickerPath:  msg.StickerPath,
				})
			}
		} else if m.telegram != nil {
			msgs, err := m.telegram.GetMessages(chatID, 50)
			if err != nil {
				return errorMsg(fmt.Sprintf("Ошибка загрузки сообщений: %v", err))
			}

			for _, msg := range msgs {
				messages = append(messages, MessageItem{
					From:         msg.From,
					Text:         msg.Text,
					Timestamp:    msg.Timestamp.Format("15:04"),
					Type:         msg.Type,
					StickerID:    msg.StickerID,
					StickerEmoji: msg.StickerEmoji,
					StickerPath:  msg.StickerPath,
				})
			}
		}

		return loadMessagesMsg(messages)
	})
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		if m.inputMode {
			switch msg.String() {
			case "enter":
				if m.input != "" {
					cmd := m.sendMessage()
					m.input = ""
					m.inputMode = false
					m.loading = true
					return m, cmd
				}
				m.inputMode = false
				return m, nil
			case "esc":
				m.inputMode = false
				m.input = ""
				return m, nil
			case "backspace":
				if len(m.input) > 0 {
					m.input = m.input[:len(m.input)-1]
				}
				return m, nil
			default:
				if len(msg.String()) == 1 {
					m.input += msg.String()
				}
				return m, nil
			}
		}

		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "up":
			if m.chatIndex > 0 {
				m.chatIndex--
				// Прокрутка вверх если нужно
				if m.chatIndex < m.chatScroll {
					m.chatScroll = m.chatIndex
				}
			}
			return m, nil
		case "down":
			if m.chatIndex < len(m.chats)-1 {
				m.chatIndex++
				// Прокрутка вниз если нужно
				visibleHeight := m.height - 5 // Высота видимой области для чатов
				if m.chatIndex >= m.chatScroll+visibleHeight {
					m.chatScroll = m.chatIndex - visibleHeight + 1
				}
			}
			return m, nil
		case "enter":
			if len(m.chats) > 0 {
				chat := m.chats[m.chatIndex]
				m.currentChat = chat.Name
				m.loading = true
				return m, m.loadMessages(chat.Name, chat.ID)
			}
			return m, nil
		case "i":
			if m.currentChat != "" {
				m.inputMode = true
				m.error = "" // Очищаем ошибку при входе в режим ввода
			}
			return m, nil
		case "r", "f5":
			if m.currentChat != "" {
				m.loading = true
				chat := m.chats[m.chatIndex]
				return m, m.loadMessages(chat.Name, chat.ID)
			} else {
				// Если мы не в чате, обновляем список чатов
				m.loading = true
				return m, m.loadChats()
			}
			return m, nil
		case "s":
			// Показать стикеры в новом Kitty терминале
			if m.currentChat != "" {
				showStickersInNewKitty(m.messages)
			}
			return m, nil
		case "tab":
			// Переключение между панелями (чаты -> сообщения -> стикеры)
			// Пока просто переключаем на панель стикеров если есть стикеры
			var stickers []MessageItem
			for _, msg := range m.messages {
				if msg.Type == "sticker" && msg.StickerPath != "" {
					stickers = append(stickers, msg)
				}
			}
			if len(stickers) > 0 {
				// Переключаемся на панель стикеров
				m.stickerPanelIndex = 0
				m.selectedSticker = &stickers[0]
			}
			return m, nil
		case "left":
			// Навигация по стикерам влево
			var stickers []MessageItem
			for _, msg := range m.messages {
				if msg.Type == "sticker" && msg.StickerPath != "" {
					stickers = append(stickers, msg)
				}
			}
			if len(stickers) > 0 && m.stickerPanelIndex > 0 {
				m.stickerPanelIndex--
				m.selectedSticker = &stickers[m.stickerPanelIndex]
			}
			return m, nil
		case "right":
			// Навигация по стикерам вправо
			var stickers []MessageItem
			for _, msg := range m.messages {
				if msg.Type == "sticker" && msg.StickerPath != "" {
					stickers = append(stickers, msg)
				}
			}
			if len(stickers) > 0 && m.stickerPanelIndex < len(stickers)-1 {
				m.stickerPanelIndex++
				m.selectedSticker = &stickers[m.stickerPanelIndex]
			}
			return m, nil
		case "v":
			// Просмотр выбранного стикера в полноэкранном режиме
			if m.selectedSticker != nil {
				// Выходим из TUI для показа стикера
				return m, tea.Quit
			}
			return m, nil
		}

	case loadChatsMsg:
		// Сохраняем текущую позицию скролла
		oldChatScroll := m.chatScroll
		oldChatIndex := m.chatIndex
		wasEmpty := len(m.chats) == 0 // Проверяем, был ли список пустым до обновления

		m.chats = []ChatItem(msg)
		m.loading = false

		// Сбрасываем прокрутку только если это первая загрузка (список был пустой)
		if wasEmpty || len(m.chats) == 0 {
			m.chatScroll = 0
			m.chatIndex = 0
		} else {
			// Восстанавливаем позицию скролла
			m.chatScroll = oldChatScroll
			m.chatIndex = oldChatIndex

			// Проверяем, что скролл не выходит за границы
			if m.chatScroll >= len(m.chats) {
				m.chatScroll = 0
			}
			if m.chatIndex >= len(m.chats) {
				m.chatIndex = len(m.chats) - 1
			}
		}
		return m, nil

	case loadMessagesMsg:
		m.messages = []MessageItem(msg)
		m.loading = false
		return m, nil

	case errorMsg:
		m.error = string(msg)
		m.loading = false
		return m, nil

	case reloadMessagesMsg:
		m.loading = true
		return m, m.loadMessages(msg.chatName, msg.chatID)
	}

	return m, nil
}

func (m model) sendMessage() tea.Cmd {
	if m.currentChat == "" || m.input == "" {
		return nil
	}

	// Найти ID текущего чата
	var chatID int64
	chatName := m.currentChat
	for _, chat := range m.chats {
		if chat.Name == m.currentChat {
			chatID = chat.ID
			break
		}
	}

	message := m.input // Сохраняем сообщение для отправки

	return tea.Tick(time.Millisecond*100, func(time.Time) tea.Msg {
		if m.config.UseMTProto && m.mtproto != nil {
			if err := m.mtproto.SendMessage(m.ctx, chatID, message); err != nil {
				return errorMsg(fmt.Sprintf("Ошибка отправки: %v", err))
			}
		} else if m.telegram != nil {
			if err := m.telegram.SendMessage(chatID, message); err != nil {
				return errorMsg(fmt.Sprintf("Ошибка отправки: %v", err))
			}
		}

		// После отправки загружаем сообщения заново
		return reloadMessagesMsg{chatName: chatName, chatID: chatID}
	})
}

func (m model) View() string {
	if m.width == 0 || m.height == 0 {
		return "Загрузка..."
	}

	// Проверяем, есть ли стикеры для отображения
	var stickers []MessageItem
	for _, msg := range m.messages {
		if msg.Type == "sticker" && msg.StickerPath != "" {
			stickers = append(stickers, msg)
		}
	}

	// Определяем размеры панелей
	leftWidth := m.width / 3
	stickerWidth := 0
	rightWidth := m.width - leftWidth - 1

	// Если есть стикеры, выделяем место для панели стикеров
	if len(stickers) > 0 {
		stickerWidth = m.width / 4                          // 25% ширины для стикеров
		rightWidth = m.width - leftWidth - stickerWidth - 2 // -2 для разделителей
	}

	// левая панель - список чатов
	leftPanel := m.renderChatList(leftWidth, m.height-0)

	// правая панель - сообщения
	rightPanel := m.renderMessages(rightWidth, m.height-0)

	// панель стикеров (если есть)
	stickerPanel := ""
	if len(stickers) > 0 {
		stickerPanel = m.renderStickerPanel(stickerWidth, m.height-0)
	}

	// склейка панелей
	leftLines := strings.Split(leftPanel, "\n")
	rightLines := strings.Split(rightPanel, "\n")
	stickerLines := strings.Split(stickerPanel, "\n")

	// Фиксируем количество строк равным высоте экрана минус заголовки
	maxLines := m.height - 2

	// Дополнительная проверка: все панели должны иметь одинаковую высоту
	if len(leftLines) != maxLines {
		// Обрезаем или дополняем левую панель
		if len(leftLines) > maxLines {
			leftLines = leftLines[:maxLines]
		}
		for len(leftLines) < maxLines {
			leftLines = append(leftLines, "")
		}
	}
	if len(rightLines) != maxLines {
		// Обрезаем или дополняем правую панель
		if len(rightLines) > maxLines {
			rightLines = rightLines[:maxLines]
		}
		for len(rightLines) < maxLines {
			rightLines = append(rightLines, "")
		}
	}
	if len(stickerLines) != maxLines {
		// Обрезаем или дополняем панель стикеров
		if len(stickerLines) > maxLines {
			stickerLines = stickerLines[:maxLines]
		}
		for len(stickerLines) < maxLines {
			stickerLines = append(stickerLines, "")
		}
	}

	lines := make([]string, maxLines)

	for i := 0; i < maxLines; i++ {
		leftLine := ""
		rightLine := ""
		stickerLine := ""

		if i < len(leftLines) {
			leftLine = leftLines[i]
		}
		if i < len(rightLines) {
			rightLine = rightLines[i]
		}
		if i < len(stickerLines) {
			stickerLine = stickerLines[i]
		}

		// Обработка левой панели
		visibleLen := lipgloss.Width(leftLine)
		if visibleLen < leftWidth {
			leftLine += strings.Repeat(" ", leftWidth-visibleLen)
		} else if visibleLen > leftWidth {
			leftLine = leftLine[:leftWidth]
		}

		// Обработка правой панели
		visibleLen = lipgloss.Width(rightLine)
		if visibleLen < rightWidth {
			rightLine += strings.Repeat(" ", rightWidth-visibleLen)
		} else if visibleLen > rightWidth {
			rightLine = rightLine[:rightWidth]
		}

		// Обработка панели стикеров
		if len(stickers) > 0 {
			visibleLen = lipgloss.Width(stickerLine)
			if visibleLen < stickerWidth {
				stickerLine += strings.Repeat(" ", stickerWidth-visibleLen)
			} else if visibleLen > stickerWidth {
				stickerLine = stickerLine[:stickerWidth]
			}
			lines[i] = leftLine + "│" + rightLine + "│" + stickerLine
		} else {
			lines[i] = leftLine + "│" + rightLine
		}
	}

	result := strings.Join(lines, "\n")

	// Добавляем строку состояния
	status := m.renderStatus()
	result += "\n" + strings.Repeat("─", m.width) + "\n" + status

	return result
}

func (m model) renderChatList(width, height int) string {
	var lines []string

	title := titleStyle.Render("Чаты")
	lines = append(lines, title)

	if m.loading && len(m.chats) == 0 {
		loadingText := "Загрузка чатов..."
		lines = append(lines, loadingText)
	} else {
		// Вычисляем видимую область для чатов
		visibleHeight := height - 0 // Вычитаем строки для заголовка

		// Показываем только видимые чаты с учетом прокрутки
		visibleChats := m.chats[m.chatScroll:]
		if len(visibleChats) > visibleHeight {
			visibleChats = visibleChats[:visibleHeight]
		}

		// Добавляем индикатор прокрутки вверх если нужно
		if m.chatScroll > 0 {
			lines = append(lines, helpStyle.Render("↑ Еще чаты выше"))
		}

		// Добавляем чаты
		for i, chat := range visibleChats {
			actualIndex := m.chatScroll + i
			line := chat.Name
			if chat.Unread > 0 {
				line = fmt.Sprintf("(%d) %s", chat.Unread, line)
			}

			// Обрезаем строку если она слишком длинная
			maxLen := width - 2
			if len(line) > maxLen {
				line = line[:maxLen-3] + "..."
			}

			// Применяем стили
			if actualIndex == m.chatIndex {
				line = selectedStyle.Render(line)
			} else {
				line = chatStyle.Render(line)
			}

			lines = append(lines, line)
		}

		// Добавляем индикатор прокрутки вниз если нужно
		if m.chatScroll+len(visibleChats) < len(m.chats) {
			lines = append(lines, helpStyle.Render("↓ Еще чаты ниже"))
		}
	}

	// Обрезаем лишние строки и дополняем до нужной высоты
	if len(lines) > height {
		lines = lines[:height]
	}
	for len(lines) < height {
		lines = append(lines, "")
	}

	return strings.Join(lines, "\n")
}

func (m model) renderMessages(width, height int) string {
	var lines []string

	if m.currentChat == "" {
		title := titleStyle.Render("Выберите чат")
		lines = append(lines, title)
	} else {
		title := titleStyle.Render(fmt.Sprintf("Чат: %s", m.currentChat))
		lines = append(lines, title)
		if os.Getenv("VI_TG_AUTO_KITTY") == "1" {
		}
		if m.loading {
			lines = append(lines, "Загрузка сообщений...")
		} else {
			for _, msg := range m.messages {
				// Форматируем сообщение с фиксированной шириной для времени и имени
				timeStr := messageStyle.Render(fmt.Sprintf("%-5s", msg.Timestamp))
				fromStr := chatStyle.Render(fmt.Sprintf("%-12s", msg.From))
				msgText := msg.Text
				prefix := fmt.Sprintf("%s %s: ", timeStr, fromStr)
				prefixWidth := lipgloss.Width(prefix)
				availableWidth := width - prefixWidth - 2

				if msg.Type == "sticker" && msg.StickerPath != "" {
					// Показываем emoji и информацию о стикере
					stickerLine := prefix
					if msg.StickerEmoji != "" {
						stickerLine += msg.StickerEmoji + " "
					}
					stickerLine += "[стикер]"
					lines = append(lines, stickerLine)

					// ЗАКОММЕНТИРОВАНО: попытка отображения стикеров в сообщениях
					/*
						// Вставляем картинку через Kitty protocol (если поддерживается)
						if isKittySupported() {
							// Добавляем безопасную обработку
							img := kittyImage(msg.StickerPath, availableWidth)
							if strings.Contains(img, "[") {
								// Если произошла ошибка, показываем fallback
								lines = append(lines, strings.Repeat(" ", prefixWidth)+img)
							} else {
								// Пытаемся вывести картинку
								lines = append(lines, strings.Repeat(" ", prefixWidth)+img)
							}
						} else {
							// Если не поддерживается, выводим путь к файлу
							lines = append(lines, strings.Repeat(" ", prefixWidth)+"Файл: "+msg.StickerPath)
						}
					*/

					// Показываем только путь к файлу
					lines = append(lines, strings.Repeat(" ", prefixWidth)+"Файл: "+msg.StickerPath)
					continue
				}

				// Обычные текстовые сообщения
				if availableWidth > 0 {
					words := strings.Fields(msgText)
					if len(words) == 0 {
						lines = append(lines, prefix)
					} else {
						currentLine := ""
						for _, word := range words {
							if len(currentLine)+len(word)+1 <= availableWidth {
								if currentLine != "" {
									currentLine += " "
								}
								currentLine += word
							} else {
								lines = append(lines, prefix+currentLine)
								currentLine = word
								prefix = strings.Repeat(" ", prefixWidth)
							}
						}
						if currentLine != "" {
							lines = append(lines, prefix+currentLine)
						}
					}
				} else {
					lines = append(lines, fmt.Sprintf("%s %s: %s", timeStr, fromStr, msgText))
				}
			}
		}
	}

	// Обрезаем лишние строки и дополняем до нужной высоты
	if len(lines) > height {
		lines = lines[:height]
	}
	for len(lines) < height {
		lines = append(lines, "")
	}

	return strings.Join(lines, "\n")
}

func (m model) renderStatus() string {
	if m.error != "" {
		return errorStyle.Render(fmt.Sprintf("Ошибка: %s", m.error))
	}

	if m.inputMode {
		return fmt.Sprintf("Сообщение: %s", m.input)
	}

	helpText := "q: выход, ↑↓: навигация, Enter: выбор, i: ввод сообщения, r: обновить (чаты/сообщения), s: показать стикеры, Tab: стикеры, ←→: навигация по стикерам, v: просмотр стикера"
	if os.Getenv("VI_TG_AUTO_KITTY") == "1" {
		helpText += " (авто-показ включен)"
	}
	if os.Getenv("VI_TG_NO_INLINE") == "1" {
		helpText += " (встроенные стикеры отключены)"
	}
	return helpStyle.Render(helpText)
}

// renderStickerPanel отображает панель со стикерами в правом нижнем углу
func (m model) renderStickerPanel(width, height int) string {
	var lines []string

	// Собираем все стикеры из сообщений
	var stickers []MessageItem
	for _, msg := range m.messages {
		if msg.Type == "sticker" && msg.StickerPath != "" {
			stickers = append(stickers, msg)
		}
	}

	if len(stickers) == 0 {
		return ""
	}

	// Заголовок панели с количеством стикеров
	title := titleStyle.Render(fmt.Sprintf("Стикеры (%d)", len(stickers)))
	lines = append(lines, title)

	// Ограничиваем количество отображаемых стикеров
	maxStickers := height - 3 // Оставляем место для заголовка и разделителя
	if len(stickers) > maxStickers {
		stickers = stickers[:maxStickers]
	}

	// Добавляем разделитель
	lines = append(lines, strings.Repeat("─", width))

	for i, sticker := range stickers {
		// Строка с номером и информацией
		infoLine := fmt.Sprintf("%d. ", i+1)
		if sticker.StickerEmoji != "" {
			infoLine += sticker.StickerEmoji + " "
		}
		infoLine += "стикер"
		if sticker.Timestamp != "" {
			infoLine += fmt.Sprintf(" (%s)", sticker.Timestamp)
		}

		// Обрезаем строку если она слишком длинная
		if len(infoLine) > width-2 {
			infoLine = infoLine[:width-5] + "..."
		}

		// Применяем стили в зависимости от выбора
		if i == m.stickerPanelIndex {
			infoLine = selectedStyle.Render(infoLine)
		} else {
			infoLine = chatStyle.Render(infoLine)
		}

		lines = append(lines, infoLine)

		// Добавляем картинку стикера если поддерживается Kitty
		// ЗАКОММЕНТИРОВАНО: попытка отображения стикеров в панели
		/*
			if isKittySupported() && sticker.StickerPath != "" {
				// Определяем путь к изображению
				var imagePath string
				if strings.HasSuffix(sticker.StickerPath, ".webp") {
					// Для WebM файлов ищем PNG версию
					pngPath := strings.Replace(sticker.StickerPath, ".webp", ".png", 1)
					if _, err := os.Stat(pngPath); err == nil {
						imagePath = pngPath
					} else {
						imagePath = sticker.StickerPath
					}
				} else {
					imagePath = sticker.StickerPath
				}

				// Проверяем, что файл существует
				if _, err := os.Stat(imagePath); err == nil {
					// Вычисляем размер изображения для панели
					imageWidth := width - 2 // Оставляем отступы
					if imageWidth > 20 {    // Минимальная ширина
						img := kittyImage(imagePath, imageWidth)
						if !strings.Contains(img, "[") { // Если нет ошибки
							lines = append(lines, img)
						} else {
							lines = append(lines, "  [ошибка загрузки]")
						}
					} else {
						lines = append(lines, "  [слишком узко]")
					}
				} else {
					lines = append(lines, "  [файл не найден]")
				}
			} else {
				// Если Kitty не поддерживается, показываем путь к файлу
				fileInfo := "  Файл: " + sticker.StickerPath
				if len(fileInfo) > width-2 {
					fileInfo = fileInfo[:width-5] + "..."
				}
				lines = append(lines, messageStyle.Render(fileInfo))
			}
		*/

		// Показываем только путь к файлу
		fileInfo := "  Файл: " + sticker.StickerPath
		if len(fileInfo) > width-2 {
			fileInfo = fileInfo[:width-5] + "..."
		}
		lines = append(lines, messageStyle.Render(fileInfo))

		// Добавляем пустую строку между стикерами
		if i < len(stickers)-1 {
			lines = append(lines, "")
		}
	}

	// Обрезаем лишние строки и дополняем до нужной высоты
	if len(lines) > height {
		lines = lines[:height]
	}
	for len(lines) < height {
		lines = append(lines, "")
	}

	return strings.Join(lines, "\n")
}

// isKittySupported проверяет, поддерживает ли терминал Kitty graphics protocol
func isKittySupported() bool {
	// Проверяем, не отключен ли Kitty через переменную окружения
	if os.Getenv("VI_TG_NO_KITTY") == "1" {
		return false
	}

	term := os.Getenv("TERM")
	kittyTerm := os.Getenv("KITTY_WINDOW_ID")

	// Более точная проверка Kitty терминала
	isKitty := term == "xterm-kitty" || strings.Contains(term, "kitty") || kittyTerm != ""

	if isKitty {
		fmt.Printf("DEBUG: Kitty терминал обнаружен (TERM=%s, KITTY_WINDOW_ID=%s)\n", term, kittyTerm)
	}

	return isKitty
}

// checkImageFormat проверяет формат изображения по заголовку файла
func checkImageFormat(data []byte) string {
	if len(data) < 4 {
		return "unknown"
	}

	// WebM файлы начинаются с EBML header (1A 45 DF A3) и содержат "webm"
	if len(data) >= 4 && data[0] == 0x1A && data[1] == 0x45 && data[2] == 0xDF && data[3] == 0xA3 {
		// Проверяем, что это WebM (анимированный стикер)
		if len(data) >= 20 {
			dataStr := string(data[:50]) // Проверяем первые 50 байт
			if strings.Contains(dataStr, "webm") {
				return "webm"
			}
		}
	}

	// WebP файлы начинаются с "RIFF" и содержат "WEBP"
	if len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP" {
		return "webp"
	}

	// PNG файлы начинаются с PNG signature
	if len(data) >= 8 && string(data[:8]) == "\x89PNG\r\n\x1a\n" {
		return "png"
	}

	// JPEG файлы начинаются с FF D8 FF
	if len(data) >= 3 && data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF {
		return "jpeg"
	}

	return "unknown"
}

// convertWebmToPng конвертирует WebM файл в PNG с помощью ffmpeg
func convertWebmToPng(webmPath string) (string, error) {
	// Создаем путь для png файла
	pngPath := strings.Replace(webmPath, ".webp", ".png", 1)

	// Проверяем, не создан ли уже png файл
	if _, err := os.Stat(pngPath); err == nil {
		return pngPath, nil
	}

	// Проверяем, что ffmpeg доступен
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return "", fmt.Errorf("ffmpeg не найден: %v", err)
	}

	// Конвертируем WebM в PNG (первый кадр) с таймаутом
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "ffmpeg", "-i", webmPath, "-vframes", "1", "-f", "image2", pngPath, "-y")

	// Подавляем вывод ffmpeg
	cmd.Stdout = nil
	cmd.Stderr = nil

	fmt.Printf("DEBUG: Запуск ffmpeg для конвертации %s\n", webmPath)

	if err := cmd.Run(); err != nil {
		// Удаляем частично созданный файл при ошибке
		os.Remove(pngPath)
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("таймаут конвертации ffmpeg (>10s)")
		}
		return "", fmt.Errorf("ошибка конвертации ffmpeg: %v", err)
	}

	fmt.Printf("DEBUG: Конвертация завершена успешно\n")

	// Проверяем, что файл создался
	if _, err := os.Stat(pngPath); err != nil {
		return "", fmt.Errorf("PNG файл не создался: %v", err)
	}

	return pngPath, nil
}

// processWebMAsync асинхронно обрабатывает WebM файл
func processWebMAsync(data []byte, path string) {
	fmt.Printf("DEBUG: Асинхронная обработка WebM файла %s\n", path)

	// Проверяем, не слишком ли большой файл
	if len(data) > 1024*1024 { // 1MB
		fmt.Printf("DEBUG: Файл слишком большой (%d байт), пропускаем\n", len(data))
		return
	}

	pngPath, err := convertWebmToPng(path)
	if err != nil {
		fmt.Printf("DEBUG: Ошибка конвертации WebM в PNG: %v\n", err)
		return
	}

	fmt.Printf("DEBUG: WebM сконвертирован в PNG: %s\n", pngPath)

	// Показываем стикер в новом Kitty терминале если включено
	if os.Getenv("VI_TG_AUTO_KITTY") == "1" {
		if err := showStickerInNewKitty(pngPath); err != nil {
			fmt.Printf("DEBUG: Не удалось открыть новый Kitty: %v\n", err)
		}
	}
}

// showStickerInNewKitty открывает новый Kitty терминал и показывает стикер
func showStickerInNewKitty(imagePath string) error {
	// Проверяем, что Kitty доступен
	if _, err := exec.LookPath("kitty"); err != nil {
		return fmt.Errorf("kitty не найден: %v", err)
	}

	// Запускаем новый Kitty терминал с kitten icat
	cmd := exec.Command("kitty", "--hold", "-e", "kitten", "icat", imagePath)

	// Запускаем в фоне
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("ошибка запуска Kitty: %v", err)
	}

	fmt.Printf("DEBUG: Запущен новый Kitty терминал для отображения %s\n", imagePath)
	return nil
}

// showStickersInNewKitty показывает все стикеры из сообщений в новых Kitty терминалах
func showStickersInNewKitty(messages []MessageItem) {
	stickersShown := 0
	maxStickers := 5 // Ограничиваем количество одновременно открываемых терминалов

	for _, msg := range messages {
		if msg.Type == "sticker" && msg.StickerPath != "" && stickersShown < maxStickers {
			// Проверяем, есть ли PNG версия файла
			var imagePath string
			if strings.HasSuffix(msg.StickerPath, ".webp") {
				// Для WebM файлов ищем PNG версию
				pngPath := strings.Replace(msg.StickerPath, ".webp", ".png", 1)
				if _, err := os.Stat(pngPath); err == nil {
					imagePath = pngPath
				} else {
					imagePath = msg.StickerPath
				}
			} else {
				imagePath = msg.StickerPath
			}

			if err := showStickerInNewKitty(imagePath); err != nil {
				fmt.Printf("DEBUG: Не удалось показать стикер %s: %v\n", imagePath, err)
			} else {
				stickersShown++
				// Небольшая задержка между открытиями терминалов
				time.Sleep(100 * time.Millisecond)
			}
		}
	}

	if stickersShown == 0 {
		fmt.Printf("DEBUG: Стикеры не найдены в текущих сообщениях\n")
	} else {
		fmt.Printf("DEBUG: Показано %d стикеров в новых Kitty терминалах", stickersShown)
		if stickersShown >= maxStickers {
			fmt.Printf(" (максимум %d за раз)", maxStickers)
		}
		fmt.Printf("\n")
	}
}

// kittyImage возвращает escape-последовательность для вывода картинки через Kitty protocol
func kittyImage(path string, width int) string {
	// Проверяем существование файла
	if _, err := os.Stat(path); err != nil {
		return "[файл стикера не найден]"
	}

	// Читаем файл с таймаутом через goroutine
	type result struct {
		data []byte
		err  error
	}

	ch := make(chan result, 1)
	go func() {
		data, err := os.ReadFile(path)
		ch <- result{data, err}
	}()

	select {
	case res := <-ch:
		if res.err != nil {
			return "[ошибка чтения стикера]"
		}
		data := res.data

		// Проверяем размер файла (не более 500KB для безопасности)
		if len(data) > 500*1024 {
			return fmt.Sprintf("[стикер слишком большой: %d байт]", len(data))
		}

		// Проверяем, что это действительно изображение
		if len(data) < 10 {
			return "[неверный формат стикера]"
		}

		// Проверяем формат файла
		format := checkImageFormat(data)
		fmt.Printf("DEBUG: Формат файла %s: %s\n", path, format)

		return processImageDataWithSize(data, path, format, width)

	case <-time.After(5 * time.Second):
		return "[таймаут чтения стикера]"
	}
}

// processImageDataWithSize обрабатывает данные изображения с указанным размером
func processImageDataWithSize(data []byte, path string, format string, width int) string {
	// Для WebM файлов запускаем обработку в фоне
	if format == "webm" {
		go func() {
			processWebMAsync(data, path)
		}()
		return fmt.Sprintf("🎬 [WebM стикер обрабатывается...]")
	}

	switch format {
	case "webp", "png", "jpeg":
		// Статические изображения
		fmt.Printf("DEBUG: Обрабатываем %s изображение размером %d байт\n", format, len(data))

		// Проверяем размер файла
		if len(data) > 1024*1024 { // 1MB
			fmt.Printf("DEBUG: Файл слишком большой (%d байт), показываем только путь\n", len(data))
			return fmt.Sprintf("[%s изображение: %s]", format, path)
		}

		// Показываем в новом Kitty терминале асинхронно (только если включено)
		if os.Getenv("VI_TG_AUTO_KITTY") == "1" {
			go func() {
				if err := showStickerInNewKitty(path); err != nil {
					fmt.Printf("DEBUG: Не удалось открыть новый Kitty: %v\n", err)
				}
			}()
		}

		// Отображаем встроенно через Kitty graphics protocol
		// ЗАКОММЕНТИРОВАНО: попытка отображения стикеров в том же окне терминала
		/*
			if isKittySupported() && os.Getenv("VI_TG_NO_INLINE") != "1" {
				// Кодируем данные в base64
				encoded := fmt.Sprintf("\033_Ga=T,f=100,s=%d,v=%d;S=%d;a=%s\033\\",
					len(data), len(data), width, base64.StdEncoding.EncodeToString(data))
				return encoded
			}
		*/

		// Если Kitty не поддерживается, показываем информацию
		fmt.Printf("DEBUG: %s изображение готово для отображения\n", format)
		return fmt.Sprintf("🖼️ [%s стикер: %s]", format, path)
	default:
		return "[неизвестный формат изображения]"
	}
}

// showStickerFullscreen показывает стикер в полноэкранном режиме
func showStickerFullscreen(sticker *MessageItem) error {
	if sticker == nil || sticker.StickerPath == "" {
		return fmt.Errorf("стикер не найден")
	}

	// Определяем путь к изображению
	var imagePath string
	if strings.HasSuffix(sticker.StickerPath, ".webp") {
		// Для WebM файлов ищем PNG версию
		pngPath := strings.Replace(sticker.StickerPath, ".webp", ".png", 1)
		if _, err := os.Stat(pngPath); err == nil {
			imagePath = pngPath
		} else {
			imagePath = sticker.StickerPath
		}
	} else {
		imagePath = sticker.StickerPath
	}

	// Проверяем, что файл существует
	if _, err := os.Stat(imagePath); err != nil {
		return fmt.Errorf("файл стикера не найден: %s", imagePath)
	}

	// Очищаем экран
	fmt.Print("\033[2J\033[H")

	// Показываем информацию о стикере
	fmt.Printf("\n\n")
	fmt.Printf("Стикер: %s\n", imagePath)
	if sticker.StickerEmoji != "" {
		fmt.Printf("Эмодзи: %s\n", sticker.StickerEmoji)
	}
	if sticker.Timestamp != "" {
		fmt.Printf("Время: %s\n", sticker.Timestamp)
	}
	fmt.Printf("\n")

	// Показываем стикер через Kitty graphics
	if isKittySupported() {
		// Читаем файл
		data, err := os.ReadFile(imagePath)
		if err != nil {
			return fmt.Errorf("ошибка чтения файла: %v", err)
		}

		// Проверяем размер
		if len(data) > 1024*1024 { // 1MB
			return fmt.Errorf("файл слишком большой: %d байт", len(data))
		}

		// Проверяем формат
		format := checkImageFormat(data)
		if format == "webm" {
			// Для WebM файлов конвертируем в PNG
			pngPath, err := convertWebmToPng(imagePath)
			if err != nil {
				return fmt.Errorf("ошибка конвертации WebM: %v", err)
			}
			data, err = os.ReadFile(pngPath)
			if err != nil {
				return fmt.Errorf("ошибка чтения PNG: %v", err)
			}
		}

		// Выводим через Kitty graphics
		encoded := fmt.Sprintf("\033_Ga=T,f=100,s=%d,v=%d;a=%s\033\\",
			len(data), len(data), base64.StdEncoding.EncodeToString(data))
		fmt.Print(encoded)

		fmt.Printf("\n\n")
		fmt.Printf("Нажмите любую клавишу для возврата...\n")

		// Ждём нажатия клавиши
		var buf [1]byte
		os.Stdin.Read(buf[:])

		return nil
	} else {
		// Если Kitty не поддерживается, показываем путь
		fmt.Printf("Kitty не поддерживается. Файл: %s\n", imagePath)
		fmt.Printf("Нажмите любую клавишу для возврата...\n")

		var buf [1]byte
		os.Stdin.Read(buf[:])

		return nil
	}
}

// processImageData обрабатывает данные изображения
func processImageData(data []byte, path string, format string) string {
	// Для WebM файлов запускаем обработку в фоне
	if format == "webm" {
		go func() {
			processWebMAsync(data, path)
		}()
		return fmt.Sprintf("🎬 [WebM стикер обрабатывается...]")
	}

	switch format {
	case "webp", "png", "jpeg":
		// Статические изображения
		fmt.Printf("DEBUG: Обрабатываем %s изображение размером %d байт\n", format, len(data))

		// Проверяем размер файла
		if len(data) > 1024*1024 { // 1MB
			fmt.Printf("DEBUG: Файл слишком большой (%d байт), показываем только путь\n", len(data))
			return fmt.Sprintf("[%s изображение: %s]", format, path)
		}

		// Показываем в новом Kitty терминале асинхронно (только если включено)
		if os.Getenv("VI_TG_AUTO_KITTY") == "1" {
			go func() {
				if err := showStickerInNewKitty(path); err != nil {
					fmt.Printf("DEBUG: Не удалось открыть новый Kitty: %v\n", err)
				}
			}()
		}

		// Отображаем встроенно через Kitty graphics protocol
		if isKittySupported() {
			// Кодируем данные в base64
			encoded := fmt.Sprintf("\033_Ga=T,f=100,s=%d,v=%d;a=%s\033\\",
				len(data), len(data), base64.StdEncoding.EncodeToString(data))
			return encoded
		}

		// Если Kitty не поддерживается, показываем информацию
		fmt.Printf("DEBUG: %s изображение готово для отображения\n", format)
		return fmt.Sprintf("🖼️ [%s стикер: %s]", format, path)
	default:
		return "[неизвестный формат изображения]"
	}
}

func main() {
	for {
		p := tea.NewProgram(initialModel(), tea.WithAltScreen())
		m, err := p.Run()
		if err != nil {
			log.Fatal(err)
		}

		// Проверяем, нужно ли показать стикер
		if model, ok := m.(model); ok && model.selectedSticker != nil {
			// Показываем стикер в полноэкранном режиме
			if err := showStickerFullscreen(model.selectedSticker); err != nil {
				fmt.Printf("Ошибка показа стикера: %v\n", err)
				fmt.Printf("Нажмите любую клавишу для продолжения...\n")
				var buf [1]byte
				os.Stdin.Read(buf[:])
			}

			// Очищаем выбранный стикер и продолжаем работу
			model.selectedSticker = nil
			model.stickerPanelIndex = 0

			// Продолжаем цикл (перезапускаем TUI)
			continue
		}

		// Если не нужно показывать стикер, выходим
		break
	}
}
