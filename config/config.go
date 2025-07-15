package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Config struct {
	TelegramToken string `json:"telegram_token"`
	PhoneNumber   string `json:"phone_number"`
	UseMTProto    bool   `json:"use_mtproto"`
	Theme         string `json:"theme"`
	AutoSave      bool   `json:"auto_save"`
}

func LoadConfig() (*Config, error) {
	configPath := getConfigPath()
	
	// Создаем директорию если не существует
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return nil, fmt.Errorf("ошибка создания директории конфига: %w", err)
	}
	
	// Если файл не существует, создаем с дефолтными значениями
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		config := &Config{
			TelegramToken: "",
			PhoneNumber:   "",
			UseMTProto:    true, // По умолчанию используем MTProto
			Theme:         "default",
			AutoSave:      true,
		}
		
		if err := SaveConfig(config); err != nil {
			return nil, err
		}
		
		return config, nil
	}
	
	// Читаем существующий конфиг
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("ошибка чтения конфига: %w", err)
	}
	
	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("ошибка парсинга конфига: %w", err)
	}
	
	return &config, nil
}

func SaveConfig(config *Config) error {
	configPath := getConfigPath()
	
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("ошибка сериализации конфига: %w", err)
	}
	
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("ошибка записи конфига: %w", err)
	}
	
	return nil
}

func getConfigPath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = "."
	}
	return filepath.Join(homeDir, ".vi-tg", "config.json")
} 