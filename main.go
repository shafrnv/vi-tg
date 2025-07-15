package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"vi-tg/auth"
	"vi-tg/config"
	"vi-tg/telegram"
)

type TelegramClient struct {
	app         *tview.Application
	telegram    *telegram.Client
	mtproto     *auth.MTProtoClient
	config      *config.Config
	chatList    *tview.List
	messageView *tview.TextView
	inputField  *tview.InputField
	currentChat string
	chats       map[string]int64
	ctx         context.Context
}

func NewTelegramClient() (*TelegramClient, error) {
	cfg, err := config.LoadConfig()
	if err != nil {
		return nil, fmt.Errorf("ошибка загрузки конфига: %w", err)
	}

	var tgClient *telegram.Client
	var mtprotoClient *auth.MTProtoClient

	if cfg.UseMTProto {
		mtprotoClient = auth.NewMTProtoClient()
	} else if cfg.TelegramToken != "" {
		tgClient, err = telegram.NewClient(cfg.TelegramToken)
		if err != nil {
			return nil, fmt.Errorf("ошибка создания Telegram клиента: %w", err)
		}
	}

	return &TelegramClient{
		app:      tview.NewApplication(),
		telegram: tgClient,
		mtproto:  mtprotoClient,
		config:   cfg,
		chats:    make(map[string]int64),
		ctx:      context.Background(),
	}, nil
}

func (tc *TelegramClient) setupUI() {
	layout := tview.NewFlex().SetDirection(tview.FlexColumn)

	tc.chatList = tview.NewList().
		ShowSecondaryText(false).
		SetSelectedFunc(func(index int, mainText string, secondaryText string, shortcut rune) {
			tc.currentChat = mainText
			tc.loadMessages(mainText)
		})

	rightPanel := tview.NewFlex().SetDirection(tview.FlexRow)

	tc.messageView = tview.NewTextView().
		SetDynamicColors(true).
		SetRegions(true).
		SetWordWrap(true)

	tc.inputField = tview.NewInputField().
		SetLabel("Сообщение: ").
		SetFieldWidth(0).
		SetDoneFunc(func(key tcell.Key) {
			if key == tcell.KeyEnter {
				tc.sendMessage()
			}
		})

	rightPanel.AddItem(tc.messageView, 0, 1, false).
		AddItem(tc.inputField, 1, 0, true)

	layout.AddItem(tc.chatList, 20, 0, true).
		AddItem(rightPanel, 0, 1, false)

	tc.app.SetRoot(layout, true)
}

func (tc *TelegramClient) initializeAuth() error {
	if tc.config.UseMTProto {
		return tc.initializeMTProto()
	}
	return nil
}

func (tc *TelegramClient) initializeMTProto() error {
	fmt.Println("Инициализация MTProto...")
	
	if tc.config.PhoneNumber == "" {
		fmt.Print("Введите номер телефона (с кодом страны): ")
		var phone string
		fmt.Scanln(&phone)
		tc.config.PhoneNumber = phone
		config.SaveConfig(tc.config)
	}
	
	fmt.Printf("Используется номер: %s\n", tc.config.PhoneNumber)
	fmt.Println("Авторизация через MTProto...")
	
	if err := tc.mtproto.AuthAndConnect(tc.ctx, tc.config.PhoneNumber); err != nil {
		fmt.Printf("Ошибка MTProto авторизации: %v\n", err)
		return fmt.Errorf("ошибка авторизации MTProto: %w", err)
	}
	
	fmt.Println("MTProto авторизация успешна!")
	return nil
}

