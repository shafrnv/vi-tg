package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"vi-tg/auth"
	"vi-tg/config"

	"github.com/gorilla/mux"
	"github.com/rs/cors"
)

type APIServer struct {
	mtproto *auth.MTProtoClient
	config  *config.Config
	ctx     context.Context
}

// API Response types
type APIResponse struct {
	Success bool   `json:"success,omitempty"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
	Code    int    `json:"code,omitempty"`
}

type AuthStatusResponse struct {
	Authorized  bool   `json:"authorized"`
	PhoneNumber string `json:"phone_number,omitempty"`
	NeedsCode   bool   `json:"needs_code"`
}

type PhoneRequest struct {
	Phone string `json:"phone"`
}

type PhoneResponse struct {
	Success   bool   `json:"success"`
	Message   string `json:"message"`
	NeedsCode bool   `json:"needs_code"`
}

type CodeRequest struct {
	Code string `json:"code"`
}

type CodeResponse struct {
	Success    bool   `json:"success"`
	Message    string `json:"message"`
	Authorized bool   `json:"authorized"`
}

type ChatResponse struct {
	ID          int64   `json:"id"`
	Title       string  `json:"title"`
	Type        string  `json:"type"`
	Unread      int     `json:"unread"`
	LastMessage *string `json:"last_message"`
}

type ChatsResponse struct {
	Chats []ChatResponse `json:"chats"`
}

type MessageResponse struct {
	ID               int     `json:"id"`
	Text             string  `json:"text"`
	From             string  `json:"from"`
	Timestamp        string  `json:"timestamp"`
	ChatID           int64   `json:"chat_id"`
	Type             string  `json:"type"`
	StickerID        *int64  `json:"sticker_id"`
	StickerEmoji     *string `json:"sticker_emoji"`
	StickerPath      *string `json:"sticker_path"`
	ImageID          *int64  `json:"image_id"`
	ImagePath        *string `json:"image_path"`
	VideoID          *int64  `json:"video_id"`
	VideoPath        *string `json:"video_path"`
	VideoPreviewPath *string `json:"video_preview_path"`
	VideoIsRound     *bool   `json:"video_is_round"`
	VoiceID          *int64  `json:"voice_id"`
	VoicePath        *string `json:"voice_path"`
	VoiceDuration    *int    `json:"voice_duration"`
	AudioID          *int64  `json:"audio_id"`
	AudioPath        *string `json:"audio_path"`
	AudioDuration    *int    `json:"audio_duration"`
	AudioTitle       *string `json:"audio_title"`
	AudioArtist      *string `json:"audio_artist"`
}

type MessagesResponse struct {
	Messages []MessageResponse `json:"messages"`
}

type SendMessageRequest struct {
	Text string `json:"text"`
}

type SendMessageResponse struct {
	Success   bool   `json:"success"`
	Message   string `json:"message"`
	MessageID *int   `json:"message_id"`
}

func NewAPIServer() *APIServer {
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatal("Ошибка загрузки конфигурации:", err)
	}

	mtproto := auth.NewMTProtoClient()

	return &APIServer{
		mtproto: mtproto,
		config:  cfg,
		ctx:     context.Background(),
	}
}

func (s *APIServer) Start() error {
	r := mux.NewRouter()

	// API routes
	api := r.PathPrefix("/api").Subrouter()

	// Auth endpoints
	api.HandleFunc("/auth/status", s.getAuthStatus).Methods("GET")
	api.HandleFunc("/auth/phone", s.setPhoneNumber).Methods("POST")
	api.HandleFunc("/auth/code", s.sendCode).Methods("POST")

	// Chat endpoints
	api.HandleFunc("/chats", s.getChats).Methods("GET")
	api.HandleFunc("/chats/{chat_id}/messages", s.getMessages).Methods("GET")
	api.HandleFunc("/chats/{chat_id}/messages", s.sendMessage).Methods("POST")

	// Sticker endpoints
	api.HandleFunc("/stickers/{sticker_id}", s.getSticker).Methods("GET")

	// Image endpoints
	api.HandleFunc("/images/{image_id}", s.getImage).Methods("GET")

	// Video endpoints
	api.HandleFunc("/videos/{video_id}", s.getVideo).Methods("GET")

	// Voice endpoints
	api.HandleFunc("/voices/{voice_id}", s.getVoice).Methods("GET")

	// Audio endpoints
	api.HandleFunc("/audios/{audio_id}", s.getAudio).Methods("GET")

	// Health check
	r.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	// CORS
	c := cors.New(cors.Options{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders: []string{"*"},
	})

	handler := c.Handler(r)

	server := &http.Server{
		Addr:         ":8080",
		Handler:      handler,
		WriteTimeout: 30 * time.Second,
		ReadTimeout:  30 * time.Second,
	}

	// Graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Println("Запуск HTTP сервера на порту 8080...")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal("Ошибка запуска сервера:", err)
		}
	}()

	<-stop
	log.Println("Остановка сервера...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return server.Shutdown(ctx)
}

func (s *APIServer) getAuthStatus(w http.ResponseWriter, r *http.Request) {
	// Проверяем состояние авторизации
	authorized := s.mtproto.IsAuthorized()
	phoneNumber := s.config.PhoneNumber

	// Проверяем, нужен ли код подтверждения
	needsCode := false
	if _, err := os.Stat("/tmp/vi-tg-needs-code"); err == nil {
		needsCode = true
	}

	response := AuthStatusResponse{
		Authorized:  authorized,
		PhoneNumber: phoneNumber,
		NeedsCode:   needsCode,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (s *APIServer) setPhoneNumber(w http.ResponseWriter, r *http.Request) {
	var req PhoneRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.sendError(w, "Неверный формат запроса", http.StatusBadRequest)
		return
	}

	if req.Phone == "" {
		s.sendError(w, "Номер телефона не может быть пустым", http.StatusBadRequest)
		return
	}

	// Сохраняем номер телефона в конфигурации
	s.config.PhoneNumber = req.Phone
	if err := config.SaveConfig(s.config); err != nil {
		s.sendError(w, "Ошибка сохранения конфигурации", http.StatusInternalServerError)
		return
	}

	// Пытаемся авторизоваться
	go func() {
		if err := s.mtproto.AuthAndConnect(s.ctx, req.Phone); err != nil {
			log.Printf("Ошибка авторизации: %v", err)
		}
	}()

	response := PhoneResponse{
		Success:   true,
		Message:   "Номер телефона установлен, код отправлен",
		NeedsCode: true,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (s *APIServer) sendCode(w http.ResponseWriter, r *http.Request) {
	var req CodeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.sendError(w, "Неверный формат запроса", http.StatusBadRequest)
		return
	}

	if req.Code == "" {
		s.sendError(w, "Код не может быть пустым", http.StatusBadRequest)
		return
	}

	// Записываем код в файл для MTProto клиента
	codeFile := "/tmp/vi-tg-auth-code"
	if err := os.WriteFile(codeFile, []byte(req.Code), 0644); err != nil {
		s.sendError(w, "Ошибка записи кода", http.StatusInternalServerError)
		return
	}

	// Ждем некоторое время для обработки кода
	time.Sleep(2 * time.Second)

	// Проверяем, авторизован ли клиент
	authorized := s.mtproto.IsAuthorized()

	response := CodeResponse{
		Success:    true,
		Message:    "Код обработан",
		Authorized: authorized,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (s *APIServer) getChats(w http.ResponseWriter, r *http.Request) {
	if !s.mtproto.IsAuthorized() {
		s.sendError(w, "Не авторизован", http.StatusUnauthorized)
		return
	}

	dialogs, err := s.mtproto.GetDialogs(s.ctx)
	if err != nil {
		s.sendError(w, fmt.Sprintf("Ошибка получения чатов: %v", err), http.StatusInternalServerError)
		return
	}

	chats := make([]ChatResponse, 0, len(dialogs))
	for _, dialog := range dialogs {
		chat := ChatResponse{
			ID:     dialog.ID,
			Title:  dialog.Title,
			Type:   dialog.Type,
			Unread: dialog.Unread,
		}

		if dialog.LastMsg != "" {
			chat.LastMessage = &dialog.LastMsg
		}

		chats = append(chats, chat)
	}

	response := ChatsResponse{
		Chats: chats,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (s *APIServer) getMessages(w http.ResponseWriter, r *http.Request) {
	if !s.mtproto.IsAuthorized() {
		s.sendError(w, "Не авторизован", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	chatIDStr := vars["chat_id"]
	chatID, err := strconv.ParseInt(chatIDStr, 10, 64)
	if err != nil {
		s.sendError(w, "Неверный ID чата", http.StatusBadRequest)
		return
	}

	limitStr := r.URL.Query().Get("limit")
	limit := 50
	if limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil {
			limit = parsedLimit
		}
	}

	messages, err := s.mtproto.GetMessages(s.ctx, chatID, limit)
	if err != nil {
		s.sendError(w, fmt.Sprintf("Ошибка получения сообщений: %v", err), http.StatusInternalServerError)
		return
	}

	messageResponses := make([]MessageResponse, 0, len(messages))
	for _, msg := range messages {
		msgResponse := MessageResponse{
			ID:        msg.ID,
			Text:      msg.Text,
			From:      msg.From,
			Timestamp: msg.Timestamp.Format(time.RFC3339),
			ChatID:    msg.ChatID,
			Type:      msg.Type,
		}

		if msg.StickerID != 0 {
			msgResponse.StickerID = &msg.StickerID
		}

		if msg.StickerEmoji != "" {
			msgResponse.StickerEmoji = &msg.StickerEmoji
		}

		if msg.StickerPath != "" {
			msgResponse.StickerPath = &msg.StickerPath
		}

		// Add support for image paths
		if msg.Type == "photo" {
			imageID := int64(msg.ID)
			// Всегда устанавливаем ImageID для фото
			msgResponse.ImageID = &imageID

			// Проверяем различные форматы изображений
			possibleExtensions := []string{".jpg", ".jpeg", ".png", ".webp", ".gif"}
			for _, ext := range possibleExtensions {
				imagePath := fmt.Sprintf("/tmp/vi-tg_image_%d%s", imageID, ext)
				if _, err := os.Stat(imagePath); err == nil {
					msgResponse.ImagePath = &imagePath
					break
				}
			}
		}

		// Add support for video paths
		if msg.Type == "video" {
			videoID := int64(msg.ID)
			// Всегда устанавливаем VideoID для видео
			msgResponse.VideoID = &videoID

			// Проверяем различные форматы видео
			videoExtensions := []string{".mp4", ".avi", ".mkv", ".mov", ".webm", ".flv"}
			for _, ext := range videoExtensions {
				videoPath := fmt.Sprintf("/tmp/vi-tg_video_%d%s", videoID, ext)
				if _, err := os.Stat(videoPath); err == nil {
					msgResponse.VideoPath = &videoPath
					break
				}
			}

			// Проверяем превью видео (извлеченный первый кадр)
			previewExtensions := []string{".jpg", ".jpeg", ".png", ".webp"}
			for _, ext := range previewExtensions {
				previewPath := fmt.Sprintf("/tmp/vi-tg_video_preview_%d%s", videoID, ext)
				if _, err := os.Stat(previewPath); err == nil {
					msgResponse.VideoPreviewPath = &previewPath
					break
				}
			}

			// Проверяем, является ли видео круглым
			msgResponse.VideoIsRound = &msg.VideoIsRound
		}

		// Add support for voice paths
		if msg.Type == "voice" {
			voiceID := int64(msg.ID)
			// Всегда устанавливаем VoiceID для голосовых сообщений
			msgResponse.VoiceID = &voiceID

			// Проверяем различные форматы голосовых файлов
			voiceExtensions := []string{".ogg", ".oga", ".mp3", ".wav", ".m4a", ".aac"}
			for _, ext := range voiceExtensions {
				voicePath := fmt.Sprintf("/tmp/vi-tg_voice_%d%s", voiceID, ext)
				if _, err := os.Stat(voicePath); err == nil {
					msgResponse.VoicePath = &voicePath
					break
				}
			}

			// Устанавливаем длительность голосового сообщения
			if msg.VoiceDuration > 0 {
				msgResponse.VoiceDuration = &msg.VoiceDuration
			}
		}

		// Add support for audio paths
		if msg.Type == "audio" {
			audioID := int64(msg.ID)
			// Всегда устанавливаем AudioID для аудио сообщений
			msgResponse.AudioID = &audioID

			// Проверяем различные форматы аудио файлов
			audioExtensions := []string{".mp3", ".m4a", ".aac", ".wav", ".ogg", ".flac"}
			for _, ext := range audioExtensions {
				audioPath := fmt.Sprintf("/tmp/vi-tg_audio_%d%s", audioID, ext)
				if _, err := os.Stat(audioPath); err == nil {
					msgResponse.AudioPath = &audioPath
					break
				}
			}

			// Устанавливаем длительность аудио файла
			if msg.AudioDuration > 0 {
				msgResponse.AudioDuration = &msg.AudioDuration
			}

			// Устанавливаем метаданные аудио файла
			if msg.AudioTitle != "" {
				msgResponse.AudioTitle = &msg.AudioTitle
			}

			if msg.AudioArtist != "" {
				msgResponse.AudioArtist = &msg.AudioArtist
			}
		}

		messageResponses = append(messageResponses, msgResponse)
	}

	response := MessagesResponse{
		Messages: messageResponses,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (s *APIServer) sendMessage(w http.ResponseWriter, r *http.Request) {
	if !s.mtproto.IsAuthorized() {
		s.sendError(w, "Не авторизован", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	chatIDStr := vars["chat_id"]
	chatID, err := strconv.ParseInt(chatIDStr, 10, 64)
	if err != nil {
		s.sendError(w, "Неверный ID чата", http.StatusBadRequest)
		return
	}

	var req SendMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.sendError(w, "Неверный формат запроса", http.StatusBadRequest)
		return
	}

	if req.Text == "" {
		s.sendError(w, "Текст сообщения не может быть пустым", http.StatusBadRequest)
		return
	}

	err = s.mtproto.SendMessage(s.ctx, chatID, req.Text)
	if err != nil {
		s.sendError(w, fmt.Sprintf("Ошибка отправки сообщения: %v", err), http.StatusInternalServerError)
		return
	}

	response := SendMessageResponse{
		Success: true,
		Message: "Сообщение отправлено",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (s *APIServer) getSticker(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	stickerIDStr := vars["sticker_id"]
	stickerID, err := strconv.ParseInt(stickerIDStr, 10, 64)
	if err != nil {
		s.sendError(w, "Неверный ID стикера", http.StatusBadRequest)
		return
	}

	// Ищем файл стикера
	stickerPath := fmt.Sprintf("/tmp/vi-tg_sticker_%d.webp", stickerID)
	if _, err := os.Stat(stickerPath); err != nil {
		// Пробуем PNG версию
		stickerPath = fmt.Sprintf("/tmp/vi-tg_sticker_%d.png", stickerID)
		if _, err := os.Stat(stickerPath); err != nil {
			s.sendError(w, "Стикер не найден", http.StatusNotFound)
			return
		}
	}

	// Определяем MIME тип
	contentType := "image/webp"
	if strings.HasSuffix(stickerPath, ".png") {
		contentType = "image/png"
	}

	// Читаем файл
	data, err := os.ReadFile(stickerPath)
	if err != nil {
		s.sendError(w, "Ошибка чтения файла стикера", http.StatusInternalServerError)
		return
	}

	// Устанавливаем заголовки
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Header().Set("Cache-Control", "public, max-age=3600")

	// Отправляем данные
	w.Write(data)
}

func (s *APIServer) getImage(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	imageIDStr := vars["image_id"]
	imageID, err := strconv.ParseInt(imageIDStr, 10, 64)
	if err != nil {
		s.sendError(w, "Неверный ID изображения", http.StatusBadRequest)
		return
	}

	// Ищем файл изображения с различными расширениями
	possibleExtensions := []string{".png", ".jpg", ".jpeg", ".webp", ".gif"}
	var imagePath string
	var contentType string

	for _, ext := range possibleExtensions {
		testPath := fmt.Sprintf("/tmp/vi-tg_image_%d%s", imageID, ext)
		if _, err := os.Stat(testPath); err == nil {
			imagePath = testPath
			// Определяем MIME тип на основе расширения
			switch ext {
			case ".png":
				contentType = "image/png"
			case ".jpg", ".jpeg":
				contentType = "image/jpeg"
			case ".webp":
				contentType = "image/webp"
			case ".gif":
				contentType = "image/gif"
			default:
				contentType = "image/png"
			}
			break
		}
	}

	if imagePath == "" {
		s.sendError(w, "Изображение не найдено", http.StatusNotFound)
		return
	}

	// Читаем файл
	data, err := os.ReadFile(imagePath)
	if err != nil {
		s.sendError(w, "Ошибка чтения файла изображения", http.StatusInternalServerError)
		return
	}

	// Устанавливаем заголовки
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Header().Set("Cache-Control", "public, max-age=3600")

	// Отправляем данные
	w.Write(data)
}

func (s *APIServer) getVideo(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	videoIDStr := vars["video_id"]
	videoID, err := strconv.ParseInt(videoIDStr, 10, 64)
	if err != nil {
		s.sendError(w, "Неверный ID видео", http.StatusBadRequest)
		return
	}

	// Ищем файл видео с различными расширениями
	videoExtensions := []string{".mp4", ".avi", ".mkv", ".mov", ".webm", ".flv"}
	var videoPath string
	var contentType string

	for _, ext := range videoExtensions {
		testPath := fmt.Sprintf("/tmp/vi-tg_video_%d%s", videoID, ext)
		if _, err := os.Stat(testPath); err == nil {
			videoPath = testPath
			// Определяем MIME тип на основе расширения
			switch ext {
			case ".mp4":
				contentType = "video/mp4"
			case ".avi":
				contentType = "video/x-msvideo"
			case ".mkv":
				contentType = "video/x-matroska"
			case ".mov":
				contentType = "video/quicktime"
			case ".webm":
				contentType = "video/webm"
			case ".flv":
				contentType = "video/x-flv"
			default:
				contentType = "video/mp4"
			}
			break
		}
	}

	if videoPath == "" {
		s.sendError(w, "Видео не найдено", http.StatusNotFound)
		return
	}

	// Читаем файл
	data, err := os.ReadFile(videoPath)
	if err != nil {
		s.sendError(w, "Ошибка чтения файла видео", http.StatusInternalServerError)
		return
	}

	// Устанавливаем заголовки
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Header().Set("Cache-Control", "public, max-age=3600")

	// Отправляем данные
	w.Write(data)
}

func (s *APIServer) getVoice(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	voiceIDStr := vars["voice_id"]
	voiceID, err := strconv.ParseInt(voiceIDStr, 10, 64)
	if err != nil {
		s.sendError(w, "Неверный ID голосового сообщения", http.StatusBadRequest)
		return
	}

	// Ищем файл голосового сообщения с различными расширениями
	voiceExtensions := []string{".ogg", ".oga", ".mp3", ".wav", ".m4a", ".aac"}
	var voicePath string
	var contentType string

	for _, ext := range voiceExtensions {
		testPath := fmt.Sprintf("/tmp/vi-tg_voice_%d%s", voiceID, ext)
		if _, err := os.Stat(testPath); err == nil {
			voicePath = testPath
			// Определяем MIME тип на основе расширения
			switch ext {
			case ".ogg", ".oga":
				contentType = "audio/ogg"
			case ".mp3":
				contentType = "audio/mpeg"
			case ".wav":
				contentType = "audio/wav"
			case ".m4a":
				contentType = "audio/mp4"
			case ".aac":
				contentType = "audio/aac"
			default:
				contentType = "audio/ogg"
			}
			break
		}
	}

	if voicePath == "" {
		s.sendError(w, "Голосовое сообщение не найдено", http.StatusNotFound)
		return
	}

	// Читаем файл
	data, err := os.ReadFile(voicePath)
	if err != nil {
		s.sendError(w, "Ошибка чтения файла голосового сообщения", http.StatusInternalServerError)
		return
	}

	// Устанавливаем заголовки
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Header().Set("Cache-Control", "public, max-age=3600")

	// Отправляем данные
	w.Write(data)
}

func (s *APIServer) getAudio(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	audioIDStr := vars["audio_id"]
	audioID, err := strconv.ParseInt(audioIDStr, 10, 64)
	if err != nil {
		s.sendError(w, "Неверный ID аудио сообщения", http.StatusBadRequest)
		return
	}

	// Ищем файл аудио сообщения с различными расширениями
	audioExtensions := []string{".mp3", ".m4a", ".aac", ".wav", ".ogg", ".flac"}
	var audioPath string
	var contentType string

	for _, ext := range audioExtensions {
		testPath := fmt.Sprintf("/tmp/vi-tg_audio_%d%s", audioID, ext)
		if _, err := os.Stat(testPath); err == nil {
			audioPath = testPath
			// Определяем MIME тип на основе расширения
			switch ext {
			case ".mp3":
				contentType = "audio/mpeg"
			case ".m4a":
				contentType = "audio/mp4"
			case ".aac":
				contentType = "audio/aac"
			case ".wav":
				contentType = "audio/wav"
			case ".ogg":
				contentType = "audio/ogg"
			case ".flac":
				contentType = "audio/flac"
			default:
				contentType = "audio/mpeg"
			}
			break
		}
	}

	if audioPath == "" {
		s.sendError(w, "Аудио сообщение не найдено", http.StatusNotFound)
		return
	}

	// Читаем файл
	data, err := os.ReadFile(audioPath)
	if err != nil {
		s.sendError(w, "Ошибка чтения файла аудио сообщения", http.StatusInternalServerError)
		return
	}

	// Устанавливаем заголовки
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Header().Set("Cache-Control", "public, max-age=3600")

	// Отправляем данные
	w.Write(data)
}

func (s *APIServer) sendError(w http.ResponseWriter, message string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)

	response := APIResponse{
		Error: message,
		Code:  code,
	}

	json.NewEncoder(w).Encode(response)
}

func main() {
	server := NewAPIServer()
	if err := server.Start(); err != nil {
		log.Fatal("Ошибка запуска сервера:", err)
	}
}
