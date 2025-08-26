package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	"github.com/rs/cors"

	"vi-tg/auth"
	"vi-tg/config"
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
	// Location support fields
	LocationID      *int64   `json:"location_id"`
	LocationLat     *float64 `json:"location_lat"`
	LocationLng     *float64 `json:"location_lng"`
	LocationTitle   *string  `json:"location_title"`
	LocationAddress *string  `json:"location_address"`
	LocationMapPath *string  `json:"location_map_path"`
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

	// Location endpoints
	api.HandleFunc("/locations/{location_id}", s.getLocation).Methods("GET")
	api.HandleFunc("/locations/{location_id}/map", s.getLocationMap).Methods("GET")

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

		// Add support for location messages
		if msg.Type == "location" {
			locationID := int64(msg.ID)
			// Всегда устанавливаем LocationID для сообщений с локацией
			msgResponse.LocationID = &locationID

			// Устанавливаем координаты если они есть
			if msg.LocationLat != 0 {
				msgResponse.LocationLat = &msg.LocationLat
			}
			if msg.LocationLng != 0 {
				msgResponse.LocationLng = &msg.LocationLng
			}

			// Устанавливаем название локации если есть
			if msg.LocationTitle != "" {
				msgResponse.LocationTitle = &msg.LocationTitle
			}

			// Устанавливаем адрес локации если есть
			if msg.LocationAddress != "" {
				msgResponse.LocationAddress = &msg.LocationAddress
			}

			// Устанавливаем путь к карте как API endpoint (не локальный файл)
			mapPath := fmt.Sprintf("/api/locations/%d/map", locationID)
			msgResponse.LocationMapPath = &mapPath
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

func (s *APIServer) getLocation(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	locationIDStr := vars["location_id"]
	locationID, err := strconv.ParseInt(locationIDStr, 10, 64)
	if err != nil {
		s.sendError(w, "Неверный ID локации", http.StatusBadRequest)
		return
	}

	// For now, return mock location data
	// In a real implementation, this would fetch from a database or cache
	location := map[string]interface{}{
		"id":        locationID,
		"latitude":  55.7558,
		"longitude": 37.6173,
		"title":     "Red Square",
		"address":   "Red Square, Moscow, Russia",
		"map_path":  fmt.Sprintf("/tmp/vi-tg_location_map_%d.png", locationID),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(location)
}

func (s *APIServer) getLocationMap(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	locationIDStr := vars["location_id"]
	locationID, err := strconv.ParseInt(locationIDStr, 10, 64)
	if err != nil {
		s.sendError(w, "Неверный ID локации", http.StatusBadRequest)
		return
	}

	// Get coordinates from query parameters
	latStr := r.URL.Query().Get("lat")
	lngStr := r.URL.Query().Get("lng")

	var lat, lng float64
	if latStr != "" && lngStr != "" {
		if parsedLat, err := strconv.ParseFloat(latStr, 64); err == nil {
			lat = parsedLat
		}
		if parsedLng, err := strconv.ParseFloat(lngStr, 64); err == nil {
			lng = parsedLng
		}
	}

	// If no coordinates provided, use default (Red Square, Moscow)
	if lat == 0 && lng == 0 {
		lat = 55.7558
		lng = 37.6173
	}

	// Ищем файл карты
	mapPath := fmt.Sprintf("/tmp/vi-tg_location_map_%d.png", locationID)
	if _, err := os.Stat(mapPath); err != nil {
		// Если файл карты не существует, создаем карту с реальными координатами
		if err := s.generateLocationMap(locationID, lat, lng, mapPath); err != nil {
			log.Printf("Error generating map: %v", err)
			s.sendError(w, "Ошибка генерации карты", http.StatusInternalServerError)
			return
		}
	}

	// Читаем файл карты
	data, err := os.ReadFile(mapPath)
	if err != nil {
		s.sendError(w, "Ошибка чтения файла карты", http.StatusInternalServerError)
		return
	}

	// Устанавливаем заголовки
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Header().Set("Cache-Control", "public, max-age=3600")

	// Отправляем данные
	w.Write(data)
}

func (s *APIServer) generateLocationMap(locationID int64, lat, lng float64, mapPath string) error {
	// Use Yandex Maps API to generate a real map image
	// Convert coordinates to tile numbers and fetch the map tile

	// Yandex Maps API configuration
	apiKey := "2a565807-86b7-4e0a-8170-edc9f6bbc99e"
	zoom := 15 // Good zoom level for location details

	// Convert lat/lng to tile coordinates
	x, y := s.latLngToTileNumbers(lat, lng, zoom)

	// Fetch map tile from Yandex Maps API
	tileURL := fmt.Sprintf("https://tiles.api-maps.yandex.ru/v1/tiles/?&x=%d&y=%d&z=%d&lang=ru_RU&l=map&apikey=%s",
		x, y, zoom, apiKey)

	resp, err := http.Get(tileURL)
	if err != nil {
		log.Printf("Error fetching map tile: %v", err)
		return s.generateFallbackMap(lat, lng, mapPath)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("Yandex Maps API returned status: %d", resp.StatusCode)
		return s.generateFallbackMap(lat, lng, mapPath)
	}

	// Read the tile image
	tileData, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Error reading tile data: %v", err)
		return s.generateFallbackMap(lat, lng, mapPath)
	}

	// Decode the tile image
	tileImg, _, err := image.Decode(bytes.NewReader(tileData))
	if err != nil {
		log.Printf("Error decoding tile image: %v", err)
		return s.generateFallbackMap(lat, lng, mapPath)
	}

	// Create a larger canvas for the final map
	finalWidth := 400
	finalHeight := 300
	finalImg := image.NewRGBA(image.Rect(0, 0, finalWidth, finalHeight))

	// Calculate position for the tile on the canvas (center it)
	tileBounds := tileImg.Bounds()
	tileWidth := tileBounds.Dx()
	tileHeight := tileBounds.Dy()

	tileX := (finalWidth - tileWidth) / 2
	tileY := (finalHeight - tileHeight) / 2

	// Draw the tile on the canvas
	for y := 0; y < tileHeight; y++ {
		for x := 0; x < tileWidth; x++ {
			srcColor := tileImg.At(tileBounds.Min.X+x, tileBounds.Min.Y+y)
			finalImg.Set(tileX+x, tileY+y, srcColor)
		}
	}

	// Add a marker at the exact location
	s.addLocationMarker(finalImg, lat, lng, zoom, x, y, tileX, tileY)

	// Save the final map image
	file, err := os.Create(mapPath)
	if err != nil {
		return err
	}
	defer file.Close()

	return png.Encode(file, finalImg)
}

// latLngToTileNumbers converts latitude/longitude to tile X,Y coordinates
func (s *APIServer) latLngToTileNumbers(lat, lng float64, zoom int) (int, int) {
	// Use proper WGS84 Mercator projection (same as JavaScript implementation)
	e := 0.0818191908426 // WGS84 eccentricity

	// Convert to radians
	beta := lat * math.Pi / 180.0

	// Calculate phi (accounts for ellipsoidal Earth)
	phi := (1 - e*math.Sin(beta)) / (1 + e*math.Sin(beta))

	// Calculate theta
	theta := math.Tan(math.Pi/4+beta/2) * math.Pow(phi, e/2)

	// Calculate pixel coordinates at zoom level
	rho := math.Pow(2, float64(zoom)+8) / 2

	xPixel := rho * (1 + lng/180)
	yPixel := rho * (1 - math.Log(theta)/math.Pi)

	// Convert to tile numbers
	x := int(math.Floor(xPixel / 256))
	y := int(math.Floor(yPixel / 256))

	return x, y
}

// latLngToPixel converts latitude/longitude to pixel coordinates at given zoom level
func (s *APIServer) latLngToPixel(lat, lng float64, zoom int) (float64, float64) {
	// Use the same proper WGS84 Mercator projection as latLngToTileNumbers
	e := 0.0818191908426 // WGS84 eccentricity

	// Convert to radians
	beta := lat * math.Pi / 180.0

	// Calculate phi (accounts for ellipsoidal Earth)
	phi := (1 - e*math.Sin(beta)) / (1 + e*math.Sin(beta))

	// Calculate theta
	theta := math.Tan(math.Pi/4+beta/2) * math.Pow(phi, e/2)

	// Calculate pixel coordinates at zoom level (consistent with tile calculation)
	rho := math.Pow(2, float64(zoom)+8) / 2

	x := rho * (1 + lng/180)
	y := rho * (1 - math.Log(theta)/math.Pi)

	return x, y
}

// addLocationMarker adds a red marker at the exact location on the map
func (s *APIServer) addLocationMarker(img *image.RGBA, lat, lng float64, zoom, tileX, tileY, offsetX, offsetY int) {
	// Calculate pixel position within the tile
	tilePixelX, tilePixelY := s.latLngToPixel(lat, lng, zoom)

	// Calculate pixel position within this specific tile
	pixelX := int(tilePixelX) - (tileX * 256)
	pixelY := int(tilePixelY) - (tileY * 256)

	// Position on the final image (centered tile + pixel offset)
	markerX := offsetX + pixelX
	markerY := offsetY + pixelY

	// Draw a red circle marker
	red := color.RGBA{255, 0, 0, 255}
	radius := 10

	for dy := -radius; dy <= radius; dy++ {
		for dx := -radius; dx <= radius; dx++ {
			if dx*dx+dy*dy <= radius*radius {
				imgX := markerX + dx
				imgY := markerY + dy

				// Check bounds
				if imgX >= 0 && imgX < img.Bounds().Dx() && imgY >= 0 && imgY < img.Bounds().Dy() {
					img.Set(imgX, imgY, red)
				}
			}
		}
	}

	// Add a small black border around the marker
	black := color.RGBA{0, 0, 0, 255}
	borderRadius := radius + 2
	for dy := -borderRadius; dy <= borderRadius; dy++ {
		for dx := -borderRadius; dx <= borderRadius; dx++ {
			if dx*dx+dy*dy <= borderRadius*borderRadius && dx*dx+dy*dy > radius*radius {
				imgX := markerX + dx
				imgY := markerY + dy

				// Check bounds
				if imgX >= 0 && imgX < img.Bounds().Dx() && imgY >= 0 && imgY < img.Bounds().Dy() {
					img.Set(imgX, imgY, black)
				}
			}
		}
	}
}

// generateFallbackMap creates a simple placeholder map when API fails
func (s *APIServer) generateFallbackMap(lat, lng float64, mapPath string) error {
	// Create a simple colored rectangle as a map placeholder
	img := image.NewRGBA(image.Rect(0, 0, 400, 300))
	blue := color.RGBA{100, 150, 200, 255}

	// Fill with blue color
	for y := 0; y < img.Bounds().Dy(); y++ {
		for x := 0; x < img.Bounds().Dx(); x++ {
			img.Set(x, y, blue)
		}
	}

	// Add a simple marker (red dot)
	centerX, centerY := 200, 150
	markerColor := color.RGBA{255, 0, 0, 255}
	for dy := -5; dy <= 5; dy++ {
		for dx := -5; dx <= 5; dx++ {
			if dx*dx+dy*dy <= 25 { // Circle
				img.Set(centerX+dx, centerY+dy, markerColor)
			}
		}
	}

	// Add coordinates text (if we had a font system)
	// For now, just save the image

	file, err := os.Create(mapPath)
	if err != nil {
		return err
	}
	defer file.Close()

	return png.Encode(file, img)
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
