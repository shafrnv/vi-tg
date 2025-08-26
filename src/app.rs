use anyhow::Result;
use std::time::{Duration, Instant};
use std::collections::HashMap;

use crate::api::ApiClient;
use crate::{AuthStatus, Chat, Message};



#[derive(Debug, Clone)]
pub struct AudioPlayer {
    pub is_playing: bool,
    pub current_position: Duration,
    pub total_duration: Option<Duration>,
    pub current_message_id: Option<i32>,
    pub process_id: Option<u32>,
    pub current_file_path: Option<String>, // Store current audio file path for restart
}

impl Default for AudioPlayer {
    fn default() -> Self {
        Self {
            is_playing: false,
            current_position: Duration::ZERO,
            total_duration: None,
            current_message_id: None,
            process_id: None,
            current_file_path: None,
        }
    }
}

impl AudioPlayer {
    pub fn new() -> Self {
        Self::default()
    }

    pub fn format_time(&self, duration: Duration) -> String {
        let total_seconds = duration.as_secs();
        let minutes = total_seconds / 60;
        let seconds = total_seconds % 60;
        format!("{:01}:{:02}", minutes, seconds)
    }

    pub fn get_current_time_display(&self) -> String {
        let current = self.format_time(self.current_position);
        if let Some(total) = self.total_duration {
            let total_str = self.format_time(total);
            format!("{} / {}", current, total_str)
        } else {
            current
        }
    }

    pub fn is_current_message(&self, message_id: i32) -> bool {
        self.current_message_id == Some(message_id)
    }

    pub fn stop(&mut self) {
        if let Some(pid) = self.process_id {
            // Try to kill the process
            let _ = std::process::Command::new("kill")
                .arg("-TERM")
                .arg(pid.to_string())
                .status();
        }
        self.is_playing = false;
        self.current_position = Duration::ZERO;
        self.current_message_id = None;
        self.process_id = None;
    }

    pub fn stop_playback(&mut self) {
        self.stop();
    }

    pub fn stop_playback_with_timestamp(&mut self, app: &mut crate::App) {
        self.stop();
        app.audio_start_time = None;
    }

    pub fn seek(&mut self, seconds: i64) -> bool {
        // Обновляем позицию в памяти для UI
        let old_position = self.current_position;
        if seconds > 0 {
            self.current_position = self.current_position.saturating_add(Duration::from_secs(seconds as u64));
        } else {
            self.current_position = self.current_position.saturating_sub(Duration::from_secs((-seconds) as u64));
        }

        if let Some(total) = self.total_duration {
            if self.current_position > total {
                self.current_position = total;
            }
        }

        // Логируем изменение позиции (только для отладки)
        log::debug!("Seek: {}s, position changed from {} to {}",
            seconds,
            format_duration(old_position),
            format_duration(self.current_position));

        // Пробуем разные методы управления плеером
        if let Some(pid) = self.process_id {
            // Проверяем, что процесс еще работает
            if let Ok(_) = std::process::Command::new("kill")
                .arg("-0")  // Проверяем, что процесс существует
                .arg(pid.to_string())
                .status() {

                log::debug!("Process {} is running, attempting to send seek command", pid);

                // Метод 1: Проверяем сокет и отправляем команду
                let socket_path = "/tmp/mpv-socket";
                if std::path::Path::new(socket_path).exists() {
                    log::debug!("Socket {} exists, sending seek command", socket_path);

                    // Пробуем разные способы отправки команды
                    let seek_command = format!("seek {}\n", seconds);

                    // Способ 1: через socat (если доступен)
                    let socat_result = std::process::Command::new("bash")
                        .arg("-c")
                        .arg(format!("echo '{}' | socat - UNIX-CONNECT:{} 2>/dev/null", seek_command.trim(), socket_path))
                        .stderr(std::process::Stdio::null())
                        .status();

                    match socat_result {
                        Ok(status) if status.success() => {
                            log::debug!("Successfully sent seek command via socat");
                            return true;
                        }
                        _ => log::debug!("Failed to send via socat")
                    }

                    // Способ 2: через nc (netcat, если доступен)
                    let nc_result = std::process::Command::new("bash")
                        .arg("-c")
                        .arg(format!("echo '{}' | nc -U {} 2>/dev/null", seek_command.trim(), socket_path))
                        .stderr(std::process::Stdio::null())
                        .status();

                    match nc_result {
                        Ok(status) if status.success() => {
                            log::debug!("Successfully sent seek command via nc");
                            return true;
                        }
                        _ => log::debug!("Failed to send via nc")
                    }

                    // Способ 3: через простой echo с перенаправлением
                    let echo_result = std::process::Command::new("bash")
                        .arg("-c")
                        .arg(format!("echo '{}' > {} 2>/dev/null", seek_command.trim(), socket_path))
                        .stderr(std::process::Stdio::null())
                        .status();

                    match echo_result {
                        Ok(status) if status.success() => {
                            log::debug!("Successfully sent seek command via echo");
                            return true;
                        }
                        _ => log::debug!("Failed to send via echo")
                    }

                    // Способ 4: Используем printf для более надежной отправки
                    let printf_result = std::process::Command::new("bash")
                        .arg("-c")
                        .arg(format!("printf '%s\\n' '{}' > {} 2>/dev/null", seek_command.trim(), socket_path))
                        .stderr(std::process::Stdio::null())
                        .status();

                    match printf_result {
                        Ok(status) if status.success() => {
                            log::debug!("Successfully sent seek command via printf");
                            return true;
                        }
                        _ => log::debug!("Failed to send via printf")
                    }

                    // Способ 5: Используем dd для бинарной записи
                    let dd_result = std::process::Command::new("bash")
                        .arg("-c")
                        .arg(format!("echo '{}' | dd of={} 2>/dev/null", seek_command.trim(), socket_path))
                        .stderr(std::process::Stdio::null())
                        .status();

                    match dd_result {
                        Ok(status) if status.success() => {
                            log::debug!("Successfully sent seek command via dd");
                            return true;
                        }
                        _ => log::debug!("Failed to send via dd")
                    }

                } else {
                    log::warn!("Socket {} does not exist", socket_path);
                }

                // Метод 2: Сигналы для управления (если IPC не работает)
                // Для mpv можно использовать SIGUSR1 для паузы/воспроизведения
                if seconds == 0 {  // Специальный случай для паузы/воспроизведения
                    let _ = std::process::Command::new("kill")
                        .arg("-USR1")
                        .arg(pid.to_string())
                        .status();
                    log::info!("Sent SIGUSR1 to process {} for pause/play", pid);
                }

                log::debug!("All seek methods attempted for process {}", pid);
            } else {
                log::warn!("Audio process {} is not running", pid);
            }
        } else {
            log::warn!("No process ID available for seek operation");
        }

        // Если все методы провалились, возвращаем false для активации restart
        log::debug!("IPC communication failed, restart needed");
        false
    }


}

