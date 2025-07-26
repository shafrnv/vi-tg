use ratatui::{
    layout::{Constraint, Direction, Layout, Rect},
    style::{Color, Modifier, Style},
    text::{Line},
    widgets::{Block, Borders, List, ListItem, ListState, Paragraph, Wrap},
    Frame,
};
use ratatui_image::{picker::Picker, protocol::StatefulProtocol, StatefulImage};

use crate::app::{App, AppState};

pub fn draw_ui(f: &mut Frame, app: &App) {
    match app.state {
        AppState::Loading => draw_loading_screen(f, app),
        AppState::PhoneInput => draw_phone_input(f, app),
        AppState::CodeInput => draw_code_input(f, app),
        AppState::Main => draw_main_screen(f, app),
        AppState::MessageInput => draw_main_screen(f, app),
        AppState::Error => draw_error_screen(f, app),
    }
}

fn draw_loading_screen(f: &mut Frame, _app: &App) {
    let area = f.size();
    
    let block = Block::default()
        .title("vi-tg")
        .borders(Borders::ALL)
        .style(Style::default().fg(Color::White));
    
    let text = vec![
        Line::from(""),
        Line::from("Загрузка..."),
        Line::from(""),
        Line::from("Проверка авторизации..."),
    ];
    
    let paragraph = Paragraph::new(text)
        .block(block)
        .wrap(Wrap { trim: true });
    
    f.render_widget(paragraph, area);
}

fn draw_phone_input(f: &mut Frame, app: &App) {
    let area = f.size();
    
    let chunks = Layout::default()
        .direction(Direction::Vertical)
        .constraints([
            Constraint::Length(3),
            Constraint::Min(0),
            Constraint::Length(3),
        ])
        .split(area);
    
    let title = Paragraph::new("Авторизация в Telegram")
        .block(Block::default().borders(Borders::ALL))
        .style(Style::default().fg(Color::Yellow).add_modifier(Modifier::BOLD));
    f.render_widget(title, chunks[0]);
    
    let main_chunks = Layout::default()
        .direction(Direction::Vertical)
        .constraints([
            Constraint::Length(1),
            Constraint::Length(3),
            Constraint::Length(1),
            Constraint::Min(0),
        ])
        .split(chunks[1]);
    
    let instruction = Paragraph::new("Введите номер телефона с кодом страны (например: +7 999 123 45 67):")
        .style(Style::default().fg(Color::White));
    f.render_widget(instruction, main_chunks[0]);
    
    let input_text = format!("Номер: {}", app.phone_input);
    let input = Paragraph::new(input_text)
        .block(Block::default().borders(Borders::ALL).title("Ввод"))
        .style(Style::default().fg(Color::Green));
    f.render_widget(input, main_chunks[1]);
    
    let status = Paragraph::new("Enter: подтвердить | Esc: выход")
        .style(Style::default().fg(Color::Gray));
    f.render_widget(status, chunks[2]);
}

fn draw_code_input(f: &mut Frame, app: &App) {
    let area = f.size();
    
    let chunks = Layout::default()
        .direction(Direction::Vertical)
        .constraints([
            Constraint::Length(3),
            Constraint::Min(0),
            Constraint::Length(3),
        ])
        .split(area);
    
    let title = Paragraph::new("Код подтверждения")
        .block(Block::default().borders(Borders::ALL))
        .style(Style::default().fg(Color::Yellow).add_modifier(Modifier::BOLD));
    f.render_widget(title, chunks[0]);
    
    let main_chunks = Layout::default()
        .direction(Direction::Vertical)
        .constraints([
            Constraint::Length(1),
            Constraint::Length(3),
            Constraint::Length(1),
            Constraint::Min(0),
        ])
        .split(chunks[1]);
    
    let instruction = Paragraph::new("Введите код, который был отправлен на ваш номер телефона:")
        .style(Style::default().fg(Color::White));
    f.render_widget(instruction, main_chunks[0]);
    
    let input_text = format!("Код: {} ({})", app.code_input, app.code_input.len());
    let input = Paragraph::new(input_text)
        .block(Block::default().borders(Borders::ALL).title("Ввод"))
        .style(Style::default().fg(Color::Green));
    f.render_widget(input, main_chunks[1]);
    
    let status = Paragraph::new("Enter: подтвердить | Esc: назад")
        .style(Style::default().fg(Color::Gray));
    f.render_widget(status, chunks[2]);
}

