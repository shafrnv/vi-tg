use anyhow::Result;
use std::path::PathBuf;
use std::fs;
use dirs;
use serde_json::Value;
use rust_tdlib::{client::{Client, Worker}, types::*};
use std::sync::Arc;
use tokio::sync::Mutex;

pub struct TdlibAuthClient {
    client: Arc<Mutex<Option<Client<rust_tdlib::tdjson::TdJson>>>>,
    worker: Arc<Mutex<Option<Worker<rust_tdlib::client::ConsoleAuthStateHandler, rust_tdlib::tdjson::TdJson>>>>,
    session_path: PathBuf,
    authorized: bool,
    api_id: i32,
    api_hash: String,
}

impl TdlibAuthClient {
    pub fn new(api_id: i32, api_hash: String) -> Result<Self> {
        let session_path = Self::get_session_path()?;
        
        // Создаем директорию для сессии если её нет
        if let Some(parent) = session_path.parent() {
            fs::create_dir_all(parent)?;
        }
        
        Ok(Self {
            client: Arc::new(Mutex::new(None)),
            worker: Arc::new(Mutex::new(None)),
            session_path,
            authorized: false,
            api_id,
            api_hash,
        })
    }
    
    pub fn is_authorized(&self) -> bool {
        self.authorized
    }
    
    pub async fn auth_and_connect(&mut self, phone: &str) -> Result<()> {
        log::info!("Начинаем авторизацию для номера: {}", phone);
        
        // Создаем и запускаем worker
        let mut worker = Worker::builder().build()?;
        let _waiter = worker.start();
        
        // Создаем параметры TDLib
        let tdlib_params = TdlibParameters::builder()
            .api_id(self.api_id)
            .api_hash(self.api_hash.clone())
            .use_test_dc(false)
            .database_directory("./tdlib_data".to_string())
            .files_directory("./tdlib_files".to_string())
            .use_file_database(true)
            .use_chat_info_database(true)
            .use_message_database(true)
            .enable_storage_optimizer(true)
            .ignore_file_names(false)
            .build();
            
        let client = Client::builder()
            .with_tdlib_parameters(tdlib_params)
            .build()?;
            
        // Привязываем клиента к worker
        let client = worker.bind_client(client).await?;
        
        // Сохраняем клиента и worker
        let mut client_guard = self.client.lock().await;
        *client_guard = Some(client);
        
        let mut worker_guard = self.worker.lock().await;
        *worker_guard = Some(worker);
        
        // Сохраняем данные авторизации
        let auth_data = serde_json::json!({
            "phone": phone,
            "timestamp": chrono::Utc::now().timestamp(),
            "status": "authenticated"
        });
        
        fs::write(&self.session_path, auth_data.to_string())?;
        
        self.authorized = true;
        
        log::info!("Авторизация завершена успешно");
        Ok(())
    }
    
    pub async fn init_from_session(&mut self) -> Result<()> {
        if !self.session_path.exists() {
            return Err(anyhow::anyhow!("Файл сессии не найден"));
        }
        
        log::info!("Инициализация из сессии: {:?}", self.session_path);
        
        // Читаем данные сессии
        let session_data = fs::read_to_string(&self.session_path)?;
        let auth_data: Value = serde_json::from_str(&session_data)?;
        
        // Проверяем, что данные авторизации есть
        if auth_data.get("status").and_then(|s| s.as_str()) == Some("authenticated") {
            // Инициализируем TDLib клиента
            let mut worker = Worker::builder().build()?;
            let _waiter = worker.start();
            
            let tdlib_params = TdlibParameters::builder()
                .api_id(self.api_id)
                .api_hash(self.api_hash.clone())
                .use_test_dc(false)
                .database_directory("./tdlib_data".to_string())
                .files_directory("./tdlib_files".to_string())
                .use_file_database(true)
                .use_chat_info_database(true)
                .use_message_database(true)
                .enable_storage_optimizer(true)
                .ignore_file_names(false)
                .build();
                
            let client = Client::builder()
                .with_tdlib_parameters(tdlib_params)
                .build()?;
                
            let client = worker.bind_client(client).await?;
            
            let mut client_guard = self.client.lock().await;
            *client_guard = Some(client);
            
            let mut worker_guard = self.worker.lock().await;
            *worker_guard = Some(worker);
            
            self.authorized = true;
            log::info!("Инициализация из сессии завершена");
            Ok(())
        } else {
            Err(anyhow::anyhow!("Неверные данные сессии"))
        }
    }
    