func (tc *TelegramClient) loadChats() {
	tc.chatList.Clear()

	if tc.config.UseMTProto && tc.mtproto != nil {
		fmt.Println("Загрузка диалогов через MTProto...")
		dialogs, err := tc.mtproto.GetDialogs(tc.ctx)
		if err != nil {
			fmt.Printf("Ошибка загрузки диалогов: %v\n", err)
			tc.chatList.AddItem("[red]Ошибка загрузки диалогов[white]", "", 0, nil)
			tc.chatList.AddItem(err.Error(), "", 0, nil)
			return
		}
		fmt.Printf("Загружено %d диалогов\n", len(dialogs))
		for _, dialog := range dialogs {
			displayName := dialog.Title
			if dialog.Unread > 0 {
				displayName = fmt.Sprintf("[red](%d)[white] %s", dialog.Unread, dialog.Title)
			}
			tc.chatList.AddItem(displayName, "", 0, nil)
			tc.chats[dialog.Title] = dialog.ID
		}
	} else if tc.telegram != nil {
		fmt.Println("Загрузка чатов через Bot API...")
		chats, err := tc.telegram.GetChats()
		if err != nil {
			tc.chatList.AddItem("[red]Ошибка загрузки чатов[white]", "", 0, nil)
			return
		}
		for _, chat := range chats {
			tc.chatList.AddItem(chat.Name, "", 0, nil)
			tc.chats[chat.Name] = chat.ID
		}
	} else {
		fmt.Println("Telegram не подключен, показываем тестовые данные")
		tc.chatList.AddItem("[red]Telegram не подключен[white]", "", 0, nil)
		tc.chatList.AddItem("Настройте авторизацию в конфиге", "", 0, nil)
	}
}

func (tc *TelegramClient) loadMessages(chatName string) {
	tc.messageView.Clear()
	fmt.Fprintf(tc.messageView, "[yellow]Чат: %s[white]\n\n", chatName)

	chatID, exists := tc.chats[chatName]
	if !exists {
		fmt.Fprintf(tc.messageView, "[red]Чат не найден[white]\n")
		return
	}

	if tc.config.UseMTProto && tc.mtproto != nil {
		fmt.Fprintf(tc.messageView, "[gray]Загрузка сообщений через MTProto пока не реализована[white]\n")
		// Здесь можно реализовать загрузку сообщений через MTProto
	} else if tc.telegram != nil {
		messages, err := tc.telegram.GetMessages(chatID, 50)
		if err != nil {
			fmt.Fprintf(tc.messageView, "[red]Ошибка загрузки сообщений: %s[white]\n", err)
			return
		}
		for _, msg := range messages {
			timestamp := msg.Timestamp.Format("15:04")
			fmt.Fprintf(tc.messageView, "[blue]%s[white] [gray]%s[white]: %s\n", msg.From, timestamp, msg.Text)
		}
	} else {
		fmt.Fprintf(tc.messageView, "[red]Telegram не подключен[white]\n")
	}
}

func (tc *TelegramClient) sendMessage() {
	message := tc.inputField.GetText()
	if message == "" {
		return
	}

	chatID, exists := tc.chats[tc.currentChat]
	if !exists {
		fmt.Fprintf(tc.messageView, "\n[red]Чат не выбран[white]\n")
		tc.inputField.SetText("")
		return
	}

	if tc.config.UseMTProto && tc.mtproto != nil {
		fmt.Fprintf(tc.messageView, "\n[gray]Отправка сообщений через MTProto пока не реализована[white]\n")
		// Здесь можно реализовать отправку сообщений через MTProto
	} else if tc.telegram != nil {
		if err := tc.telegram.SendMessage(chatID, message); err != nil {
			fmt.Fprintf(tc.messageView, "\n[red]Ошибка отправки: %s[white]\n", err)
		} else {
			fmt.Fprintf(tc.messageView, "\n[green]Вы: %s[white]\n", message)
		}
	} else {
		fmt.Fprintf(tc.messageView, "\n[red]Telegram не подключен[white]\n")
	}
	tc.inputField.SetText("")
}

func (tc *TelegramClient) Run() error {
	if err := tc.initializeAuth(); err != nil {
		return fmt.Errorf("ошибка инициализации авторизации: %w", err)
	}
	tc.setupUI()
	tc.loadChats()
	tc.app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyCtrlQ {
			tc.app.Stop()
			return nil
		}
		return event
	})
	return tc.app.Run()
}

func main() {
	client, err := NewTelegramClient()
	if err != nil {
		log.Fatal(err)
		os.Exit(1)
	}
	if err := client.Run(); err != nil {
		log.Fatal(err)
		os.Exit(1)
	}
} 