fn draw_main_screen(f: &mut Frame, app: &App) {
    let area = f.size();
    
    let chunks = Layout::default()
        .direction(Direction::Vertical)
        .constraints([
            Constraint::Min(0),
            Constraint::Length(3),
        ])
        .split(area);
    
    let main_chunks = Layout::default()
        .direction(Direction::Horizontal)
        .constraints([
            Constraint::Length(30),
            Constraint::Min(0),
        ])
        .split(chunks[0]);
    
    draw_chat_list(f, app, main_chunks[0]);
    draw_messages(f, app, main_chunks[1]);
    draw_status_bar(f, app, chunks[1]);
}

fn draw_chat_list(f: &mut Frame, app: &App, area: Rect) {
    let items: Vec<ListItem> = app.chats
        .iter()
        .enumerate()
        .map(|(i, chat)| {
            let mut text = chat.title.clone();
            if chat.unread > 0 {
                text = format!("({}) {}", chat.unread, text);
            }
            
            let style = if i == app.selected_chat_index {
                Style::default().fg(Color::Yellow).add_modifier(Modifier::BOLD)
            } else {
                Style::default().fg(Color::White)
            };
            
            ListItem::new(text).style(style)
        })
        .collect();
    
    let list = List::new(items)
        .block(Block::default().borders(Borders::ALL).title("Чаты"))
        .highlight_style(Style::default().fg(Color::Yellow).add_modifier(Modifier::BOLD))
        .highlight_symbol("▶ ");
    
    let mut state = ListState::default();
    state.select(Some(app.selected_chat_index));
    f.render_stateful_widget(list, area, &mut state);
}

fn draw_messages(f: &mut Frame, app: &App, area: Rect) {
    let title = app.get_current_chat_title();
    
    let inner_area = Rect {
        x: area.x + 1,
        y: area.y + 1,
        width: area.width.saturating_sub(2),
        height: area.height.saturating_sub(2),
    };
    
    let message_height = 1; // базовая высота для сообщения
    let image_height = 12; // высота для изображения
    
    let mut y_offset = 0;
    let available_height = inner_area.height as i32;

    let picker = match Picker::from_query_stdio() {
        Ok(p) => Some(p),
        Err(_) => None,
    };
    
    for (i, msg) in app.messages.iter().enumerate() {
        let current_height = if msg.r#type == "photo" { image_height } else { message_height };
        
        if y_offset + current_height > available_height {
            break;
        }
        
        let message_area = Rect {
            x: inner_area.x,
            y: inner_area.y + y_offset as u16,
            width: inner_area.width,
            height: current_height as u16,
        };
        
        let timestamp = msg.timestamp.split('T').next().unwrap_or(&msg.timestamp);
        let time = msg.timestamp.split(' ').last().unwrap_or("00:00");
        
        match msg.r#type.as_str() {
            "sticker" => {
                let sticker_text = if let Some(emoji) = &msg.sticker_emoji {
                    format!("{} [стикер]", emoji)
                } else {
                    "[стикер]".to_string()
                };
                
                let text_content = format!("{} {}: {}", time, msg.from, sticker_text);
                let text_widget = Paragraph::new(text_content)
                    .style(Style::default().fg(Color::Magenta))
                    .wrap(Wrap { trim: true });
                
                f.render_widget(text_widget, message_area);
            }
            "photo" => {
                draw_photo_message(f, msg, message_area, time, picker.as_ref());
            }
            _ => {
                // Обычное текстовое сообщение
                let text_content = format!("{} {}: {}", time, msg.from, msg.text);
                let text_widget = Paragraph::new(text_content)
                    .style(Style::default().fg(Color::White))
                    .wrap(Wrap { trim: true });
                
                f.render_widget(text_widget, message_area);
            }
        }
        
        y_offset += current_height;
    }
    
    // Граница для области сообщений
    let messages_block = Block::default()
        .title(title)
        .borders(Borders::ALL)
        .style(Style::default().fg(Color::White));
    f.render_widget(messages_block, area);
}

