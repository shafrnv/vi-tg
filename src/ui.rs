use ratatui::{
    layout::{Constraint, Direction, Layout, Rect},
    style::{Color, Modifier, Style},
    text::{Line, Span},
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
        .wrap(Wrap { trim: true })
        .alignment(ratatui::layout::Alignment::Center);
    
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
        .style(Style::default().fg(Color::Yellow).add_modifier(Modifier::BOLD))
        .alignment(ratatui::layout::Alignment::Center);
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
        .style(Style::default().fg(Color::White))
        .alignment(ratatui::layout::Alignment::Center);
    f.render_widget(instruction, main_chunks[0]);
    
    // Поле ввода
    let input_text = format!("Номер: {}", app.phone_input);
    let input = Paragraph::new(input_text)
        .block(Block::default().borders(Borders::ALL).title("Ввод"))
        .style(Style::default().fg(Color::Green));
    f.render_widget(input, main_chunks[1]);
    
    // Статус
    let status = Paragraph::new("Enter: подтвердить | Esc: выход")
        .style(Style::default().fg(Color::Gray))
        .alignment(ratatui::layout::Alignment::Center);
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
        .style(Style::default().fg(Color::Yellow).add_modifier(Modifier::BOLD))
        .alignment(ratatui::layout::Alignment::Center);
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
        .style(Style::default().fg(Color::White))
        .alignment(ratatui::layout::Alignment::Center);
    f.render_widget(instruction, main_chunks[0]);
    
    // Поле ввода
    let input_text = format!("Код: {} ({})", app.code_input, app.code_input.len());
    let input = Paragraph::new(input_text)
        .block(Block::default().borders(Borders::ALL).title("Ввод"))
        .style(Style::default().fg(Color::Green));
    f.render_widget(input, main_chunks[1]);
    
    // Статус
    let status = Paragraph::new("Enter: подтвердить | Esc: назад")
        .style(Style::default().fg(Color::Gray))
        .alignment(ratatui::layout::Alignment::Center);
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
    
    let messages_text: Vec<Line> = app.messages
        .iter()
        .map(|msg| {
            let timestamp = msg.timestamp.split('T').next().unwrap_or(&msg.timestamp);
            let time = timestamp.split(' ').last().unwrap_or(timestamp);
            
            match msg.r#type.as_str() {
                "sticker" => {
                    let sticker_text = if let Some(emoji) = &msg.sticker_emoji {
                        format!("{} [стикер]", emoji)
                    } else {
                        "[стикер]".to_string()
                    };
                    
                    Line::from(vec![
                        Span::styled(
                            format!("{:5} {:12}: ", time, msg.from),
                            Style::default().fg(Color::Gray)
                        ),
                        Span::styled(
                            sticker_text,
                            Style::default().fg(Color::Magenta)
                        ),
                    ])
                }
                _ => {
                    Line::from(vec![
                        Span::styled(
                            format!("{:5} {:12}: ", time, msg.from),
                            Style::default().fg(Color::Gray)
                        ),
                        Span::styled(
                            msg.text.clone(),
                            Style::default().fg(Color::White)
                        ),
                    ])
                }
            }
        })
        .collect();
    
    let messages = Paragraph::new(messages_text)
        .block(Block::default().borders(Borders::ALL).title(title))
        .wrap(Wrap { trim: true });
    
    f.render_widget(messages, area);
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
        .style(Style::default().fg(Color::Red).add_modifier(Modifier::BOLD))
        .alignment(ratatui::layout::Alignment::Center);
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
        .alignment(ratatui::layout::Alignment::Center)
        .wrap(Wrap { trim: true });
    
    f.render_widget(error_msg, chunks[1]);
    
    // Статус
    let status = Paragraph::new("Любая клавиша: продолжить | q: выход")
        .style(Style::default().fg(Color::Gray))
        .alignment(ratatui::layout::Alignment::Center);
    f.render_widget(status, chunks[2]);
} 