fn format_duration(duration: Duration) -> String {
    let total_seconds = duration.as_secs();
    let minutes = total_seconds / 60;
    let seconds = total_seconds % 60;
    format!("{:02}:{:02}", minutes, seconds)
}

// Standalone async function for downloading map images
async fn download_map_image_async(url: &str, message_id: i32) -> Result<String> {
    log::info!("Скачиваем карту с URL: {} в фоновом режиме", url);

    // Create a temporary file path for the downloaded image
    let temp_path = format!("/tmp/vi-tg_location_preview_{}.png", message_id);

    // Check if we already have this image downloaded
    if std::path::Path::new(&temp_path).exists() {
        log::info!("Карта уже скачана, используем существующий файл: {}", temp_path);
        return Ok(temp_path);
    }

    // Create HTTP client and download the image
    let client = reqwest::Client::new();
    let response = client.get(url).send().await
        .map_err(|e| anyhow::anyhow!("Ошибка HTTP запроса: {}", e))?;

    if !response.status().is_success() {
        return Err(anyhow::anyhow!("HTTP ошибка: {} для URL: {}", response.status(), url));
    }

    // Read the image data
    let image_data = response.bytes().await
        .map_err(|e| anyhow::anyhow!("Ошибка чтения данных изображения: {}", e))?;

    // Write to temporary file
    tokio::fs::write(&temp_path, &image_data).await
        .map_err(|e| anyhow::anyhow!("Ошибка сохранения файла: {}", e))?;

    log::info!("Карта успешно скачана и сохранена в фоновом режиме: {}", temp_path);
    Ok(temp_path)
}

#[derive(Debug, Clone, PartialEq)]
pub enum AppState {
    Loading,
    PhoneInput,
    CodeInput,
    Main,
    MessageInput,
    Error,
    ImagePreview,
    VideoPreview,
}

pub struct App {
    pub api_client: ApiClient,
    pub state: AppState,

    // Состояние авторизации
    pub auth_status: Option<AuthStatus>,
    pub phone_input: String,
    pub code_input: String,

    // Основное состояние
    pub chats: Vec<Chat>,
    pub selected_chat_index: usize,
    pub selected_chat: Option<Chat>,
    pub messages: Vec<Message>,
    pub message_input: String,

    // Выбор и фокус сообщений
    pub focus_on_messages: bool,
    pub selected_message_index: usize,
    pub message_scroll_offset: usize,
    pub last_loaded_chat_id: Option<i64>,

    // Просмотр изображения
    pub preview_image_path: Option<String>,

    // Просмотр видео
    pub preview_video_path: Option<String>,

    // Состояние ошибки
    pub error_message: String,

    // Изображения
    pub image_paths: HashMap<i64, String>,

    // Стикеры
    pub sticker_paths: HashMap<i64, String>,

    // Таймеры для обновления
    pub last_update: Instant,
    pub last_auth_check: Instant,
    pub last_data_refresh: Instant,
    pub audio_start_time: Option<Instant>,

    // Реальная видимая емкость из UI
    pub visible_capacity: usize,

    // Аудио плеер состояние
    pub audio_player: AudioPlayer,
}

impl App {
    pub fn new(api_client: ApiClient) -> Self {
        Self {
            api_client,
            state: AppState::Loading,
            auth_status: None,
            phone_input: String::new(),
            code_input: String::new(),
            chats: Vec::new(),
            selected_chat_index: 0,
            selected_chat: None,
            messages: Vec::new(),
            message_input: String::new(),
            //
            focus_on_messages: false,
            selected_message_index: 0,
            message_scroll_offset: 0,
            last_loaded_chat_id: None,
            //
            preview_image_path: None,
            preview_video_path: None,
            error_message: String::new(),
            image_paths: HashMap::new(),
            sticker_paths: HashMap::new(),
            last_update: Instant::now(),
            last_auth_check: Instant::now(),
            last_data_refresh: Instant::now(),
            audio_start_time: None,
            visible_capacity: 15, // Значение по умолчанию
            audio_player: AudioPlayer::new(),
        }
    }

    pub async fn update(&mut self) -> Result<()> {
        let now = Instant::now();

        // В режиме предпросмотра картинки ничего не обновляем, чтобы не дергать layout
        if self.state == AppState::ImagePreview {
            return Ok(());
        }

        // Обновляем позицию аудио плеера
        self.update_audio_position(now);

        // Проверяем авторизацию каждые 2 секунды
        if now.duration_since(self.last_auth_check) > Duration::from_secs(2) {
            self.check_auth_status().await?;
            self.last_auth_check = now;
        }

    // ВРЕМЕННО ОТКЛЮЧЕНО: Обновляем данные каждые 5 секунд в основном состоянии
    /*
    if self.state == AppState::Main &&
       now.duration_since(self.last_data_refresh) > Duration::from_secs(5) {
        self.refresh_data().await?;
        self.last_data_refresh = now;
    }
    */

        self.last_update = now;
        Ok(())
    }

    pub fn update_audio_position(&mut self, now: Instant) {
        if self.audio_player.is_playing {
            if let Some(start_time) = self.audio_start_time {
                let elapsed = now.duration_since(start_time);
                self.audio_player.current_position = elapsed;

                // Проверяем, не закончилось ли воспроизведение
                if let Some(total) = self.audio_player.total_duration {
                    if elapsed >= total {
                        // Воспроизведение закончено
                        self.audio_player.stop();
                        self.audio_start_time = None;
                    }
                }
            } else {
                // Если время начала не установлено, но плеер отмечен как играющий, остановим его
                self.audio_player.stop();
                self.audio_start_time = None;
            }
        }
    }

    async fn check_auth_status(&mut self) -> Result<()> {
        match self.api_client.get_auth_status().await {
            Ok(auth_status) => {
                let previously_authorized = self.auth_status
                    .as_ref()
                    .map(|s| s.authorized)
                    .unwrap_or(false);

                self.auth_status = Some(auth_status.clone());

                // Определяем новое состояние на основе статуса авторизации
                match self.state {
                    AppState::Loading => {
                        if auth_status.authorized {
                            self.state = AppState::Main;
                            self.load_chats().await?;
                        } else if auth_status.needs_code {
                            self.state = AppState::CodeInput;
                        } else {
                            self.state = AppState::PhoneInput;
                        }
                    }
                    AppState::PhoneInput => {
                        if auth_status.authorized {
                            self.state = AppState::Main;
                            self.load_chats().await?;
                        } else if auth_status.needs_code {
                            self.state = AppState::CodeInput;
                        }
                    }
                    AppState::CodeInput => {
                        if auth_status.authorized {
                            self.state = AppState::Main;
                            self.load_chats().await?;
                        } else if !auth_status.needs_code {
                            self.state = AppState::PhoneInput;
                        }
                    }
                    AppState::Main => {
                        if !auth_status.authorized {
                            self.state = AppState::PhoneInput;
                            self.chats.clear();
                            self.messages.clear();
                            self.selected_chat = None;
                        }
                    }
                    _ => {}
                }

                // Если только что авторизовались, загружаем чаты
                if !previously_authorized && auth_status.authorized {
                    self.load_chats().await?;
                }
            }
            Err(e) => {
                log::error!("Ошибка проверки статуса авторизации: {}", e);
                // Не меняем состояние при ошибке сети
            }
        }

        Ok(())
    }