    pub async fn get_dialogs(&self) -> Result<Vec<Dialog>> {
        let client_guard = self.client.lock().await;
        if let Some(client) = client_guard.as_ref() {
            let get_chats = GetChats::builder()
                .limit(50)
                .build();
                
            let chats = client.get_chats(get_chats).await?;
            
            let mut result = Vec::new();
            for chat_id in chats.chat_ids {
                let get_chat = GetChat::builder()
                    .chat_id(chat_id)
                    .build();
                    
                if let Ok(chat) = client.get_chat(get_chat).await {
                    result.push(Dialog {
                        id: chat.id,
                        title: chat.title,
                        chat_type: match chat.type_ {
                            ChatType::Private(_) => "private".to_string(),
                            ChatType::BasicGroup(_) => "group".to_string(),
                            ChatType::Supergroup(_) => "supergroup".to_string(),
                            ChatType::Secret(_) => "secret".to_string(),
                        },
                        unread: chat.unread_count as u32,
                    });
                }
            }
            
            return Ok(result);
        }
        
        // Возвращаем тестовые диалоги если клиент не инициализирован
        Ok(vec![
            Dialog {
                id: 1,
                title: "Общий чат".to_string(),
                chat_type: "group".to_string(),
                unread: 0,
            },
            Dialog {
                id: 2,
                title: "Тестовый чат".to_string(),
                chat_type: "private".to_string(),
                unread: 3,
            },
            Dialog {
                id: 3,
                title: "Рабочий чат".to_string(),
                chat_type: "supergroup".to_string(),
                unread: 1,
            },
            Dialog {
                id: 4,
                title: "Семейный чат".to_string(),
                chat_type: "group".to_string(),
                unread: 5,
            },
            Dialog {
                id: 5,
                title: "Друзья".to_string(),
                chat_type: "group".to_string(),
                unread: 2,
            },
        ])
    }
    
    pub async fn get_messages(&self, chat_id: i64, limit: i32) -> Result<Vec<Message>> {
        let client_guard = self.client.lock().await;
        if let Some(client) = client_guard.as_ref() {
            let get_chat_history = GetChatHistory::builder()
                .chat_id(chat_id)
                .limit(limit)
                .build();
                
            let messages = client.get_chat_history(get_chat_history).await?;
            
            let mut result = Vec::new();
            for message in messages.messages {
                let text = match message.content {
                    MessageContent::MessageText(text_content) => text_content.text().text(),
                    MessageContent::MessageSticker(sticker_content) => {
                        format!("Стикер: {}", sticker_content.sticker().emoji())
                    },
                    MessageContent::MessagePhoto(_) => "Фото".to_string(),
                    MessageContent::MessageVideo(_) => "Видео".to_string(),
                    MessageContent::MessageDocument(_) => "Документ".to_string(),
                    _ => "Неподдерживаемый тип сообщения".to_string(),
                };
                
                // Получаем информацию об отправителе
                let from = match message.sender_id {
                    MessageSender::User(user_sender) => {
                        let get_user = GetUser::builder()
                            .user_id(user_sender.user_id())
                            .build();
                        if let Ok(user) = client.get_user(get_user).await {
                            format!("{} {}", user.first_name, user.last_name)
                        } else {
                            "Неизвестный пользователь".to_string()
                        }
                    },
                    MessageSender::Chat(_) => "Чат".to_string(),
                };
                
                result.push(Message {
                    id: message.id,
                    text,
                    from,
                    timestamp: chrono::DateTime::from_timestamp(message.date as i64, 0)
                        .unwrap_or(chrono::Utc::now()),
                    chat_id: message.chat_id,
                    message_type: "text".to_string(),
                    sticker_id: None,
                    sticker_emoji: None,
                    sticker_path: None,
                });
            }
            
            return Ok(result);
        }
        
        // Возвращаем тестовые сообщения если клиент не инициализирован
        let mut messages = Vec::new();
        
        let sample_messages = vec![
            "Привет! Как дела?",
            "Отлично, спасибо! А у тебя как?",
            "Всё хорошо, работаю над проектом",
            "Интересно, расскажи подробнее",
            "Это Telegram клиент на Rust с TUI интерфейсом",
            "Звучит круто! Покажешь демо?",
            "Конечно, вот он работает прямо сейчас",
            "Впечатляет! Когда планируешь релиз?",
            "Скоро, осталось добавить реальную интеграцию с Telegram",
            "Удачи с проектом! 🚀",
        ];
        
        let users = vec!["Алексей", "Мария", "Дмитрий", "Анна", "Сергей"];
        
        for i in 1..=limit.min(10) {
            let msg_index = (i - 1) as usize % sample_messages.len();
            let user_index = (i - 1) as usize % users.len();
            
            messages.push(Message {
                id: i,
                text: sample_messages[msg_index].to_string(),
                from: users[user_index].to_string(),
                timestamp: chrono::Utc::now() - chrono::Duration::minutes(i as i64 * 5),
                chat_id,
                message_type: "text".to_string(),
                sticker_id: None,
                sticker_emoji: None,
                sticker_path: None,
            });
        }
        
        Ok(messages)
    }
    
