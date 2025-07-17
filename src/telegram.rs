use anyhow::Result;
use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};
use rust_tdlib::{client::{Client, Worker}, types::*};
use std::sync::Arc;
use tokio::sync::Mutex;

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Chat {
    pub id: i64,
    pub name: String,
    pub chat_type: String,
    pub unread_count: u32,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Message {
    pub id: i32,
    pub text: String,
    pub from: String,
    pub timestamp: DateTime<Utc>,
    pub chat_id: i64,
    pub message_type: String, // "text", "sticker", "photo", "video", etc.
    pub sticker_id: Option<i64>,
    pub sticker_emoji: Option<String>,
    pub sticker_path: Option<String>,
}

#[async_trait::async_trait]
pub trait TelegramClient: Send + Sync {
    async fn send_message(&self, chat_id: i64, text: &str) -> Result<()>;
    async fn get_chats(&self) -> Result<Vec<Chat>>;
    async fn get_messages(&self, chat_id: i64, limit: usize) -> Result<Vec<Message>>;
}

// TDLib клиент
pub struct TdlibClient {
    client: Arc<Mutex<Option<Client<rust_tdlib::tdjson::TdJson>>>>,
}

impl TdlibClient {
    pub fn new() -> Self {
        Self {
            client: Arc::new(Mutex::new(None)),
        }
    }

    pub async fn initialize(&self, api_id: i32, api_hash: String) -> Result<()> {
        let mut worker = Worker::builder().build()?;
        let _waiter = worker.start();
        
        let tdlib_params = TdlibParameters::builder()
            .api_id(api_id)
            .api_hash(api_hash)
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
        
        Ok(())
    }
}

#[async_trait::async_trait]
impl TelegramClient for TdlibClient {
    async fn send_message(&self, chat_id: i64, text: &str) -> Result<()> {
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
        }
        Ok(())
    }
    
    async fn get_chats(&self) -> Result<Vec<Chat>> {
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
                    result.push(Chat {
                        id: chat.id,
                        name: chat.title,
                        chat_type: match chat.type_ {
                            ChatType::Private(_) => "private".to_string(),
                            ChatType::BasicGroup(_) => "group".to_string(),
                            ChatType::Supergroup(_) => "supergroup".to_string(),
                            ChatType::Secret(_) => "secret".to_string(),
                        },
                        unread_count: chat.unread_count as u32,
                    });
                }
            }
            
            return Ok(result);
        }
        Err(anyhow::anyhow!("TDLib клиент не инициализирован"))
    }
    
    async fn get_messages(&self, chat_id: i64, limit: usize) -> Result<Vec<Message>> {
        let client_guard = self.client.lock().await;
        if let Some(client) = client_guard.as_ref() {
            let get_chat_history = GetChatHistory::builder()
                .chat_id(chat_id)
                .limit(limit as i32)
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
                    timestamp: DateTime::from_timestamp(message.date as i64, 0).unwrap_or(Utc::now()),
                    chat_id: message.chat_id,
                    message_type: "text".to_string(),
                    sticker_id: None,
                    sticker_emoji: None,
                    sticker_path: None,
                });
            }
            
            return Ok(result);
        }
        Err(anyhow::anyhow!("TDLib клиент не инициализирован"))
    }
}

// Мок клиент для тестирования
pub struct MockClient;

#[async_trait::async_trait]
impl TelegramClient for MockClient {
    async fn send_message(&self, _chat_id: i64, _text: &str) -> Result<()> {
        log::info!("Отправка сообщения: {}", _text);
        Ok(())
    }
    
    async fn get_chats(&self) -> Result<Vec<Chat>> {
        Ok(vec![
            Chat {
                id: 1,
                name: "Общий чат".to_string(),
                chat_type: "group".to_string(),
                unread_count: 0,
            },
            Chat {
                id: 2,
                name: "Тестовый чат".to_string(),
                chat_type: "private".to_string(),
                unread_count: 3,
            },
            Chat {
                id: 3,
                name: "Рабочий чат".to_string(),
                chat_type: "group".to_string(),
                unread_count: 1,
            },
            Chat {
                id: 4,
                name: "Семейный чат".to_string(),
                chat_type: "private".to_string(),
                unread_count: 5,
            },
            Chat {
                id: 5,
                name: "Друзья".to_string(),
                chat_type: "group".to_string(),
                unread_count: 0,
            },
        ])
    }
    
