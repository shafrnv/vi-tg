use anyhow::Result;
use serde::{Deserialize, Serialize};

mod api;
mod app;
mod ui;

use api::ApiClient;
use app::{App, AppState};
use ui as ui_module;

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
    
    let app = App::new(api_client);

    run_tui(app).await?;
    
    Ok(())
}

async fn run_tui(mut app: App) -> Result<()> {
    // Настройка терминала
    crossterm::terminal::enable_raw_mode()?;
    let mut stdout = std::io::stdout();
    crossterm::execute!(stdout, crossterm::terminal::EnterAlternateScreen)?;
    let backend = ratatui::backend::CrosstermBackend::new(stdout);
    let mut terminal = ratatui::Terminal::new(backend)?;

    loop {
        terminal.draw(|frame| ui_module::draw_ui(frame, &mut app))?;

        // Обработка событий
        if crossterm::event::poll(std::time::Duration::from_millis(100))? {
            if let crossterm::event::Event::Key(key) = crossterm::event::read()? {
                match key.code {
                    crossterm::event::KeyCode::Char('q') => break,
                    crossterm::event::KeyCode::Tab => {
                        app.toggle_focus();
                    }
                    crossterm::event::KeyCode::Up => {
                        if app.focus_on_messages {
                            app.move_message_selection(-1, app.calculate_visible_capacity());
                        } else {
                            app.move_chat_selection(-1);
                        }
                    }
                    crossterm::event::KeyCode::Down => {
                        if app.focus_on_messages {
                            app.move_message_selection(1, app.calculate_visible_capacity());
                        } else {
                            app.move_chat_selection(1);
                        }
                    }
                    crossterm::event::KeyCode::Char('r') => {
                        if let Err(e) = app.refresh_data().await {
                            app.show_error(&format!("Ошибка обновления: {}", e));
                        }
                    }
                    crossterm::event::KeyCode::Char('i') => {
                        if app.state == AppState::Main {
                            app.state = AppState::MessageInput;
                        }
                    }
                    crossterm::event::KeyCode::Enter => {
                        match app.state {
                            AppState::Main => {
                                if app.focus_on_messages {
                                    app.open_selected_message();
                                } else {
                                    if let Err(e) = app.select_chat().await {
                                        app.show_error(&format!("Ошибка выбора чата: {}", e));
                                    }
                                    // после выбора чата переводим фокус на сообщения
                                    app.focus_messages();
                                }
                            }
                            AppState::MessageInput => {
                                if let Err(e) = app.send_message().await {
                                    app.show_error(&format!("Ошибка отправки: {}", e));
                                }
                                app.state = AppState::Main;
                            }
                            AppState::PhoneInput => {
                                if let Err(e) = app.set_phone_number().await {
                                    app.show_error(&format!("Ошибка установки номера: {}", e));
                                }
                            }
                            AppState::CodeInput => {
                                if let Err(e) = app.send_code().await {
                                    app.show_error(&format!("Ошибка отправки кода: {}", e));
                                }
                            }
                            AppState::ImagePreview => {
                                app.close_image_preview();
                            }
                            _ => {}
                        }
                    }
                    crossterm::event::KeyCode::Esc => {
                        if app.state == AppState::MessageInput {
                            app.state = AppState::Main;
                            app.message_input.clear();
                        } else if app.state == AppState::Main {
                            // Esc возвращает фокус на список чатов
                            app.focus_chats();
                        } else if app.state == AppState::ImagePreview {
                            app.close_image_preview();
                        }
                    }
                    crossterm::event::KeyCode::Char(c) => {
                        match app.state {
                            AppState::PhoneInput => app.phone_input.push(c),
                            AppState::CodeInput => app.code_input.push(c),
                            AppState::MessageInput => app.message_input.push(c),
                            _ => {}
                        }
                    }
                    crossterm::event::KeyCode::Backspace => {
                        match app.state {
                            AppState::PhoneInput => { app.phone_input.pop(); }
                            AppState::CodeInput => { app.code_input.pop(); }
                            AppState::MessageInput => { app.message_input.pop(); }
                            _ => {}
                        }
                    }
                    _ => {}
                }
            }
        }

        // Обновление данных
        if let Err(e) = app.update().await {
            app.show_error(&format!("Ошибка обновления: {}", e));
        }
    }

    // Восстановление терминала
    crossterm::terminal::disable_raw_mode()?;
    crossterm::execute!(
        std::io::stdout(),
        crossterm::terminal::LeaveAlternateScreen
    )?;

    Ok(())
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