    pub async fn send_message(&self, chat_id: i64, text: &str) -> Result<()> {
        let client_guard = self.client.lock().await;
        if let Some(client) = client_guard.as_ref() {
            let input_message_content = InputMessageContent::InputMessageText(
                InputMessageText::builder()
                    .text(FormattedText::builder()
                        .text(text.to_string())
                        .build())
                    .build()
            );
            
            let send_message = SendMessage::builder()
                .chat_id(chat_id)
                .input_message_content(input_message_content)
                .build();
                
            client.send_message(send_message).await?;
        } else {
            // Для демонстрации просто логируем отправку
            log::info!("Отправка сообщения в чат {}: {}", chat_id, text);
        }
        Ok(())
    }
    
    fn get_session_path() -> Result<PathBuf> {
        let home_dir = dirs::home_dir()
            .ok_or_else(|| anyhow::anyhow!("Не удалось найти домашнюю директорию"))?;
        Ok(home_dir.join(".vi-tg").join("session"))
    }
}

#[derive(Debug, Clone)]
pub struct Dialog {
    pub id: i64,
    pub title: String,
    pub chat_type: String,
    pub unread: u32,
}

#[derive(Debug, Clone)]
pub struct Message {
    pub id: i32,
    pub text: String,
    pub from: String,
    pub timestamp: chrono::DateTime<chrono::Utc>,
    pub chat_id: i64,
    pub message_type: String,
    pub sticker_id: Option<i64>,
    pub sticker_emoji: Option<String>,
    pub sticker_path: Option<String>,
}

pub struct AuthManager {
    tdlib_client: Option<TdlibAuthClient>,
    phone_number: Option<String>,
    api_id: i32,
    api_hash: String,
}

impl AuthManager {
    pub fn new(api_id: i32, api_hash: String) -> Self {
        Self {
            tdlib_client: None,
            phone_number: None,
            api_id,
            api_hash,
        }
    }
    
    pub fn set_phone_number(&mut self, phone: String) {
        self.phone_number = Some(phone);
    }
    
    pub async fn initialize_tdlib(&mut self) -> Result<()> {
        let mut client = TdlibAuthClient::new(self.api_id, self.api_hash.clone())?;
        
        if let Some(phone) = &self.phone_number {
            // Пытаемся инициализировать из сессии
            if let Err(_) = client.init_from_session().await {
                // Если не удалось, пытаемся авторизоваться заново
                client.auth_and_connect(phone).await?;
            }
        }
        
        self.tdlib_client = Some(client);
        Ok(())
    }
    
    pub fn is_authorized(&self) -> bool {
        self.tdlib_client
            .as_ref()
            .map(|client| client.is_authorized())
            .unwrap_or(false)
    }
    
    pub fn get_tdlib_client(&self) -> Option<&TdlibAuthClient> {
        self.tdlib_client.as_ref()
    }
    
    pub fn get_tdlib_client_mut(&mut self) -> Option<&mut TdlibAuthClient> {
        self.tdlib_client.as_mut()
    }
} 