package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type ServerConfig struct {
	Name    string `yaml:"name"`
	Host    string `yaml:"host"`
	Port    int    `yaml:"port"`
	WebPort int    `yaml:"web_port"`
	SyncDir string `yaml:"sync_dir"`
}

type PeerConfig struct {
	Name string `yaml:"name"`
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

type SyncConfig struct {
	Interval             int `yaml:"interval"`
	MaxConcurrentTransfers int `yaml:"max_concurrent_transfers"`
	ChunkSize           int `yaml:"chunk_size"`
}

type DatabaseConfig struct {
	Path string `yaml:"path"`
}

type LoggingConfig struct {
	Level string `yaml:"level"`
	File  string `yaml:"file"`
}

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Peers    []PeerConfig   `yaml:"peers"`
	Sync     SyncConfig     `yaml:"sync"`
	Database DatabaseConfig `yaml:"database"`
	Logging  LoggingConfig  `yaml:"logging"`
}

func LoadConfig(configPath string) (*Config, error) {
	file, err := os.Open(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open config file: %w", err)
	}
	defer file.Close()

	var config Config
	decoder := yaml.NewDecoder(file)
	if err := decoder.Decode(&config); err != nil {
		return nil, fmt.Errorf("failed to decode config: %w", err)
	}

	return &config, nil
}

func (c *Config) GetServerAddr() string {
	return fmt.Sprintf("%s:%d", c.Server.Host, c.Server.Port)
}

func (c *Config) GetWebAddr() string {
	return fmt.Sprintf("%s:%d", c.Server.Host, c.Server.WebPort)
}

func (p *PeerConfig) GetAddr() string {
	return fmt.Sprintf("%s:%d", p.Host, p.Port)
}