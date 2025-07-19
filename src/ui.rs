use ratatui::{
    layout::{Constraint, Direction, Layout, Rect},
    style::{Color, Modifier, Style},
    text::{Line},
    widgets::{Block, Borders, List, ListItem, ListState, Paragraph, Wrap},
    Frame,
};

use crate::app::{App, AppState};

pub fn draw_ui(f: &mut Frame, app: &App) {
    match app.state {
        AppState::Loading => draw_loading_screen(f, app),
        AppState::PhoneInput => draw_phone_input(f, app),
        AppState::CodeInput => draw_code_input(f, app),
        AppState::Main => draw_main_screen(f, app),
        AppState::MessageInput => draw_main_screen(f, app), // Основной экран с полем ввода
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
    
    // Заголовок
    let title = Paragraph::new("Авторизация в Telegram")
        .block(Block::default().borders(Borders::ALL))
        .style(Style::default().fg(Color::Yellow).add_modifier(Modifier::BOLD));
    f.render_widget(title, chunks[0]);
    
    // Основная область
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
    
    // Поле ввода
    let input_text = format!("Номер: {}", app.phone_input);
    let input = Paragraph::new(input_text)
        .block(Block::default().borders(Borders::ALL).title("Ввод"))
        .style(Style::default().fg(Color::Green));
    f.render_widget(input, main_chunks[1]);
    
    // Статус
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
    
    // Заголовок
    let title = Paragraph::new("Код подтверждения")
        .block(Block::default().borders(Borders::ALL))
        .style(Style::default().fg(Color::Yellow).add_modifier(Modifier::BOLD));
    f.render_widget(title, chunks[0]);
    
    // Основная область
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
    
    // Поле ввода
    let input_text = format!("Код: {} ({})", app.code_input, app.code_input.len());
    let input = Paragraph::new(input_text)
        .block(Block::default().borders(Borders::ALL).title("Ввод"))
        .style(Style::default().fg(Color::Green));
    f.render_widget(input, main_chunks[1]);
    
    // Статус
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
    
    // Левая панель - список чатов
    draw_chat_list(f, app, main_chunks[0]);
    
    // Правая панель - сообщения
    draw_messages(f, app, main_chunks[1]);
    
    // Нижняя панель - статус или поле ввода
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
    
    // Создаем область для сообщений с отступами
    let messages_area = area;
    
    // Вычисляем высоту для каждого сообщения
    let message_height = 3; // базовая высота для текстового сообщения
    let image_height = 15; // высота для изображения
    
    let mut y_offset = 0;
    let mut remaining_height = messages_area.height as i32;
    
    // Отображаем сообщения сверху вниз
    for (_i, msg) in app.messages.iter().enumerate() {
        if remaining_height <= 0 {
            break;
        }
        
        let timestamp = msg.timestamp.split('T').next().unwrap_or(&msg.timestamp);
        let time = timestamp.split(' ').last().unwrap_or(timestamp);
        
        let current_height = if msg.r#type == "photo" { image_height } else { message_height };
        
        if remaining_height < current_height {
            break;
        }
        
        let message_area = Rect {
            x: messages_area.x,
            y: messages_area.y + y_offset as u16,
            width: messages_area.width,
            height: current_height as u16,
        };
        
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
                    .block(Block::default().borders(Borders::ALL).style(Style::default().fg(Color::Gray)))
                    .wrap(Wrap { trim: true });
                
                f.render_widget(text_widget, message_area);
            }
            "photo" => {
                // Вычисляем точную ширину для текста
                let text_content = format!("{} {}:", time, msg.from);
                let text_width = text_content.len() as u16 + 2; // +2 для небольшого отступа
                
                // Текстовая часть сообщения с изображением (слева)
                let text_area = Rect {
                    x: message_area.x + 1,
                    y: message_area.y + 1,
                    width: text_width.min(message_area.width / 2), // ограничиваем максимальную ширину
                    height: message_area.height - 2,
                };
                
                let text_widget = Paragraph::new(text_content)
                    .style(Style::default().fg(Color::Yellow))
                    .wrap(Wrap { trim: true });
                
                f.render_widget(text_widget, text_area);
                
                // Изображение (справа, сразу после текста)
                let image_area = Rect {
                    x: text_area.x + text_area.width + 1, // уменьшаем отступ
                    y: message_area.y + 1,
                    width: message_area.width - text_area.width - 3, // корректируем ширину
                    height: message_area.height - 2,
                };
                
                // Отображаем изображение если есть ID
                if let Some(image_id) = msg.image_id {
                    if let Some(image_path) = &msg.image_path {
                        // Проверяем, существует ли файл
                        if std::path::Path::new(image_path).exists() {
                            let placeholder = Paragraph::new("[📷 Изображение]")
                                .style(Style::default().fg(Color::Green));
                            f.render_widget(placeholder, image_area);
                        } else {
                            let placeholder = Paragraph::new("[📷 Загрузка...]")
                                .style(Style::default().fg(Color::Yellow));
                            f.render_widget(placeholder, image_area);
                        }
                    } else {
                        let placeholder = Paragraph::new("[📷 Скачивание...]")
                            .style(Style::default().fg(Color::Blue));
                        f.render_widget(placeholder, image_area);
                    }
                } else {
                    let placeholder = Paragraph::new("[📷 Ошибка]")
                        .style(Style::default().fg(Color::Red));
                    f.render_widget(placeholder, image_area);
                }
                
                // Общая граница для всего сообщения
                let message_block = Block::default()
                    .borders(Borders::ALL)
                    .style(Style::default().fg(Color::Gray));
                f.render_widget(message_block, message_area);
            }
            _ => {
                // Обычное текстовое сообщение
                let text_content = format!("{} {}: {}", time, msg.from, msg.text);
                let text_widget = Paragraph::new(text_content)
                    .style(Style::default().fg(Color::White))
                    .block(Block::default().borders(Borders::ALL).style(Style::default().fg(Color::Gray)))
                    .wrap(Wrap { trim: true });
                
                f.render_widget(text_widget, message_area);
            }
        }
        
        y_offset += current_height;
        remaining_height -= current_height;
    }
    
    // Граница для области сообщений
    let messages_block = Block::default()
        .title(title)
        .borders(Borders::ALL)
        .style(Style::default().fg(Color::White));
    f.render_widget(messages_block, area);
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
    
    // Заголовок
    let title = Paragraph::new("Ошибка")
        .block(Block::default().borders(Borders::ALL))
        .style(Style::default().fg(Color::Red).add_modifier(Modifier::BOLD));
    f.render_widget(title, chunks[0]);
    
    // Сообщение об ошибке
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
    
    // Статус
    let status = Paragraph::new("Любая клавиша: продолжить | q: выход")
        .style(Style::default().fg(Color::Gray));
    f.render_widget(status, chunks[2]);
} 