use anyhow::Result;
use std::time::{Duration, Instant};
use std::collections::HashMap;

use crate::api::ApiClient;
use crate::{AuthStatus, Chat, Message};

#[derive(Debug, Clone, PartialEq)]
pub enum AppState {
    Loading,
    PhoneInput,
    CodeInput,
    Main,
    MessageInput,
    Error,
    ImagePreview,
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

    // Состояние ошибки
    pub error_message: String,

    // Изображения
    pub image_paths: HashMap<i64, String>,

    // Таймеры для обновления
    pub last_update: Instant,
    pub last_auth_check: Instant,
    pub last_data_refresh: Instant,

    // Реальная видимая емкость из UI
    pub visible_capacity: usize,
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
            error_message: String::new(),
            image_paths: HashMap::new(),
            last_update: Instant::now(),
            last_auth_check: Instant::now(),
            last_data_refresh: Instant::now(),
            visible_capacity: 15, // Значение по умолчанию
        }
    }

    pub async fn update(&mut self) -> Result<()> {
        let now = Instant::now();

        // В режиме предпросмотра картинки ничего не обновляем, чтобы не дергать layout
        if self.state == AppState::ImagePreview {
            return Ok(());
        }

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
            let chat_changed = self.last_loaded_chat_id != Some(current_chat_id);
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
            return;
        }
        if let Some(msg) = self.messages.get(self.selected_message_index) {
            if msg.r#type == "photo" {
                if let Some(path) = &msg.image_path {
                    self.preview_image_path = Some(path.clone());
                    self.state = AppState::ImagePreview;
                }
            }
        }
    }

    pub fn close_image_preview(&mut self) {
        self.preview_image_path = None;
        self.state = AppState::Main;
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
                        "Чатов: {} | Фокус: {} | q: выход, Tab: переключить фокус, ↑↓: навигация, Enter: действие, i: сообщение, r: обновить",
                        self.chats.len(), focus
                    )
                }
            }
            AppState::MessageInput => "Введите сообщение (Enter: отправить, Esc: отмена)".to_string(),
            AppState::Error => format!("Ошибка: {}", self.error_message),
            AppState::ImagePreview => {
                if let Some(path) = &self.preview_image_path {
                    format!("Предпросмотр изображения: {}", path)
                } else {
                    "Предпросмотр изображения".to_string()
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
}
