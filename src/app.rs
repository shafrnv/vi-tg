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
    
    // Состояние ошибки
    pub error_message: String,
    
    // Изображения
    pub image_paths: HashMap<i64, String>,
    
    // Таймеры для обновления
    pub last_update: Instant,
    pub last_auth_check: Instant,
    pub last_data_refresh: Instant,
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
            error_message: String::new(),
            image_paths: HashMap::new(),
            last_update: Instant::now(),
            last_auth_check: Instant::now(),
            last_data_refresh: Instant::now(),
        }
    }

    pub async fn update(&mut self) -> Result<()> {
        let now = Instant::now();
        
        // Проверяем авторизацию каждые 2 секунды
        if now.duration_since(self.last_auth_check) > Duration::from_secs(2) {
            self.check_auth_status().await?;
            self.last_auth_check = now;
        }
        
        // Обновляем данные каждые 5 секунд в основном состоянии
        if self.state == AppState::Main && 
           now.duration_since(self.last_data_refresh) > Duration::from_secs(5) {
            self.refresh_data().await?;
            self.last_data_refresh = now;
        }
        
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
            match self.api_client.get_messages(chat.id, Some(50)).await {
                Ok(messages) => {
                    self.messages = messages;
                    // Загружаем пути к изображениям
                    self.load_image_paths().await?;
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

    pub async fn select_chat(&mut self) -> Result<()> {
        if self.selected_chat_index < self.chats.len() {
            self.selected_chat = Some(self.chats[self.selected_chat_index].clone());
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
                    format!("Чатов: {} | q: выход, ↑↓: навигация, Enter: выбор, i: сообщение, r: обновить", self.chats.len())
                }
            }
            AppState::MessageInput => "Введите сообщение (Enter: отправить, Esc: отмена)".to_string(),
            AppState::Error => format!("Ошибка: {}", self.error_message),
        }
    }
}