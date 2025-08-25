use ratatui::{
    layout::{Constraint, Direction, Layout, Rect},
    style::{Color, Modifier, Style},
    text::{Line},
    widgets::{Block, Borders, List, ListItem, ListState, Paragraph, Wrap, Clear},
    Frame,
};
use ratatui_image::{picker::Picker, protocol::StatefulProtocol, StatefulImage};

use crate::app::{App, AppState};

pub fn draw_ui(f: &mut Frame, app: &mut App) {
    match app.state {
        AppState::Loading => draw_loading_screen(f, app),
        AppState::PhoneInput => draw_phone_input(f, app),
        AppState::CodeInput => draw_code_input(f, app),
        AppState::Main => draw_main_screen(f, app),
        AppState::MessageInput => draw_main_screen(f, app),
        AppState::Error => draw_error_screen(f, app),
        AppState::ImagePreview => draw_image_preview(f, app),
        AppState::VideoPreview => draw_video_preview(f, app),
    }
}

fn draw_loading_screen(f: &mut Frame, _app: &mut App) {
    let area = f.area();

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
    let area = f.area();

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
    let area = f.area();

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

fn draw_main_screen(f: &mut Frame, app: &mut App) {
    let area = f.area();

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
            
            let mut style = if i == app.selected_chat_index {
                Style::default().fg(Color::Yellow).add_modifier(Modifier::BOLD)
            } else {
                Style::default().fg(Color::White)
            };
            if !app.focus_on_messages && i == app.selected_chat_index {
                style = style.bg(Color::Blue);
            }
            
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

fn draw_messages(f: &mut Frame, app: &mut App, area: Rect) {
    let title = app.get_current_chat_title();

    let inner_area = Rect {
        x: area.x + 1,
        y: area.y + 1,
        width: area.width.saturating_sub(2),
        height: area.height.saturating_sub(2),
    };

    let message_height = 1; // базовая высота для сообщения
    let image_height = 12; // высота для изображения

    let picker = match Picker::from_query_stdio() {
        Ok(p) => Some(p),
        Err(_) => None,
    };

    // Умная логика прокрутки с учетом изображений
    let mut start_index = 0;
    if app.selected_message_index < app.messages.len() {
        let visible_height = inner_area.height as usize;

        // Проверяем, является ли выбранное сообщение изображением или видео
        let selected_msg = &app.messages[app.selected_message_index];
        let is_image_selected = app.focus_on_messages && selected_msg.r#type == "photo";
        let is_video_selected = app.focus_on_messages && selected_msg.r#type == "video";

        // Проверяем, находится ли изображение в последних 12 строках
        let last_message_index = app.messages.len().saturating_sub(1);
        let last_12_messages_start = last_message_index.saturating_sub(11); // 12 строк от конца

        if (is_image_selected || is_video_selected) && app.selected_message_index >= last_12_messages_start {
            // Для изображений и видео в последних 12 строках: прокручиваем на 11 строк вниз
            let base_start = app.messages.len().saturating_sub(visible_height);
            start_index = base_start + 11; // Прокрутка на 11 строк
            start_index = start_index.min(app.messages.len().saturating_sub(1));
        } else {
            // Для обычных сообщений или изображений не в последних 12 строках: обычная логика
            start_index = app.messages.len().saturating_sub(visible_height);

            // Определяем диапазон, в котором маркер может перемещаться без прокрутки
            let cursor_range_start = last_message_index.saturating_sub(10);

            // Если маркер в диапазоне последних 10 сообщений - не прокручиваем
            if app.selected_message_index >= cursor_range_start {
                start_index = app.messages.len().saturating_sub(visible_height);
            } else {
                // Маркер вышел за диапазон - прокручиваем, но сохраняем зазор в 10 сообщений
                let adjusted_selected = app.selected_message_index + 10;
                if adjusted_selected < app.messages.len() {
                    start_index = adjusted_selected.saturating_sub(visible_height / 2);
                    start_index = start_index.min(app.messages.len().saturating_sub(visible_height));
                }
            }
        }

        // Убеждаемся, что не выходим за границы
        start_index = start_index.min(app.messages.len().saturating_sub(1));
    }

    let mut y_offset = 0i32;
    let available_height = inner_area.height as i32;

    // Начинаем с рассчитанного индекса
    let mut index = start_index;
    while index < app.messages.len() && y_offset < available_height {
        let msg = &app.messages[index];
        let is_selected = app.focus_on_messages && index == app.selected_message_index;
        let current_height = if msg.r#type == "photo" || msg.r#type == "video" {
            if is_selected { image_height } else { message_height }
        } else { message_height };

        // Проверяем, что область сообщения не выходит за границы
        let max_available_height = (inner_area.y as i32 + inner_area.height as i32 - y_offset) as u16;
        let safe_height = current_height.min(max_available_height);

        let message_area = Rect {
            x: inner_area.x,
            y: inner_area.y + y_offset as u16,
            width: inner_area.width,
            height: safe_height,
        };

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
                if is_selected {
                    draw_photo_message(f, msg, message_area, time, picker.as_ref());
                } else {
                    let label = "[📷 Фото — Enter: открыть]";
                    let text_content = format!("{} {}: {}", time, msg.from, label);
                    let text_widget = Paragraph::new(text_content)
                        .style(Style::default().fg(Color::Cyan))
                        .wrap(Wrap { trim: true });
                    f.render_widget(text_widget, message_area);
                }
            }
            "video" => {
                if is_selected {
                    draw_video_message(f, msg, message_area, time, picker.as_ref());
                } else {
                    let label = "[🎬 Видео — Enter: открыть]";
                    let text_content = format!("{} {}: {}", time, msg.from, label);
                    let text_widget = Paragraph::new(text_content)
                        .style(Style::default().fg(Color::Green))
                        .wrap(Wrap { trim: true });
                    f.render_widget(text_widget, message_area);
                }
            }
            _ => {
                let text_content = format!("{} {}: {}", time, msg.from, msg.text);
                let text_widget = Paragraph::new(text_content)
                    .style(Style::default().fg(Color::White))
                    .wrap(Wrap { trim: true });
                f.render_widget(text_widget, message_area);
            }
        }

        // Индикатор выбора
        if is_selected {
            let indicator = Rect { x: message_area.x, y: message_area.y, width: 1, height: message_area.height };
            let block = Block::default().style(Style::default().bg(Color::Blue));
            f.render_widget(block, indicator);
        }

        y_offset += current_height as i32;
        index += 1;
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

    // Проверяем, достаточно ли места для отображения текста сверху
    let has_space_for_text = inner_area.height > 1;

    let _text_area = if has_space_for_text {
        // Метаданные сверху
        let text_rect = Rect {
            x: inner_area.x,
            y: inner_area.y,
            width: inner_area.width,
            height: 1,
        };

        let text_content = format!("{} {}:", time, msg.from);
        let text_widget = Paragraph::new(text_content)
            .style(Style::default().fg(Color::Yellow));
        f.render_widget(text_widget, text_rect);

        text_rect
    } else {
        // Если нет места для текста, возвращаем пустую область
        Rect::default()
    };

    // Изображение снизу от метаданных
    let image_area = Rect {
        x: inner_area.x,
        y: if has_space_for_text { inner_area.y + 1 } else { inner_area.y },
        width: inner_area.width,
        height: if has_space_for_text { inner_area.height.saturating_sub(1) } else { inner_area.height },
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

fn try_display_image(image_path: &str, picker: &Picker, _area: Rect) -> Result<StatefulProtocol, String> {
    if !std::path::Path::new(image_path).exists() {
        return Err(format!("файл не найден: {}", image_path));
    }

    let metadata = std::fs::metadata(image_path)
        .map_err(|e| format!("не удалось получить метаданные: {}", e))?;

    if metadata.len() < 100 {
        return Err(format!("файл слишком мал: {} байт", metadata.len()));
    }

    // Проверяем, что файл не пустой и читаемый
    let file = std::fs::File::open(image_path)
        .map_err(|e| format!("не удалось открыть файл: {}", e))?;

    let dyn_img = image::open(image_path)
        .map_err(|e| format!("не удалось открыть изображение: {} (путь: {})", e, image_path))?;

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
    let area = f.area();

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

fn draw_image_preview(f: &mut Frame, app: &App) {
    let area = f.area();

    // Чёрный фон на весь экран
    let overlay = Block::default().style(Style::default().bg(Color::Black));
    f.render_widget(Clear, area); // очистка
    f.render_widget(overlay, area);

    // Рисуем изображение, если путь есть
    if let Some(path) = &app.preview_image_path {
        let inner = Rect { x: area.x + 1, y: area.y + 1, width: area.width.saturating_sub(2), height: area.height.saturating_sub(4) };
        if let Ok(picker) = Picker::from_query_stdio() {
            match try_display_image_full(path, &picker) {
                Ok(mut protocol) => {
                    let widget = StatefulImage::new();
                    f.render_stateful_widget(widget, inner, &mut protocol);
                }
                Err(e) => {
                    let text = Paragraph::new(format!("Не удалось отобразить изображение: {}", e))
                        .style(Style::default().fg(Color::Red))
                        .wrap(Wrap { trim: true });
                    f.render_widget(text, inner);
                }
            }
        } else {
            let text = Paragraph::new("Терминал не поддерживает отрисовку изображений")
                .style(Style::default().fg(Color::Yellow))
                .wrap(Wrap { trim: true });
            f.render_widget(text, inner);
        }
    }

    // Нижняя подсказка - зависит от типа превью
    let (hint_text, title) = if let Some(video_path) = &app.preview_video_path {
        if !video_path.is_empty() {
            // Это видео превью
            ("Enter: воспроизвести в mpv | Esc: назад", "Превью видео")
        } else {
            // Это обычное изображение
            ("Esc/Enter: выйти из просмотра", "Просмотр изображения")
        }
    } else {
        // Это обычное изображение
        ("Esc/Enter: выйти из просмотра", "Просмотр изображения")
    };

    let hint = Paragraph::new(hint_text)
        .style(Style::default().fg(Color::Gray))
        .block(Block::default().borders(Borders::ALL).title(title));
    let hint_area = Rect { x: area.x + 2, y: area.y + area.height.saturating_sub(3), width: area.width.saturating_sub(4), height: 3 };
    f.render_widget(hint, hint_area);
}

fn try_display_image_full(image_path: &str, picker: &Picker) -> Result<StatefulProtocol, String> {
    if !std::path::Path::new(image_path).exists() {
        return Err(format!("файл не найден: {}", image_path));
    }

    // Проверяем размер файла
    let metadata = std::fs::metadata(image_path)
        .map_err(|e| format!("не удалось получить метаданные: {}", e))?;

    if metadata.len() < 100 {
        return Err(format!("файл слишком мал: {} байт (путь: {})", metadata.len(), image_path));
    }

    // Проверяем, что файл читаем
    let _file = std::fs::File::open(image_path)
        .map_err(|e| format!("не удалось открыть файл: {} (путь: {})", e, image_path))?;

    // Пытаемся определить формат по первым байтам
    if let Ok(header) = std::fs::read(image_path) {
        if header.is_empty() || header.len() < 4 {
            return Err(format!("файл пустой или слишком мал для определения формата (путь: {})", image_path));
        }

        // Проверяем магические байты различных форматов
        let is_jpeg = header.len() >= 2 && header[0] == 0xFF && header[1] == 0xD8;
        let is_png = header.len() >= 8 && header[0] == 0x89 && header[1] == 0x50 && header[2] == 0x4E && header[3] == 0x47;
        let is_gif = header.len() >= 4 && header[0] == 0x47 && header[1] == 0x49 && header[2] == 0x46 && header[3] == 0x38;

        if !is_jpeg && !is_png && !is_gif {
            return Err(format!("неподдерживаемый формат файла (путь: {}). Поддерживаемые: JPEG, PNG, GIF", image_path));
        }
    }

    let dyn_img = image::open(image_path)
        .map_err(|e| format!("не удалось открыть изображение: {} (путь: {})", e, image_path))?;

    Ok(picker.new_resize_protocol(dyn_img))
}

fn draw_video_message(f: &mut Frame, msg: &crate::Message, area: Rect, time: &str, picker: Option<&Picker>) {
    let inner_area = Rect {
        x: area.x + 1,
        y: area.y + 1,
        width: area.width.saturating_sub(2),
        height: area.height.saturating_sub(2),
    };

    // Проверяем, достаточно ли места для отображения текста сверху
    let has_space_for_text = inner_area.height > 1;

    let _text_area = if has_space_for_text {
        // Метаданные сверху
        let text_rect = Rect {
            x: inner_area.x,
            y: inner_area.y,
            width: inner_area.width,
            height: 1,
        };

        let text_content = format!("{} {}:", time, msg.from);
        let text_widget = Paragraph::new(text_content)
            .style(Style::default().fg(Color::Yellow));
        f.render_widget(text_widget, text_rect);

        text_rect
    } else {
        // Если нет места для текста, возвращаем пустую область
        Rect::default()
    };

    // Превью видео снизу от метаданных
    let preview_area = Rect {
        x: inner_area.x,
        y: if has_space_for_text { inner_area.y + 1 } else { inner_area.y },
        width: inner_area.width,
        height: if has_space_for_text { inner_area.height.saturating_sub(1) } else { inner_area.height },
    };

    if let Some(preview_path) = &msg.video_preview_path {
        if let Some(picker) = picker {
            match try_display_image(preview_path, picker, preview_area) {
                Ok(mut protocol) => {
                    let image_widget = StatefulImage::new();
                    f.render_stateful_widget(image_widget, preview_area, &mut protocol);
                }
                Err(e) => {
                    let error_text = format!("[🎬 Ошибка превью: {}]", e);
                    let error_widget = Paragraph::new(error_text)
                        .style(Style::default().fg(Color::Red));
                    f.render_widget(error_widget, preview_area);
                }
            }
        } else {
            let placeholder = Paragraph::new("[🎬 Терминал не поддерживает изображения]")
                .style(Style::default().fg(Color::Yellow));
            f.render_widget(placeholder, preview_area);
        }
    } else {
        let placeholder = Paragraph::new("[🎬 Загрузка превью...]")
            .style(Style::default().fg(Color::Blue));
        f.render_widget(placeholder, preview_area);
    }

    let message_block = Block::default();
    f.render_widget(message_block, area);
}

fn draw_video_preview(f: &mut Frame, app: &App) {
    let area = f.area();

    // Чёрный фон на весь экран
    let overlay = Block::default().style(Style::default().bg(Color::Black));
    f.render_widget(Clear, area); // очистка
    f.render_widget(overlay, area);

    // Рисуем превью видео, если путь есть
    if let Some(preview_path) = &app.preview_video_path {
        let inner = Rect { x: area.x + 1, y: area.y + 1, width: area.width.saturating_sub(2), height: area.height.saturating_sub(4) };
        if let Ok(picker) = Picker::from_query_stdio() {
            match try_display_image_full(preview_path, &picker) {
                Ok(mut protocol) => {
                    let widget = StatefulImage::new();
                    f.render_stateful_widget(widget, inner, &mut protocol);
                }
                Err(e) => {
                    let text = Paragraph::new(format!("Не удалось отобразить превью видео: {}", e))
                        .style(Style::default().fg(Color::Red))
                        .wrap(Wrap { trim: true });
                    f.render_widget(text, inner);
                }
            }
        } else {
            let text = Paragraph::new("Терминал не поддерживает отрисовку изображений")
                .style(Style::default().fg(Color::Yellow))
                .wrap(Wrap { trim: true });
            f.render_widget(text, inner);
        }
    }

    // Нижняя подсказка
    let hint = Paragraph::new("Enter: воспроизвести в mpv | Esc: назад")
        .style(Style::default().fg(Color::Gray))
        .block(Block::default().borders(Borders::ALL).title("Превью видео"));
    let hint_area = Rect { x: area.x + 2, y: area.y + area.height.saturating_sub(3), width: area.width.saturating_sub(4), height: 3 };
    f.render_widget(hint, hint_area);
}
