use anyhow::Result;
use crossterm::event::{Event, KeyCode, KeyEvent, KeyEventKind};
use crossterm::terminal::{disable_raw_mode, enable_raw_mode, EnterAlternateScreen, LeaveAlternateScreen};
use crossterm::ExecutableCommand;
use ratatui::prelude::*;
use serde::{Deserialize, Serialize};
use std::io::stdout;
use std::time::Duration;
use tokio::time::sleep;

mod api;
mod app;
mod ui;

use api::ApiClient;
use app::{App, AppState};
use ui::draw_ui;

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Chat {
    pub id: i64,
    pub title: String,
    pub r#type: String,
    pub unread: i32,
    pub last_message: Option<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Message {
    pub id: i32,
    pub text: String,
    pub from: String,
    pub timestamp: String,
    pub chat_id: i64,
    pub r#type: String,
    pub sticker_id: Option<i64>,
    pub sticker_emoji: Option<String>,
    pub sticker_path: Option<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct AuthStatus {
    pub authorized: bool,
    pub phone_number: Option<String>,
    pub needs_code: bool,
}

#[tokio::main]
async fn main() -> Result<()> {
    // Инициализация логирования
    env_logger::init();
    
    // Создаем API клиент
    let api_client = ApiClient::new("http://localhost:8080".to_string());
    
    // Создаем приложение
    let mut app = App::new(api_client);
    
    // Запускаем TUI
    run_tui(&mut app).await?;
    
    Ok(())
}

async fn run_tui(app: &mut App) -> Result<()> {
    // Настраиваем терминал
    enable_raw_mode()?;
    stdout().execute(EnterAlternateScreen)?;
    
    let mut terminal = Terminal::new(CrosstermBackend::new(stdout()))?;
    
    // Основной цикл
    loop {
        // Отрисовываем UI
        terminal.draw(|frame| {
            draw_ui(frame, app);
        })?;
        
        // Обрабатываем события
        if crossterm::event::poll(Duration::from_millis(100))? {
            if let Event::Key(key) = crossterm::event::read()? {
                if key.kind == KeyEventKind::Press {
                    if handle_key_event(app, key).await? {
                        break; // Выход из приложения
                    }
                }
            }
        }
        
        // Периодически обновляем данные
        app.update().await?;
        
        // Небольшая задержка для снижения нагрузки на CPU
        sleep(Duration::from_millis(50)).await;
    }
    
    // Восстанавливаем терминал
    disable_raw_mode()?;
    stdout().execute(LeaveAlternateScreen)?;
    
    Ok(())
}

async fn handle_key_event(app: &mut App, key: KeyEvent) -> Result<bool> {
    match app.state {
        AppState::PhoneInput => {
            match key.code {
                KeyCode::Enter => {
                    if !app.phone_input.is_empty() {
                        app.set_phone_number().await?;
                    }
                }
                KeyCode::Esc => {
                    return Ok(true); // Выход
                }
                KeyCode::Backspace => {
                    app.phone_input.pop();
                }
                KeyCode::Char(c) => {
                    app.phone_input.push(c);
                }
                _ => {}
            }
        }
        AppState::CodeInput => {
            match key.code {
                KeyCode::Enter => {
                    if !app.code_input.is_empty() {
                        app.send_code().await?;
                    }
                }
                KeyCode::Esc => {
                    app.state = AppState::PhoneInput;
                    app.code_input.clear();
                }
                KeyCode::Backspace => {
                    app.code_input.pop();
                }
                KeyCode::Char(c) if c.is_ascii_digit() => {
                    if app.code_input.len() < 6 {
                        app.code_input.push(c);
                    }
                }
                _ => {}
            }
        }
        AppState::Main => {
            match key.code {
                KeyCode::Char('q') => {
                    return Ok(true); // Выход
                }
                KeyCode::Up => {
                    app.move_chat_selection(-1);
                }
                KeyCode::Down => {
                    app.move_chat_selection(1);
                }
                KeyCode::Enter => {
                    app.select_chat().await?;
                }
                KeyCode::Char('i') => {
                    if app.selected_chat.is_some() {
                        app.state = AppState::MessageInput;
                        app.message_input.clear();
                    }
                }
                KeyCode::Char('r') | KeyCode::F(5) => {
                    app.refresh_data().await?;
                }
                _ => {}
            }
        }
        AppState::MessageInput => {
            match key.code {
                KeyCode::Enter => {
                    if !app.message_input.is_empty() {
                        app.send_message().await?;
                        app.state = AppState::Main;
                        app.message_input.clear();
                    }
                }
                KeyCode::Esc => {
                    app.state = AppState::Main;
                    app.message_input.clear();
                }
                KeyCode::Backspace => {
                    app.message_input.pop();
                }
                KeyCode::Char(c) => {
                    app.message_input.push(c);
                }
                _ => {}
            }
        }
        AppState::Loading => {
            // В состоянии загрузки только выход
            if let KeyCode::Char('q') = key.code {
                return Ok(true);
            }
        }
        AppState::Error => {
            // В состоянии ошибки любая клавиша переводит в основное состояние
            app.state = AppState::Main;
            app.error_message.clear();
        }
    }
    
    Ok(false)
} 