fn draw_photo_message(f: &mut Frame, msg: &crate::Message, area: Rect, time: &str, picker: Option<&Picker>) {
    let inner_area = Rect {
        x: area.x + 1,
        y: area.y + 1,
        width: area.width.saturating_sub(2),
        height: area.height.saturating_sub(2),
    };
    let text_content = format!("{} {}:", time, msg.from);
    let text_area = Rect {
        x: inner_area.x,
        y: inner_area.y,
        width: inner_area.width,
        height: 1,
    };
    
    let text_widget = Paragraph::new(text_content)
        .style(Style::default().fg(Color::Yellow));
    f.render_widget(text_widget, text_area);

    let image_area = Rect {
        x: inner_area.x,
        y: inner_area.y + 1,
        width: inner_area.width,
        height: inner_area.height.saturating_sub(1),
    };

    if let Some(image_path) = &msg.image_path {
        if let Some(picker) = picker {
            match try_display_image(image_path, picker, image_area) {
                Ok(mut protocol) => {
                    let image_widget = StatefulImage::new();
                    f.render_stateful_widget(image_widget, image_area, &mut protocol);
                }
                Err(e) => {
                    let error_text = format!("[📷 Ошибка: {}]", e);
                    let error_widget = Paragraph::new(error_text)
                        .style(Style::default().fg(Color::Red));
                    f.render_widget(error_widget, image_area);
                }
            }
        } else {
            let placeholder = Paragraph::new("[📷 Терминал не поддерживает изображения]")
                .style(Style::default().fg(Color::Yellow));
            f.render_widget(placeholder, image_area);
        }
    } else {
        let placeholder = Paragraph::new("[📷 Загрузка...]")
            .style(Style::default().fg(Color::Blue));
        f.render_widget(placeholder, image_area);
    }
    
    let message_block = Block::default();
    f.render_widget(message_block, area);
}

fn try_display_image(image_path: &str, picker: &Picker, area: Rect) -> Result<StatefulProtocol, String> {
    if !std::path::Path::new(image_path).exists() {
        return Err("файл не найден".to_string());
    }
    
    let metadata = std::fs::metadata(image_path)
        .map_err(|_| "не удалось получить метаданные")?;
    
    if metadata.len() < 100 {
        return Err("файл слишком мал".to_string());
    }
    
    let dyn_img = image::open(image_path)
        .map_err(|e| format!("не удалось открыть: {}", e))?;
    
    let protocol = picker.new_resize_protocol(dyn_img);
    
    Ok(protocol)
}

fn draw_status_bar(f: &mut Frame, app: &App, area: Rect) {
    let status_text = if app.state == AppState::MessageInput {
        format!("Сообщение: {}", app.message_input)
    } else {
        app.get_status_text()
    };
    
    let color = match app.state {
        AppState::Error => Color::Red,
        AppState::MessageInput => Color::Green,
        _ => Color::Gray,
    };
    
    let status = Paragraph::new(status_text)
        .block(Block::default().borders(Borders::ALL).title("Статус"))
        .style(Style::default().fg(color))
        .wrap(Wrap { trim: true });
    
    f.render_widget(status, area);
}

fn draw_error_screen(f: &mut Frame, app: &App) {
    let area = f.size();
    
    let chunks = Layout::default()
        .direction(Direction::Vertical)
        .constraints([
            Constraint::Length(3),
            Constraint::Min(0),
            Constraint::Length(3),
        ])
        .split(area);
    
    let title = Paragraph::new("Ошибка")
        .block(Block::default().borders(Borders::ALL))
        .style(Style::default().fg(Color::Red).add_modifier(Modifier::BOLD));
    f.render_widget(title, chunks[0]);
    
    let error_text = vec![
        Line::from(""),
        Line::from(app.error_message.clone()),
        Line::from(""),
        Line::from("Нажмите любую клавишу для продолжения..."),
    ];
    
    let error_msg = Paragraph::new(error_text)
        .block(Block::default().borders(Borders::ALL).title("Подробности"))
        .style(Style::default().fg(Color::Red))
        .wrap(Wrap { trim: true });
    
    f.render_widget(error_msg, chunks[1]);
    
    let status = Paragraph::new("Любая клавиша: продолжить | q: выход")
        .style(Style::default().fg(Color::Gray));
    f.render_widget(status, chunks[2]);
}