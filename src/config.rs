use anyhow::Result;
use serde::{Deserialize, Serialize};
use std::fs;
use std::path::PathBuf;

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Config {
    pub api_id: i32,
    pub api_hash: String,
    pub phone_number: Option<String>,
    pub use_tdlib: bool,
    pub theme: String,
    pub auto_save: bool,
}

impl Default for Config {
    fn default() -> Self {
        Self {
            api_id: 0, // Должно быть установлено пользователем
            api_hash: String::new(), // Должно быть установлено пользователем
            phone_number: None,
            use_tdlib: true,
            theme: "default".to_string(),
            auto_save: true,
        }
    }
}

impl Config {
    pub fn load() -> Result<Self> {
        let config_path = Self::get_config_path()?;
        
        // Создаем директорию если не существует
        if let Some(parent) = config_path.parent() {
            fs::create_dir_all(parent)?;
        }
        
        // Если файл не существует, создаем с дефолтными значениями
        if !config_path.exists() {
            let config = Config::default();
            config.save()?;
            return Ok(config);
        }
        
        // Читаем существующий конфиг
        let data = fs::read_to_string(&config_path)?;
        let config: Config = serde_json::from_str(&data)?;
        
        Ok(config)
    }
    
    pub fn save(&self) -> Result<()> {
        let config_path = Self::get_config_path()?;
        
        // Создаем директорию если не существует
        if let Some(parent) = config_path.parent() {
            fs::create_dir_all(parent)?;
        }
        
        let data = serde_json::to_string_pretty(self)?;
        fs::write(&config_path, data)?;
        
        Ok(())
    }
    
    fn get_config_path() -> Result<PathBuf> {
        let home_dir = dirs::home_dir()
            .ok_or_else(|| anyhow::anyhow!("Не удалось найти домашнюю директорию"))?;
        Ok(home_dir.join(".vi-tg").join("config.json"))
    }
} 