    pub async fn set_phone_number(&mut self) -> Result<()> {
        match self.api_client.set_phone_number(&self.phone_input).await {
            Ok(response) => {
                if response.success {
                    if response.needs_code {
                        self.state = AppState::CodeInput;
                        self.code_input.clear();
                    } else {
                        self.state = AppState::Main;
                        self.load_chats().await?;
                    }
                } else {
                    self.show_error(&response.message);
                }
            }
            Err(e) => {
                self.show_error(&format!("Ошибка установки номера: {}", e));
            }
        }

        Ok(())
    }

    pub async fn send_code(&mut self) -> Result<()> {
        match self.api_client.send_code(&self.code_input).await {
            Ok(response) => {
                if response.success {
                    if response.authorized {
                        self.state = AppState::Main;
                        self.load_chats().await?;
                    } else {
                        self.show_error("Код неверный, попробуйте еще раз");
                        self.code_input.clear();
                    }
                } else {
                    self.show_error(&response.message);
                }
            }
            Err(e) => {
                self.show_error(&format!("Ошибка отправки кода: {}", e));
            }
        }

        Ok(())
    }

    async fn load_chats(&mut self) -> Result<()> {
        match self.api_client.get_chats().await {
            Ok(chats) => {
                self.chats = chats;
                if self.selected_chat_index >= self.chats.len() {
                    self.selected_chat_index = 0;
                }

                // Автоматически выбираем первый чат если есть
                if !self.chats.is_empty() && self.selected_chat.is_none() {
                    self.selected_chat = Some(self.chats[0].clone());
                    self.load_messages().await?;
                }
            }
            Err(e) => {
                log::error!("Ошибка загрузки чатов: {}", e);
                self.show_error(&format!("Ошибка загрузки чатов: {}", e));
            }
        }

        Ok(())
    }

    async fn load_messages(&mut self) -> Result<()> {
        if let Some(chat) = &self.selected_chat {
            let current_chat_id = chat.id;
            let old_len = self.messages.len();
            let was_at_bottom = old_len > 0 && self.selected_message_index == old_len - 1;
            let old_selected_id = self.messages.get(self.selected_message_index).map(|m| m.id);

            // Загружаем большое количество сообщений для полноценного листания
            let message_limit = 200 as i32;
            match self.api_client.get_messages(chat.id, Some(message_limit)).await {
                Ok(messages) => {
                    // Инвертируем порядок: новые сообщения внизу, старые вверху
                    self.messages = messages.into_iter().rev().collect();

                    // Выбор сообщения после обновления
                    // Сохраняем позицию выделенного сообщения
                    if self.messages.is_empty() {
                        self.selected_message_index = 0;
                        self.message_scroll_offset = 0;
                    } else {
                        // Пытаемся сохранить предыдущую позицию
                        if let Some(old_id) = old_selected_id {
                            // Ищем сообщение с тем же id
                            if let Some(pos) = self.messages.iter().position(|m| m.id == old_id) {
                                self.selected_message_index = pos;
                            } else {
                                // Если не нашли, выбираем последнее сообщение
                                self.selected_message_index = self.messages.len() - 1;
                            }
                        } else {
                            // Если нет предыдущего id, выбираем последнее
                            self.selected_message_index = self.messages.len() - 1;
                        }
                        self.message_scroll_offset = 0; // Всегда начинаем с начала
                    }

                    // Загружаем пути к изображениям
                    self.load_image_paths().await?;

                    // Загружаем пути к стикерам
                    self.load_sticker_paths().await?;

                    // Отмечаем id чата, для которого загружены сообщения
                    self.last_loaded_chat_id = Some(current_chat_id);
                }
                Err(e) => {
                    log::error!("Ошибка загрузки сообщений: {}", e);
                    self.show_error(&format!("Ошибка загрузки сообщений: {}", e));
                }
            }
        }

        Ok(())
    }

    async fn load_image_paths(&mut self) -> Result<()> {
        for msg in &self.messages {
            if msg.r#type == "photo" {
                if let Some(image_path) = &msg.image_path {
                    if let Some(image_id) = msg.image_id {
                        // Проверяем, не загружен ли уже путь к изображению
                        if !self.image_paths.contains_key(&image_id) {
                            self.image_paths.insert(image_id, image_path.clone());
                        }
                    }
                }
            }
        }

