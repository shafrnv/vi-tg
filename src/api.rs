use anyhow::Result;
use reqwest::Client;
use serde::{Deserialize, Serialize};

use crate::{AuthStatus, Chat, Message};

#[derive(Debug, Clone)]
pub struct ApiClient {
    client: Client,
    base_url: String,
}

#[derive(Debug, Serialize, Deserialize)]
struct PhoneRequest {
    phone: String,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct PhoneResponse {
    pub success: bool,
    pub message: String,
    pub needs_code: bool,
}

#[derive(Debug, Serialize, Deserialize)]
struct CodeRequest {
    code: String,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct CodeResponse {
    pub success: bool,
    pub message: String,
    pub authorized: bool,
}

#[derive(Debug, Serialize, Deserialize)]
struct ChatsResponse {
    chats: Vec<Chat>,
}

#[derive(Debug, Serialize, Deserialize)]
struct MessagesResponse {
    messages: Vec<Message>,
}

#[derive(Debug, Serialize, Deserialize)]
struct SendMessageRequest {
    text: String,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct SendMessageResponse {
    pub success: bool,
    pub message: String,
    pub message_id: Option<i32>,
}

#[derive(Debug, Serialize, Deserialize)]
struct ErrorResponse {
    error: String,
    code: i32,
}

impl ApiClient {
    pub fn new(base_url: String) -> Self {
        Self {
            client: Client::new(),
            base_url,
        }
    }

    pub async fn get_auth_status(&self) -> Result<AuthStatus> {
        let url = format!("{}/api/auth/status", self.base_url);
        let response = self.client.get(&url).send().await?;
        
        if response.status().is_success() {
            let auth_status: AuthStatus = response.json().await?;
            Ok(auth_status)
        } else {
            let error: ErrorResponse = response.json().await?;
            Err(anyhow::anyhow!("API error: {}", error.error))
        }
    }

    pub async fn set_phone_number(&self, phone: &str) -> Result<PhoneResponse> {
        let url = format!("{}/api/auth/phone", self.base_url);
        let request = PhoneRequest {
            phone: phone.to_string(),
        };
        
        let response = self.client
            .post(&url)
            .json(&request)
            .send()
            .await?;
        
        if response.status().is_success() {
            let phone_response: PhoneResponse = response.json().await?;
            Ok(phone_response)
        } else {
            let error: ErrorResponse = response.json().await?;
            Err(anyhow::anyhow!("API error: {}", error.error))
        }
    }

    pub async fn send_code(&self, code: &str) -> Result<CodeResponse> {
        let url = format!("{}/api/auth/code", self.base_url);
        let request = CodeRequest {
            code: code.to_string(),
        };
        
        let response = self.client
            .post(&url)
            .json(&request)
            .send()
            .await?;
        
        if response.status().is_success() {
            let code_response: CodeResponse = response.json().await?;
            Ok(code_response)
        } else {
            let error: ErrorResponse = response.json().await?;
            Err(anyhow::anyhow!("API error: {}", error.error))
        }
    }

    pub async fn get_chats(&self) -> Result<Vec<Chat>> {
        let url = format!("{}/api/chats", self.base_url);
        let response = self.client.get(&url).send().await?;
        
        if response.status().is_success() {
            let chats_response: ChatsResponse = response.json().await?;
            Ok(chats_response.chats)
        } else {
            let error: ErrorResponse = response.json().await?;
            Err(anyhow::anyhow!("API error: {}", error.error))
        }
    }

    pub async fn get_messages(&self, chat_id: i64, limit: Option<i32>) -> Result<Vec<Message>> {
        let mut url = format!("{}/api/chats/{}/messages", self.base_url, chat_id);
        
        if let Some(limit) = limit {
            url = format!("{}?limit={}", url, limit);
        }
        
        let response = self.client.get(&url).send().await?;
        
        if response.status().is_success() {
            let messages_response: MessagesResponse = response.json().await?;
            Ok(messages_response.messages)
        } else {
            let error: ErrorResponse = response.json().await?;
            Err(anyhow::anyhow!("API error: {}", error.error))
        }
    }

    pub async fn send_message(&self, chat_id: i64, text: &str) -> Result<SendMessageResponse> {
        let url = format!("{}/api/chats/{}/messages", self.base_url, chat_id);
        let request = SendMessageRequest {
            text: text.to_string(),
        };
        
        let response = self.client
            .post(&url)
            .json(&request)
            .send()
            .await?;
        
        if response.status().is_success() {
            let send_response: SendMessageResponse = response.json().await?;
            Ok(send_response)
        } else {
            let error: ErrorResponse = response.json().await?;
            Err(anyhow::anyhow!("API error: {}", error.error))
        }
    }

    pub async fn get_sticker(&self, sticker_id: i64) -> Result<Vec<u8>> {
        let url = format!("{}/api/stickers/{}", self.base_url, sticker_id);
        let response = self.client.get(&url).send().await?;
        
        if response.status().is_success() {
            let bytes = response.bytes().await?;
            Ok(bytes.to_vec())
        } else {
            let error: ErrorResponse = response.json().await?;
            Err(anyhow::anyhow!("API error: {}", error.error))
        }
    }
} 