    async fn get_messages(&self, chat_id: i64, _limit: usize) -> Result<Vec<Message>> {
        let messages = match chat_id {
            1 => vec![
                Message {
                    id: 1,
                    text: "Привет всем! Как дела?".to_string(),
                    from: "Алексей".to_string(),
                    timestamp: Utc::now() - chrono::Duration::minutes(30),
                    chat_id,
                    message_type: "text".to_string(),
                    sticker_id: None,
                    sticker_emoji: None,
                    sticker_path: None,
                },
                Message {
                    id: 2,
                    text: "Все отлично! Работаю над новым проектом".to_string(),
                    from: "Мария".to_string(),
                    timestamp: Utc::now() - chrono::Duration::minutes(25),
                    chat_id,
                    message_type: "text".to_string(),
                    sticker_id: None,
                    sticker_emoji: None,
                    sticker_path: None,
                },
                Message {
                    id: 3,
                    text: "Отлично! Расскажи подробнее".to_string(),
                    from: "Дмитрий".to_string(),
                    timestamp: Utc::now() - chrono::Duration::minutes(20),
                    chat_id,
                    message_type: "text".to_string(),
                    sticker_id: None,
                    sticker_emoji: None,
                    sticker_path: None,
                },
            ],
            2 => vec![
                Message {
                    id: 4,
                    text: "Это Telegram клиент на Rust с TUI интерфейсом".to_string(),
                    from: "Анна".to_string(),
                    timestamp: Utc::now() - chrono::Duration::minutes(15),
                    chat_id,
                    message_type: "text".to_string(),
                    sticker_id: None,
                    sticker_emoji: None,
                    sticker_path: None,
                },
                Message {
                    id: 5,
                    text: "Круто! Используешь ratatui?".to_string(),
                    from: "Сергей".to_string(),
                    timestamp: Utc::now() - chrono::Duration::minutes(10),
                    chat_id,
                    message_type: "text".to_string(),
                    sticker_id: None,
                    sticker_emoji: None,
                    sticker_path: None,
                },
                Message {
                    id: 6,
                    text: "Да, именно! Очень удобная библиотека".to_string(),
                    from: "Анна".to_string(),
                    timestamp: Utc::now() - chrono::Duration::minutes(5),
                    chat_id,
                    message_type: "text".to_string(),
                    sticker_id: None,
                    sticker_emoji: None,
                    sticker_path: None,
                },
            ],
            _ => vec![
                Message {
                    id: 7,
                    text: "Сообщение в чате".to_string(),
                    from: "Пользователь".to_string(),
                    timestamp: Utc::now(),
                    chat_id,
                    message_type: "text".to_string(),
                    sticker_id: None,
                    sticker_emoji: None,
                    sticker_path: None,
                },
            ],
        };
        
        Ok(messages)
    }
}

// Enum для хранения разных типов клиентов
pub enum TelegramClientEnum {
    Tdlib(TdlibClient),
    Mock(MockClient),
}

#[async_trait::async_trait]
impl TelegramClient for TelegramClientEnum {
    async fn send_message(&self, chat_id: i64, text: &str) -> Result<()> {
        match self {
            TelegramClientEnum::Tdlib(client) => client.send_message(chat_id, text).await,
            TelegramClientEnum::Mock(client) => client.send_message(chat_id, text).await,
        }
    }
    
    async fn get_chats(&self) -> Result<Vec<Chat>> {
        match self {
            TelegramClientEnum::Tdlib(client) => client.get_chats().await,
            TelegramClientEnum::Mock(client) => client.get_chats().await,
        }
    }
    
    async fn get_messages(&self, chat_id: i64, limit: usize) -> Result<Vec<Message>> {
        match self {
            TelegramClientEnum::Tdlib(client) => client.get_messages(chat_id, limit).await,
            TelegramClientEnum::Mock(client) => client.get_messages(chat_id, limit).await,
        }
    }
} 