        Ok(())
    }

    async fn load_sticker_paths(&mut self) -> Result<()> {
        for msg in &self.messages {
            if msg.r#type == "sticker" {
                if let Some(sticker_path) = &msg.sticker_path {
                    if let Some(sticker_id) = msg.sticker_id {
                        // Проверяем, не загружен ли уже путь к стикеру
                        if !self.sticker_paths.contains_key(&sticker_id) {
                            self.sticker_paths.insert(sticker_id, sticker_path.clone());
                        }
                    }
                }
            }
        }

        Ok(())
    }

    pub async fn refresh_data(&mut self) -> Result<()> {
        self.load_chats().await?;
        if self.selected_chat.is_some() {
            self.load_messages().await?;
        }
        Ok(())
    }

    pub fn move_chat_selection(&mut self, direction: i32) {
        if self.chats.is_empty() {
            return;
        }

        let new_index = if direction > 0 {
            (self.selected_chat_index + 1).min(self.chats.len() - 1)
        } else {
            self.selected_chat_index.saturating_sub(1)
        };

        if new_index != self.selected_chat_index {
            self.selected_chat_index = new_index;
        }
    }

    pub fn move_message_selection(&mut self, direction: i32, visible_capacity: usize) {
        if self.messages.is_empty() {
            return;
        }
        let last_index = self.messages.len() - 1;
        let new_index = if direction > 0 {
            (self.selected_message_index + 1).min(last_index)
        } else {
            self.selected_message_index.saturating_sub(1)
        };
        if new_index != self.selected_message_index {
            self.selected_message_index = new_index;
            // Обновляем прокрутку, чтобы выделение было видно
            if self.selected_message_index < self.message_scroll_offset {
                self.message_scroll_offset = self.selected_message_index;
            } else if self.selected_message_index >= self.message_scroll_offset + visible_capacity {
                let overshoot = self.selected_message_index - (self.message_scroll_offset + visible_capacity) + 1;
                self.message_scroll_offset += overshoot;
            }
        }
    }

    pub fn toggle_focus(&mut self) {
        self.focus_on_messages = !self.focus_on_messages;
    }

    pub fn focus_messages(&mut self) {
        self.focus_on_messages = true;
    }

    pub fn focus_chats(&mut self) {
        self.focus_on_messages = false;
    }

    pub fn open_selected_message(&mut self) {
        if self.messages.is_empty() {
            log::warn!("Нет сообщений для открытия");
            return;
        }
        if let Some(msg) = self.messages.get(self.selected_message_index) {
            log::info!("Открываем сообщение типа: {}, id: {}", msg.r#type, msg.id);

            if msg.r#type == "photo" {
                if let Some(path) = &msg.image_path {
                    log::info!("Открываем фото: {}", path);
                    self.preview_image_path = Some(path.clone());
                    self.state = AppState::ImagePreview;
                } else {
                    log::warn!("Фото сообщение без пути к файлу");
                }
            } else if msg.r#type == "video" {
                log::info!("Открываем видео. Путь к превью: {:?}, путь к видео: {:?}", msg.video_preview_path, msg.video_path);

                // For video preview, use the preview image (JPEG) and show overlay
                if let Some(preview_path) = &msg.video_preview_path {
                    self.preview_image_path = Some(preview_path.clone());
                    // Store video path for later playback when Enter is pressed in ImagePreview
                    self.preview_video_path = Some(msg.video_path.clone().unwrap_or_default());
                    self.state = AppState::ImagePreview;
                    log::info!("Установлен режим ImagePreview для видео с превью");
                } else if let Some(video_path) = &msg.video_path {
                    // Fallback to video file if no preview is available
                    self.preview_video_path = Some(video_path.clone());
                    self.state = AppState::VideoPreview;
                    log::info!("Установлен режим VideoPreview для видео без превью");
                } else {
                    log::warn!("Видео сообщение без путей к файлам");
                }
            } else if msg.r#type == "sticker" {
                if let Some(path) = &msg.sticker_path {
                    log::info!("Открываем стикер: {}", path);
                    self.preview_image_path = Some(path.clone());
                    self.state = AppState::ImagePreview;
                } else {
                    log::warn!("Стикер сообщение без пути к файлу");
                }
            } else if msg.r#type == "voice" {
                log::info!("Воспроизводим голосовое сообщение");
                log::info!("Проверяем voice_path: {:?}", msg.voice_path);
                if let Err(e) = self.play_voice() {
                    log::error!("Ошибка воспроизведения голосового сообщения: {}", e);
                    self.show_error(&format!("Ошибка воспроизведения голосового сообщения: {}", e));
                }
            } else if msg.r#type == "audio" {
                log::info!("Воспроизводим аудио сообщение");
                log::info!("Проверяем audio_path: {:?}", msg.audio_path);
                if let Err(e) = self.play_audio() {
                    log::error!("Ошибка воспроизведения аудио сообщения: {}", e);
                    self.show_error(&format!("Ошибка воспроизведения аудио сообщения: {}", e));
                }
            } else if msg.r#type == "location" {
                log::info!("Открываем сообщение с местоположением");
                log::info!("Координаты: lat={:?}, lng={:?}, title: {:?}", msg.location_lat, msg.location_lng, msg.location_title);
                log::info!("Адрес: {:?}", msg.location_address);
                log::info!("Карта: {:?}", msg.location_map_path);

                // Show location preview - this will show detailed location info
                if let Err(e) = self.open_location() {
                    log::error!("Ошибка открытия местоположения: {}", e);
                    self.show_error(&format!("Ошибка открытия местоположения: {}", e));
                }
            } else {
                log::info!("Неизвестный тип сообщения: {}", msg.r#type);
            }
        } else {
            log::error!("Сообщение не найдено по индексу {}", self.selected_message_index);
        }
    }

    pub fn close_image_preview(&mut self) {
        self.preview_image_path = None;
        self.preview_video_path = None; // Clear video path too
        self.state = AppState::Main;
    }

    pub fn close_video_preview(&mut self) {
        self.preview_video_path = None;
        self.state = AppState::Main;
    }

    pub fn play_video(&mut self) -> Result<()> {
        // Get the video file path - prefer the stored preview_video_path if available
        // This is important for fullscreen preview mode where we have a stored path
        let video_path = if let Some(preview_path) = &self.preview_video_path {
            if !preview_path.is_empty() {
                log::info!("Используем сохраненный путь к видео из превью: {}", preview_path);
                preview_path.clone()
            } else {
                // Fallback to getting path from current message
                if let Some(msg) = self.messages.get(self.selected_message_index) {
                    if let Some(msg_video_path) = &msg.video_path {
                        log::info!("Используем путь к видео из текущего сообщения: {}", msg_video_path);
                        msg_video_path.clone()
                    } else {
                        log::error!("Путь к видео файлу не найден в сообщении");
                        return Err(anyhow::anyhow!("Путь к видео файлу не найден"));
                    }
                } else {
                    log::error!("Сообщение не найдено по индексу {}", self.selected_message_index);
                    return Err(anyhow::anyhow!("Сообщение не найдено"));
                }
            }
        } else {
            // No stored preview path, get from current message
            if let Some(msg) = self.messages.get(self.selected_message_index) {
                if let Some(msg_video_path) = &msg.video_path {
                    log::info!("Пытаемся воспроизвести видео из сообщения: {}", msg_video_path);
                    msg_video_path.clone()
                } else {
                    log::error!("Путь к видео файлу не найден в сообщении");
                    return Err(anyhow::anyhow!("Путь к видео файлу не найден"));
                }
            } else {
                log::error!("Сообщение не найдено по индексу {}", self.selected_message_index);
                return Err(anyhow::anyhow!("Сообщение не найдено"));
            }
        };

        log::info!("Пытаемся воспроизвести видео: {}", video_path);

        // Проверяем, существует ли файл
        if !std::path::Path::new(&video_path).exists() {
            return Err(anyhow::anyhow!("Файл видео не существует: {}", video_path));
        }

        // Пробуем получить ID окна терминала для overlay
        let window_id = self.get_terminal_window_id();
        log::info!("ID окна терминала: {:?}", window_id);

        // Создаем строку для window ID заранее, чтобы избежать временных значений
        let window_id_str = window_id.unwrap_or(0).to_string();

        // Пробуем разные стратегии запуска mpv для tiling WM
        let strategies = vec![
            // Стратегия 1: Floating window for tiling WM (dwm)
            (Some("floating"), vec![
                "--no-terminal",
                "--input-ipc-server=/tmp/mpv-socket",
                "--force-window=yes",
                "--keep-open=no",
                "--geometry=800x600+100+100"  // Set specific position
            ]),
            // Стратегия 2: Terminal-based with audio only (for headless)
            (Some("audio-only"), vec![
                "--no-terminal",
                "--input-ipc-server=/tmp/mpv-socket",
                "--force-window=no",
                "--audio-device=alsa",
                "--keep-open=no",
                "--no-video"  // Audio only
            ]),
            // Стратегия 3: Minimal config for compatibility
            (Some("minimal"), vec![
                "--no-config",
                "--no-terminal",
                "--force-window=yes",
                "--keep-open=no",
                "--geometry=50%x50%"  // Center, half size
            ])
        ];

        for (strategy_name, args) in strategies {
            if strategy_name.is_none() && window_id.is_none() {
                continue; // Skip overlay strategy if no window ID
            }

            log::info!("Пробуем стратегию: {:?}", strategy_name);

            let mut cmd = std::process::Command::new("mpv");
            for arg in &args {
                if arg != &"0" {  // Skip the dummy window ID
                    cmd.arg(arg);
                }
            }

            // Set environment for better compatibility
            cmd.env("DISPLAY", ":0")
               .env("XDG_SESSION_TYPE", "tty");

            cmd.arg(&video_path);
            cmd.stdout(std::process::Stdio::null())
               .stderr(std::process::Stdio::null());

            log::info!("Запускаем команду: {:?}", cmd);
            let result = cmd.spawn();

            match result {
                Ok(child) => {
                    log::info!("mpv успешно запущен стратегией {:?}, PID: {}", strategy_name, child.id());
                    // Wait a moment and check if process is still running
                    std::thread::sleep(std::time::Duration::from_millis(500));

                    // Check if process is still alive
                    let pid = child.id();
                    let is_alive = std::process::Command::new("kill")
                        .arg("-0")  // Signal 0 just checks if process exists
                        .arg(pid.to_string())
                        .status()
                        .is_ok();

                    if is_alive {
                        log::info!("mpv процесс работает нормально");
                    } else {
                        log::info!("mpv процесс завершен быстро, возможно проблема с окном");
                    }
                    return Ok(());
                }
                Err(e) => {
                    log::warn!("Стратегия {:?} не сработала: {}", strategy_name, e);
                    continue;
                }
            }
        }

        // If all strategies failed, try alternative video players
        log::info!("Все стратегии mpv провалились, пробуем альтернативные плееры");

        let alternative_players = vec![
            ("vlc", vec!["--no-video-title", "--play-and-exit"]),
            ("ffplay", vec!["-autoexit", "-fs"]), // fullscreen
            ("mplayer", vec!["-fs", "-really-quiet"]),
            ("xine", vec!["-f", "-q"]), // fullscreen, quiet
        ];

        for (player, args) in alternative_players {
            log::info!("Пробуем альтернативный плеер: {}", player);

            let mut cmd = std::process::Command::new(player);
            for arg in &args {
                cmd.arg(arg);
            }
            cmd.arg(&video_path);
            cmd.stdout(std::process::Stdio::null())
               .stderr(std::process::Stdio::null());

            log::info!("Запускаем команду: {:?}", cmd);
            let result = cmd.spawn();

            match result {
                Ok(child) => {
                    log::info!("{} успешно запущен, PID: {}", player, child.id());
                    return Ok(());
                }
                Err(e) => {
                    log::warn!("Не удалось запустить {}: {}", player, e);
                    continue;
                }
            }
        }

        // If all strategies failed, try one more time with minimal mpv options
        log::info!("Все плееры провалились, пробуем минимальную конфигурацию mpv");
        let result = std::process::Command::new("mpv")
            .arg("--no-config")
            .arg("--no-terminal")
            .arg(&video_path)
            .stdout(std::process::Stdio::null())
            .stderr(std::process::Stdio::null())
            .spawn();

        match result {
            Ok(child) => {
                log::info!("mpv запущен с минимальной конфигурацией, PID: {}", child.id());
                Ok(())
            }
            Err(e) => {
                log::error!("Не удалось запустить mpv даже с минимальной конфигурацией: {}", e);
                Err(anyhow::anyhow!("Не удалось запустить видео плеер. Установите mpv, vlc, ffplay, mplayer или xine"))
            }
        }
    }

    pub fn play_voice(&mut self) -> Result<()> {
        // Останавливаем текущее воспроизведение, если оно есть
        if self.audio_player.is_playing {
            self.audio_player.stop();
            log::info!("Остановлено предыдущее воспроизведение перед запуском нового");
        }

        // Get the actual voice file path from the current message
        if let Some(msg) = self.messages.get(self.selected_message_index) {
            if let Some(voice_path) = &msg.voice_path {
                log::info!("Пытаемся воспроизвести голосовое сообщение: {}", voice_path);

                // Проверяем, существует ли файл
                if !std::path::Path::new(voice_path).exists() {
                    return Err(anyhow::anyhow!("Файл голосового сообщения не существует: {}", voice_path));
                }

                // Проверяем, является ли это то же самое сообщение, что уже играет
                if self.audio_player.is_current_message(msg.id) && self.audio_player.is_playing {
                    // Останавливаем текущее воспроизведение
                    self.audio_player.stop();
                    log::info!("Остановлено воспроизведение голосового сообщения");
                    return Ok(());
                }



                // Инициализируем состояние аудио плеера
                self.audio_player.current_message_id = Some(msg.id);
                self.audio_player.current_position = Duration::ZERO;
                self.audio_player.total_duration = msg.voice_duration.map(|d| Duration::from_secs(d as u64));
                self.audio_player.is_playing = true;
                self.audio_player.current_file_path = Some(voice_path.clone()); // Store file path for restart functionality

                // Пробуем разные плееры для воспроизведения аудио с усилением громкости
                // ffplay как основной (работает надежно), mpv как запасной
                let audio_players = vec![
                    ("ffplay", vec!["-nodisp", "-autoexit", "-af", "volume=10"]),
                    ("mpv", vec![
                        "--volume=200",
                        "--input-ipc-server=/tmp/mpv-socket",
                        "--input-ipc-server=/tmp/mpv-socket:rw"  // Явно указываем права на чтение/запись
                    ]), // Для перемотки
                    ("mplayer", vec!["-really-quiet", "-noconsolecontrols", "-af", "volume=10"]),
                    ("play", vec!["-v", "10"]), // SoX play with 10x volume boost
                    ("paplay", vec![]), // PulseAudio player (no volume control)
                ];

                for (player, args) in audio_players {
                    log::info!("Пробуем плеер: {}", player);

                    let mut cmd = std::process::Command::new(player);
                    for arg in &args {
                        cmd.arg(arg);
                    }
                    cmd.arg(voice_path);

                    // Подавляем вывод для ffplay и других плееров
                    if player == "ffplay" {
                        cmd.stdout(std::process::Stdio::null())
                           .stderr(std::process::Stdio::null());
                    } else if player == "mplayer" {
                        cmd.stdout(std::process::Stdio::null())
                           .stderr(std::process::Stdio::null());
                    } else if player == "mpv" {
                        cmd.stdout(std::process::Stdio::null())
                           .stderr(std::process::Stdio::null());
                    } else if player == "play" {
                        cmd.stdout(std::process::Stdio::null())
                           .stderr(std::process::Stdio::null());
                    }

                    log::info!("Запускаем команду: {:?}", cmd);
                    let result = cmd.spawn();

                    match result {
                        Ok(child) => {
                            log::info!("{} успешно запущен, PID: {}", player, child.id());
                            self.audio_player.process_id = Some(child.id() as u32);
                            // Устанавливаем время начала воспроизведения
                            self.audio_start_time = Some(Instant::now());
                            return Ok(());
                        }
                        Err(e) => {
                            log::warn!("Не удалось запустить {}: {}", player, e);
                            continue;
                        }
                    }
                }

                // Если ни один плеер не сработал
                log::error!("Не удалось найти подходящий аудио плеер");
                self.audio_player.is_playing = false;
                self.audio_player.current_message_id = None;
                Err(anyhow::anyhow!("Не удалось найти подходящий аудио плеер. Установите mpv, ffplay, mplayer, sox или alsa-utils"))
            } else {
                log::error!("Путь к файлу голосового сообщения не найден в сообщении");
                return Err(anyhow::anyhow!("Путь к файлу голосового сообщения не найден"));
            }
        } else {
            log::error!("Сообщение не найдено по индексу {}", self.selected_message_index);
            return Err(anyhow::anyhow!("Сообщение не найдено"));
        }
    }

    pub fn play_audio(&mut self) -> Result<()> {
        // Останавливаем текущее воспроизведение, если оно есть
        if self.audio_player.is_playing {
            self.audio_player.stop();
            log::info!("Остановлено предыдущее воспроизведение перед запуском нового");
        }

        // Get the actual audio file path from the current message
        if let Some(msg) = self.messages.get(self.selected_message_index) {
            if let Some(audio_path) = &msg.audio_path {
                log::info!("Пытаемся воспроизвести аудио сообщение: {}", audio_path);

                // Проверяем, существует ли файл
                if !std::path::Path::new(audio_path).exists() {
                    return Err(anyhow::anyhow!("Файл аудио сообщения не существует: {}", audio_path));
                }

                // Проверяем, является ли это то же самое сообщение, что уже играет
                if self.audio_player.is_current_message(msg.id) && self.audio_player.is_playing {
                    // Останавливаем текущее воспроизведение
                    self.audio_player.stop();
                    log::info!("Остановлено воспроизведение аудио сообщения");
                    return Ok(());
                }



                // Инициализируем состояние аудио плеера
                self.audio_player.current_message_id = Some(msg.id);
                self.audio_player.current_position = Duration::ZERO;
                self.audio_player.total_duration = msg.audio_duration.map(|d| Duration::from_secs(d as u64));
                self.audio_player.is_playing = true;
                self.audio_player.current_file_path = Some(audio_path.clone()); // Store file path for restart functionality

                // Пробуем разные плееры для воспроизведения аудио с усилением громкости
                // ffplay как основной (работает надежно), mpv как запасной
                let audio_players = vec![
                    ("ffplay", vec!["-nodisp", "-autoexit", "-af", "volume=10"]),
                    ("mpv", vec![
                        "--volume=200",
                        "--input-ipc-server=/tmp/mpv-socket",
                        "--input-ipc-server=/tmp/mpv-socket:rw"  // Явно указываем права на чтение/запись
                    ]), // Для перемотки
                    ("mplayer", vec!["-really-quiet", "-noconsolecontrols", "-af", "volume=10"]),
                    ("play", vec!["-v", "10"]), // SoX play with 10x volume boost
                    ("paplay", vec![]), // PulseAudio player (no volume control)
                ];

                for (player, args) in audio_players {
                    log::info!("Пробуем плеер: {}", player);

                    let mut cmd = std::process::Command::new(player);
                    for arg in &args {
                        cmd.arg(arg);
                    }
                    cmd.arg(audio_path);

                    // Подавляем вывод для ffplay и других плееров
                    if player == "ffplay" {
                        cmd.stdout(std::process::Stdio::null())
                           .stderr(std::process::Stdio::null());
                    } else if player == "mplayer" {
                        cmd.stdout(std::process::Stdio::null())
                           .stderr(std::process::Stdio::null());
                    } else if player == "mpv" {
                        cmd.stdout(std::process::Stdio::null())
                           .stderr(std::process::Stdio::null());
                    } else if player == "play" {
                        cmd.stdout(std::process::Stdio::null())
                           .stderr(std::process::Stdio::null());
                    }

                    log::info!("Запускаем команду: {:?}", cmd);
                    let result = cmd.spawn();

                    match result {
                        Ok(child) => {
                            log::info!("{} успешно запущен, PID: {}", player, child.id());
                            self.audio_player.process_id = Some(child.id() as u32);
                            // Устанавливаем время начала воспроизведения
                            self.audio_start_time = Some(Instant::now());
                            return Ok(());
                        }
                        Err(e) => {
                            log::warn!("Не удалось запустить {}: {}", player, e);
                            continue;
                        }
                    }
                }

                // Если ни один плеер не сработал
                log::error!("Не удалось найти подходящий аудио плеер");
                self.audio_player.is_playing = false;
                self.audio_player.current_message_id = None;
                Err(anyhow::anyhow!("Не удалось найти подходящий аудио плеер. Установите mpv, ffplay, mplayer, sox или alsa-utils"))
            } else {
                log::error!("Путь к файлу аудио сообщения не найден в сообщении");
                return Err(anyhow::anyhow!("Путь к файлу аудио сообщения не найден"));
            }
        } else {
            log::error!("Сообщение не найдено по индексу {}", self.selected_message_index);
            return Err(anyhow::anyhow!("Сообщение не найдено"));
        }
    }

    fn get_terminal_window_id(&self) -> Option<u64> {
        // Пробуем различные способы получить ID окна терминала

        // Способ 1: через переменную окружения WINDOWID (для X11)
        if let Ok(window_id_str) = std::env::var("WINDOWID") {
            if let Ok(wid) = window_id_str.parse::<u64>() {
                // Проверяем, что ID не равен 0 (некорректное значение)
                if wid > 0 {
                    log::info!("Получен window ID из переменной WINDOWID: {}", wid);
                    return Some(wid);
                } else {
                    log::warn!("WINDOWID содержит некорректное значение: {}", wid);
                }
            } else {
                log::warn!("Не удалось распарсить WINDOWID: {}", window_id_str);
            }
        } else {
            log::debug!("Переменная WINDOWID не установлена");
        }

        // Способ 2: через xdotool (если доступен)
        if let Ok(output) = std::process::Command::new("xdotool")
            .args(&["getactivewindow"])
            .output() {
            if output.status.success() {
                if let Ok(window_id_str) = String::from_utf8(output.stdout) {
                    if let Ok(wid) = window_id_str.trim().parse::<u64>() {
                        if wid > 0 {
                            log::info!("Получен window ID через xdotool: {}", wid);
                            return Some(wid);
                        } else {
                            log::warn!("xdotool вернул некорректный window ID: {}", wid);
                        }
                    } else {
                        log::warn!("Не удалось распарсить вывод xdotool: {}", window_id_str);
                    }
                } else {
                    log::warn!("Вывод xdotool не является валидной UTF-8 строкой");
                }
            } else {
                log::debug!("xdotool не найден или вернул ошибку");
            }
        }

        // Способ 3: через xprop (если доступен)
        if let Ok(output) = std::process::Command::new("xprop")
            .args(&["-root", "_NET_ACTIVE_WINDOW"])
            .output() {
            if output.status.success() {
                if let Ok(output_str) = String::from_utf8(output.stdout) {
                    // Парсим вывод вида "_NET_ACTIVE_WINDOW(WINDOW): window id # 0x..."
                    if let Some(hex_id) = output_str.split("0x").nth(1) {
                        if let Some(hex_clean) = hex_id.split_whitespace().next() {
                            if let Ok(wid) = u64::from_str_radix(hex_clean, 16) {
                                if wid > 0 {
                                    log::info!("Получен window ID через xprop: {}", wid);
                                    return Some(wid);
                                } else {
                                    log::warn!("xprop вернул некорректный window ID: {}", wid);
                                }
                            } else {
                                log::warn!("Не удалось распарсить hex значение: {}", hex_clean);
                            }
                        } else {
                            log::warn!("Не удалось найти hex часть в выводе xprop: {}", output_str);
                        }
                    } else {
                        log::warn!("Не найден hex ID в выводе xprop: {}", output_str);
                    }
                } else {
                    log::warn!("Вывод xprop не является валидной UTF-8 строкой");
                }
            } else {
                log::debug!("xprop не найден или вернул ошибку");
            }
        }

        log::warn!("Не удалось получить корректный window ID ни одним из способов");
        None
    }

    pub async fn select_chat(&mut self) -> Result<()> {
        if self.selected_chat_index < self.chats.len() {
            self.selected_chat = Some(self.chats[self.selected_chat_index].clone());
            self.last_loaded_chat_id = self.selected_chat.as_ref().map(|c| c.id);
            self.load_messages().await?;
        }
        Ok(())
    }

    pub async fn send_message(&mut self) -> Result<()> {
        if let Some(chat) = &self.selected_chat {
            match self.api_client.send_message(chat.id, &self.message_input).await {
                Ok(response) => {
                    if response.success {
                        // Обновляем сообщения после отправки
                        self.load_messages().await?;
                    } else {
                        self.show_error(&response.message);
                    }
                }
                Err(e) => {
                    self.show_error(&format!("Ошибка отправки сообщения: {}", e));
                }
            }
        }

        Ok(())
    }

    pub fn show_error(&mut self, message: &str) {
        self.error_message = message.to_string();
        self.state = AppState::Error;
    }

    pub fn restart_player_at_position(&mut self) {
        // Этот метод перезапустит плеер с нужной позиции
        log::debug!("Restarting player at position: {}", format_duration(self.audio_player.current_position));

        // Проверяем, что у нас есть путь к файлу для перезапуска
        if let Some(file_path) = &self.audio_player.current_file_path {
            // Останавливаем текущий процесс
            if let Some(pid) = self.audio_player.process_id {
                let _ = std::process::Command::new("kill")
                    .arg("-TERM")
                    .arg(pid.to_string())
                    .status();
                log::debug!("Killed old process {}", pid);
            }

            // Получаем позицию в секундах для перезапуска
            let position_seconds = self.audio_player.current_position.as_secs();

            // Создаем строковые значения для использования в векторах
            let ffplay_ss_arg = format!("-ss {}", position_seconds);
            let mpv_start_arg = format!("--start={}", position_seconds);
            let mplayer_ss_arg = format!("-ss {}", position_seconds);
            let ffplay_position_str = position_seconds.to_string();

            // Пробуем разные плееры для перезапуска с нужной позиции
            let audio_players = vec![
                ("ffplay", vec![
                    "-nodisp",
                    "-autoexit",
                    "-af", "volume=10",
                    "-ss", &ffplay_position_str, // start position
                    file_path
                ]),
                ("mpv", vec![
                    "--volume=200",
                    "--input-ipc-server=/tmp/mpv-socket",
                    &mpv_start_arg,
                    file_path
                ]),
                ("mplayer", vec![
                    "-really-quiet",
                    "-noconsolecontrols",
                    "-af", "volume=10",
                    &mplayer_ss_arg,
                    file_path
                ]),
            ];

            for (player, args) in audio_players {
                log::debug!("Attempting to restart with {} at position {}s", player, position_seconds);

                let mut cmd = std::process::Command::new(player);
                for arg in &args {
                    cmd.arg(arg);
                }

                // Подавляем вывод для всех плееров
                cmd.stdout(std::process::Stdio::null())
                   .stderr(std::process::Stdio::null());

                log::debug!("Restart command: {:?}", cmd);
                let result = cmd.spawn();

                match result {
                    Ok(child) => {
                        log::debug!("Successfully restarted {} at position {}s, new PID: {}",
                            player, position_seconds, child.id());

                        // Обновляем process_id и устанавливаем новое время начала
                        self.audio_player.process_id = Some(child.id() as u32);
                        // Корректируем время начала так, чтобы позиция продолжала отображаться правильно
                        self.audio_start_time = Some(std::time::Instant::now() - self.audio_player.current_position);

                        return;
                    }
                    Err(e) => {
                        log::warn!("Failed to restart {}: {}", player, e);
                        continue;
                    }
                }
            }

            log::error!("Failed to restart player with any available audio player");
        } else {
            log::error!("No current file path available for restart");
        }
    }

    pub fn get_current_chat_title(&self) -> String {
        self.selected_chat
            .as_ref()
            .map(|c| c.title.clone())
            .unwrap_or_else(|| "Выберите чат".to_string())
    }

    pub fn get_status_text(&self) -> String {
        match self.state {
            AppState::Loading => "Загрузка...".to_string(),
            AppState::PhoneInput => "Введите номер телефона".to_string(),
            AppState::CodeInput => "Введите код подтверждения".to_string(),
            AppState::Main => {
                if self.chats.is_empty() {
                    "Нет чатов".to_string()
                } else {
                    let focus = if self.focus_on_messages { "Сообщения" } else { "Чаты" };
                    format!(
                        "Чатов: {} | Фокус: {} | q: выход, Tab: переключить фокус, ↑↓: навигация, Enter: открыть/проиграть, i: сообщение, r: обновить",
                        self.chats.len(), focus
                    )
                }
            }
            AppState::MessageInput => "Введите сообщение (Enter: отправить, Esc: отмена)".to_string(),
            AppState::Error => format!("Ошибка: {}", self.error_message),
            AppState::ImagePreview => {
                if let Some(path) = &self.preview_image_path {
                    if let Some(video_path) = &self.preview_video_path {
                        if !video_path.is_empty() {
                            format!("Превью видео: {} | Enter: воспроизвести в mpv, Esc: назад", path)
                        } else {
                            format!("Предпросмотр изображения: {}", path)
                        }
                    } else {
                        format!("Предпросмотр изображения: {}", path)
                    }
                } else {
                    "Предпросмотр изображения".to_string()
                }
            }
            AppState::VideoPreview => {
                if let Some(path) = &self.preview_video_path {
                    format!("Предпросмотр видео: {} | Enter: воспроизвести в mpv, Esc: назад", path)
                } else {
                    "Предпросмотр видео".to_string()
                }
            }
        }
    }

    pub fn calculate_visible_capacity(&self) -> usize {
        // Возвращаем разумное значение по умолчанию, реальный расчет будет в UI
        15 // Предполагаем 15 сообщений по умолчанию
    }

    pub fn set_actual_visible_capacity(&mut self, capacity: usize) {
        // Этот метод будет вызван из UI с реальными размерами
        self.visible_capacity = capacity;
    }

    pub fn get_actual_visible_capacity(&self) -> usize {
        self.visible_capacity
    }

    pub fn open_location(&mut self) -> Result<()> {
        // Get the current location message
        if let Some(msg) = self.messages.get(self.selected_message_index) {
            if msg.r#type == "location" {
                log::info!("Открываем подробную информацию о местоположении");

                // Construct map URL with coordinates
                if let (Some(lat), Some(lng)) = (msg.location_lat, msg.location_lng) {
                    // Use the API endpoint provided by the backend
                    let base_url = if let Some(ref map_path) = msg.location_map_path {
                        map_path.clone()
                    } else {
                        format!("/api/locations/{}/map", msg.location_id.unwrap_or(msg.id as i64))
                    };

                    // Check if URL already has query parameters
                    let separator = if base_url.contains('?') { '&' } else { '?' };
                    let map_url = format!("{}{}lat={:.6}&lng={:.6}", base_url, separator, lat, lng);

                    let full_map_url = format!("http://localhost:8080{}", map_url);
                    log::info!("Запрашиваем карту с координатами: {} для сообщения {}", full_map_url, msg.id);

                    // For now, set a placeholder path - the actual download will happen when the image is displayed
                    let temp_path = format!("/tmp/vi-tg_location_preview_{}.png", msg.id);
                    self.preview_image_path = Some(temp_path.clone());

                    // Spawn async task to download the map image
                    let url_clone = full_map_url.clone();
                    let message_id = msg.id;
                    tokio::spawn(async move {
                        match download_map_image_async(&url_clone, message_id).await {
                            Ok(local_path) => {
                                log::info!("Карта успешно скачана в фоновом режиме: {}", local_path);
                                // Note: We can't update self.preview_image_path here since we're in a different task
                                // The UI will need to check if the file exists when trying to display it
                            }
                            Err(e) => {
                                log::error!("Ошибка скачивания карты в фоновом режиме: {}", e);
                            }
                        }
                    });
                } else {
                    // Fallback to basic map path without coordinates
                    if let Some(ref map_path) = msg.location_map_path {
                        let full_map_url = format!("http://localhost:8080{}", map_path);
                        log::warn!("Координаты не найдены, используем базовый путь к карте");

                        // For now, set a placeholder path
                        let temp_path = format!("/tmp/vi-tg_location_preview_{}.png", msg.id);
                        self.preview_image_path = Some(temp_path.clone());

                        // Spawn async task to download the map image
                        let url_clone = full_map_url.clone();
                        let message_id = msg.id;
                        tokio::spawn(async move {
                            match download_map_image_async(&url_clone, message_id).await {
                                Ok(local_path) => {
                                    log::info!("Карта успешно скачана в фоновом режиме: {}", local_path);
                                }
                                Err(e) => {
                                    log::error!("Ошибка скачивания карты в фоновом режиме: {}", e);
                                }
                            }
                        });
                    } else {
                        log::error!("Путь к карте не найден и координаты отсутствуют");
                        return Err(anyhow::anyhow!("Путь к карте не найден"));
                    }
                }

                self.state = AppState::ImagePreview; // Use ImagePreview to show map if available

                log::info!("Установлен режим просмотра местоположения");
                return Ok(());
            } else {
                log::error!("Сообщение не является сообщением о местоположении");
                return Err(anyhow::anyhow!("Сообщение не является сообщением о местоположении"));
            }
        } else {
            log::error!("Сообщение не найдено по индексу {}", self.selected_message_index);
            return Err(anyhow::anyhow!("Сообщение не найдено"));
        }
    }

    async fn download_map_image(&self, url: &str, message_id: i32) -> Result<String> {
        log::info!("Скачиваем карту с URL: {}", url);

        // Create a temporary file path for the downloaded image
        let temp_path = format!("/tmp/vi-tg_location_preview_{}.png", message_id);

        // Check if we already have this image downloaded
        if std::path::Path::new(&temp_path).exists() {
            log::info!("Карта уже скачана, используем существующий файл: {}", temp_path);
            return Ok(temp_path);
        }

        // Create HTTP client and download the image
        let client = reqwest::Client::new();
        let response = client.get(url).send().await
            .map_err(|e| anyhow::anyhow!("Ошибка HTTP запроса: {}", e))?;

        if !response.status().is_success() {
            return Err(anyhow::anyhow!("HTTP ошибка: {} для URL: {}", response.status(), url));
        }

        // Read the image data
        let image_data = response.bytes().await
            .map_err(|e| anyhow::anyhow!("Ошибка чтения данных изображения: {}", e))?;

        // Write to temporary file
        tokio::fs::write(&temp_path, &image_data).await
            .map_err(|e| anyhow::anyhow!("Ошибка сохранения файла: {}", e))?;

        log::info!("Карта успешно скачана и сохранена в: {}", temp_path);
        Ok(temp_path)
    }
}
