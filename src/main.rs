use anyhow::Result;
use crossterm::event::{Event, KeyCode, KeyEvent, KeyEventKind};
use crossterm::terminal::{disable_raw_mode, enable_raw_mode, EnterAlternateScreen, LeaveAlternateScreen};
use crossterm::ExecutableCommand;
use ratatui::prelude::*;
use serde::{Deserialize, Serialize};
use std::io::{stdout, Write};
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
    pub image_id: Option<i64>,
    pub image_path: Option<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct AuthStatus {
    pub authorized: bool,
    pub phone_number: Option<String>,
    pub needs_code: bool,
}



// Функция для отображения изображений через Kitty терминал
fn display_images_kitty(app: &App) -> Result<()> {
    let mut stdout = stdout();
    
    // Проверяем, поддерживает ли терминал Kitty graphics
    if std::env::var("TERM").unwrap_or_default() != "xterm-kitty" {
        return Ok(());
    }
    
    // Очищаем все предыдущие изображения
    if let Err(_) = write!(stdout, "\x1b_Ga=d,d=A\x1b\\") {
        return Ok(());
    }
    
    // Небольшая задержка после очистки
    std::thread::sleep(std::time::Duration::from_millis(10));
    
    // Отображаем изображения из сообщений с уникальными ID
    for (index, msg) in app.messages.iter().enumerate() {
        if msg.r#type == "photo" {
            if let Some(image_path) = &msg.image_path {
                // Проверяем, что файл существует и не пустой
                if let Ok(metadata) = std::fs::metadata(image_path) {
                    if metadata.len() > 0 {
                        // Проверяем, что файл действительно является изображением
                        if is_valid_image_file(image_path) {
                            if let Ok(absolute_path) = std::fs::canonicalize(image_path) {
                                let path_str = absolute_path.to_string_lossy();
                                
                                // Используем уникальный ID для каждого изображения (начиная с 100)
                                let image_id = 100 + index as u32;
                                
                                // Определяем формат изображения для Kitty
                                let format = get_image_format(image_path);
                                
                                // Отображаем изображение с правильным форматом
                                if let Err(_) = write!(
                                    stdout,
                                    "\x1b_Gf={},i={},s=200,v=200;{}\x1b\\",
                                    format, image_id, path_str
                                ) {
                                    continue;
                                }
                                
                                // Небольшая задержка между изображениями
                                std::thread::sleep(std::time::Duration::from_millis(5));
                            }
                        }
                    }
                }
            }
        }
    }
    
    if let Err(_) = stdout.flush() {
        // Игнорируем ошибки flush
    }
    
    Ok(())
}

// Функция для очистки старых поврежденных файлов изображений
fn cleanup_corrupted_images() {
    let tmp_dir = "/tmp";
    if let Ok(entries) = std::fs::read_dir(tmp_dir) {
        for entry in entries {
            if let Ok(entry) = entry {
                let path = entry.path();
                if let Some(file_name) = path.file_name() {
                    if let Some(name_str) = file_name.to_str() {
                        if name_str.starts_with("vi-tg_image_") {
                            let path_str = path.to_str().unwrap();
                            
                            // Проверяем файлы с различными расширениями изображений
                            if name_str.ends_with(".png") || name_str.ends_with(".jpg") || 
                               name_str.ends_with(".jpeg") || name_str.ends_with(".webp") || 
                               name_str.ends_with(".gif") {
                                // Простая проверка размера файла
                                if let Ok(metadata) = std::fs::metadata(path_str) {
                                    if metadata.len() < 100 {
                                        let _ = std::fs::remove_file(&path);
                                        continue;
                                    }
                                }
                                
                                // Проверяем, что файл действительно является изображением
                                if !is_valid_image_file(path_str) {
                                    let _ = std::fs::remove_file(&path);
                                }
                            }
                        }
                    }
                }
            }
        }
    }
}

#[tokio::main]
async fn main() -> Result<()> {
    env_logger::init();
    
    // Очищаем старые поврежденные файлы
    cleanup_corrupted_images();
    
    let api_client = ApiClient::new("http://localhost:8080".to_string());
    
    let mut app = App::new(api_client);

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
        
        // Отображаем изображения через Kitty после отрисовки UI
        display_images_kitty(app)?;
        
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

// Функция для проверки, является ли файл валидным PNG
fn is_valid_image_file(file_path: &str) -> bool {
    if let Ok(mut file) = std::fs::File::open(file_path) {
        let mut header = [0u8; 12];
        if let Ok(_) = std::io::Read::read_exact(&mut file, &mut header) {
            // Проверяем различные форматы изображений
            if header.len() >= 2 {
                // JPEG: начинается с 0xFF 0xD8
                if header[0] == 0xFF && header[1] == 0xD8 {
                    return true;
                }
            }
            
            if header.len() >= 8 {
                // PNG: начинается с 0x89 0x50 0x4E 0x47 0x0D 0x0A 0x1A 0x0A
                if header[0] == 0x89 && header[1] == 0x50 && header[2] == 0x4E && header[3] == 0x47 &&
                   header[4] == 0x0D && header[5] == 0x0A && header[6] == 0x1A && header[7] == 0x0A {
                    return true;
                }
            }
            
            if header.len() >= 4 {
                // GIF: начинается с "GIF8"
                if header[0] == 0x47 && header[1] == 0x49 && header[2] == 0x46 && header[3] == 0x38 {
                    return true;
                }
            }
            
            if header.len() >= 12 {
                // WebP: начинается с "RIFF" и содержит "WEBP"
                if header[0] == 0x52 && header[1] == 0x49 && header[2] == 0x46 && header[3] == 0x46 &&
                   header[8] == 0x57 && header[9] == 0x45 && header[10] == 0x42 && header[11] == 0x50 {
                    return true;
                }
            }
        }
    }
    false
}

// Функция для определения формата изображения
fn get_image_format(file_path: &str) -> &'static str {
    if let Ok(mut file) = std::fs::File::open(file_path) {
        let mut header = [0u8; 12];
        if let Ok(_) = std::io::Read::read_exact(&mut file, &mut header) {
            if header.len() >= 2 {
                // JPEG: начинается с 0xFF 0xD8
                if header[0] == 0xFF && header[1] == 0xD8 {
                    return "jpeg";
                }
            }
            
            if header.len() >= 8 {
                // PNG: начинается с 0x89 0x50 0x4E 0x47 0x0D 0x0A 0x1A 0x0A
                if header[0] == 0x89 && header[1] == 0x50 && header[2] == 0x4E && header[3] == 0x47 &&
                   header[4] == 0x0D && header[5] == 0x0A && header[6] == 0x1A && header[7] == 0x0A {
                    return "png";
                }
            }
            
            if header.len() >= 4 {
                // GIF: начинается с "GIF8"
                if header[0] == 0x47 && header[1] == 0x49 && header[2] == 0x46 && header[3] == 0x38 {
                    return "gif";
                }
            }
            
            if header.len() >= 12 {
                // WebP: начинается с "RIFF" и содержит "WEBP"
                if header[0] == 0x52 && header[1] == 0x49 && header[2] == 0x46 && header[3] == 0x46 &&
                   header[8] == 0x57 && header[9] == 0x45 && header[10] == 0x42 && header[11] == 0x50 {
                    return "webp";
                }
            }
        }
    }
    "png" // fallback
}
