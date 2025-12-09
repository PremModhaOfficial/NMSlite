// Package config
package config

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server    ServerConfig    `yaml:"server"`
	TLS       TLSConfig       `yaml:"tls"`
	CORS      CORSConfig      `yaml:"cors"`
	Database  DatabaseConfig  `yaml:"database"`
	Auth      AuthConfig      `yaml:"auth"`
	Poller    PollerConfig    `yaml:"poller"`
	Metrics   MetricsConfig   `yaml:"metrics"`
	Discovery DiscoveryConfig `yaml:"discovery"`
	Plugins   PluginsConfig   `yaml:"plugins"`
	Logging   LoggingConfig   `yaml:"logging"`
}

type ServerConfig struct {
	Host           string `yaml:"host"`
	Port           int    `yaml:"port"`
	ReadTimeoutMS  int    `yaml:"read_timeout_ms"`
	WriteTimeoutMS int    `yaml:"write_timeout_ms"`
}

type TLSConfig struct {
	Enabled  bool   `yaml:"enabled"`
	CertFile string `yaml:"cert_file"`
	KeyFile  string `yaml:"key_file"`
}

type CORSConfig struct {
	Enabled        bool     `yaml:"enabled"`
	AllowedOrigins []string `yaml:"allowed_origins"`
	AllowedMethods []string `yaml:"allowed_methods"`
	AllowedHeaders []string `yaml:"allowed_headers"`
	MaxAgeSeconds  int      `yaml:"max_age_seconds"`
}

type DatabaseConfig struct {
	Host                   string `yaml:"host"`
	Port                   int    `yaml:"port"`
	User                   string `yaml:"user"`
	Password               string `yaml:"password"`
	DBName                 string `yaml:"dbname"`
	SSLMode                string `yaml:"ssl_mode"`
	MaxOpenConns           int    `yaml:"max_open_conns"`
	MaxIdleConns           int    `yaml:"max_idle_conns"`
	ConnMaxLifetimeMinutes int    `yaml:"conn_max_lifetime_minutes"`
}

type AuthConfig struct {
	AdminUsername  string `yaml:"admin_username"`
	AdminPassword  string `yaml:"admin_password"`
	JWTSecret      string `yaml:"jwt_secret"`
	JWTExpiryHours int    `yaml:"jwt_expiry_hours"`
	EncryptionKey  string `yaml:"encryption_key"`
}

type PollerConfig struct {
	WorkerPoolSize       int `yaml:"worker_pool_size"`
	LivenessPoolSize     int `yaml:"liveness_pool_size"`
	LivenessTimeoutMS    int `yaml:"liveness_timeout_ms"`
	LivenessBatchSize    int `yaml:"liveness_batch_size"`
	BatchFlushIntervalMS int `yaml:"batch_flush_interval_ms"`
	PluginTimeoutMS      int `yaml:"plugin_timeout_ms"`
	DownThreshold        int `yaml:"down_threshold"`
}

type MetricsConfig struct {
	BatchSize             int `yaml:"batch_size"`
	FlushIntervalMS       int `yaml:"flush_interval_ms"`
	RetentionDays         int `yaml:"retention_days"`
	CompressionAfterHours int `yaml:"compression_after_hours"`
}

type DiscoveryConfig struct {
	MaxDiscoveryWorkers  int `yaml:"max_discovery_workers"`
	DefaultPortTimeoutMS int `yaml:"default_port_timeout_ms"`
}

type PluginsConfig struct {
	Directory           string `yaml:"directory"`
	ScanIntervalSeconds int    `yaml:"scan_interval_seconds"`
}

type LoggingConfig struct {
	Level    string `yaml:"level"`
	Format   string `yaml:"format"`
	Output   string `yaml:"output"`
	FilePath string `yaml:"file_path"`
}

// Load reads configuration from file and applies environment variable overrides
func Load(configPath string) (*Config, error) {
	cfg := &Config{}

	// Read config file
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Parse YAML
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Apply environment variable overrides
	applyEnvOverrides(cfg)

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return cfg, nil
}

// Validate ensures all required configuration values are set
func (c *Config) Validate() error {
	// Required auth fields in production
	if c.Auth.JWTSecret == "" {
		return fmt.Errorf("NMS_AUTH_JWT_SECRET is required (minimum 32 characters)")
	}
	if len(c.Auth.JWTSecret) < 32 {
		return fmt.Errorf("jwt_secret must be at least 32 characters")
	}

	if c.Auth.EncryptionKey == "" {
		return fmt.Errorf("NMS_AUTH_ENCRYPTION_KEY is required (32 bytes for AES-256)")
	}
	if len(c.Auth.EncryptionKey) != 32 {
		return fmt.Errorf("encryption_key must be exactly 32 bytes")
	}

	if c.Auth.AdminPassword == "" || c.Auth.AdminPassword == "changeme" {
		return fmt.Errorf("NMS_AUTH_ADMIN_PASSWORD must be set to a strong password")
	}

	// Validate database config
	if c.Database.Host == "" || c.Database.DBName == "" {
		return fmt.Errorf("database host and dbname are required")
	}

	return nil
}

// applyEnvOverrides checks for environment variables with NMS_ prefix
func applyEnvOverrides(cfg *Config) {
	// Database overrides
	if v := os.Getenv("NMS_DATABASE_HOST"); v != "" {
		cfg.Database.Host = v
	}
	if v := os.Getenv("NMS_DATABASE_PORT"); v != "" {
		fmt.Sscanf(v, "%d", &cfg.Database.Port)
	}
	if v := os.Getenv("NMS_DATABASE_PASSWORD"); v != "" {
		cfg.Database.Password = v
	}

	// Auth overrides
	if v := os.Getenv("NMS_AUTH_ADMIN_PASSWORD"); v != "" {
		cfg.Auth.AdminPassword = v
	}
	if v := os.Getenv("NMS_AUTH_JWT_SECRET"); v != "" {
		cfg.Auth.JWTSecret = v
	}
	if v := os.Getenv("NMS_AUTH_ENCRYPTION_KEY"); v != "" {
		cfg.Auth.EncryptionKey = v
	}
}

// GetReadTimeout returns the read timeout as a duration
func (s *ServerConfig) GetReadTimeout() time.Duration {
	return time.Duration(s.ReadTimeoutMS) * time.Millisecond
}

// GetWriteTimeout returns the write timeout as a duration
func (s *ServerConfig) GetWriteTimeout() time.Duration {
	return time.Duration(s.WriteTimeoutMS) * time.Millisecond
}

// GetDSN returns the PostgreSQL connection string
func (d *DatabaseConfig) GetDSN() string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		d.Host, d.Port, d.User, d.Password, d.DBName, d.SSLMode,
	)
}

// GetJWTExpiry returns JWT expiry as duration
func (a *AuthConfig) GetJWTExpiry() time.Duration {
	return time.Duration(a.JWTExpiryHours) * time.Hour
}

// IsLogLevelValid checks if the log level is valid
func (l *LoggingConfig) IsLogLevelValid() bool {
	validLevels := []string{"debug", "info", "warn", "error"}
	return slices.Contains(validLevels, strings.ToLower(l.Level))
}
