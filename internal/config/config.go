// Package config
package config

import (
	"fmt"
	"net/url"
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
	Scheduler SchedulerConfig `yaml:"scheduler"`
	Metrics   MetricsConfig   `yaml:"metrics"`
	Discovery DiscoveryConfig `yaml:"discovery"`
	Plugins   PluginsConfig   `yaml:"plugins"`
	Channel   EventBusConfig  `yaml:"channel"`
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

// PoolConfig defines connection pool settings
type PoolConfig struct {
	MaxConns                 int `yaml:"max_conns"`
	MinConns                 int `yaml:"min_conns"`
	MaxConnLifetimeMinutes   int `yaml:"max_conn_lifetime_minutes"`
	MaxConnIdleTimeMinutes   int `yaml:"max_conn_idle_time_minutes"`
	HealthCheckPeriodSeconds int `yaml:"health_check_period_seconds"`
}

type DatabaseConfig struct {
	Host        string     `yaml:"host"`
	Port        int        `yaml:"port"`
	User        string     `yaml:"user"`
	Password    string     `yaml:"password"`
	DBName      string     `yaml:"dbname"`
	SSLMode     string     `yaml:"ssl_mode"`
	APIPool     PoolConfig `yaml:"api_pool"`
	MetricsPool PoolConfig `yaml:"metrics_pool"`
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

type SchedulerConfig struct {
	TickIntervalMS    int `yaml:"tick_interval_ms"`
	LivenessWorkers   int `yaml:"liveness_workers"`
	PluginWorkers     int `yaml:"plugin_workers"`
	LivenessTimeoutMS int `yaml:"liveness_timeout_ms"`
	PluginTimeoutMS   int `yaml:"plugin_timeout_ms"`
	DownThreshold     int `yaml:"down_threshold"`
}

type MetricsConfig struct {
	BatchSize             int `yaml:"batch_size"`
	FlushIntervalMS       int `yaml:"flush_interval_ms"`
	RetentionDays         int `yaml:"retention_days"`
	CompressionAfterHours int `yaml:"compression_after_hours"`
	MaxBufferSize         int `yaml:"max_buffer_size"`
	MaxMetricAgeMinutes   int `yaml:"max_metric_age_minutes"`
}

type DiscoveryConfig struct {
	MaxDiscoveryWorkers  int `yaml:"max_discovery_workers"`
	DefaultPortTimeoutMS int `yaml:"default_port_timeout_ms"`
	HandshakeTimeoutMS   int `yaml:"handshake_timeout_ms"`
}

type PluginsConfig struct {
	Directory           string `yaml:"directory"`
	ScanIntervalSeconds int    `yaml:"scan_interval_seconds"`
}

type EventBusConfig struct {
	PollJobsChannelSize        int `yaml:"poll_jobs_channel_size"`
	MetricResultsChannelSize   int `yaml:"metric_results_channel_size"`
	CacheEventsChannelSize     int `yaml:"cache_events_channel_size"`
	StateSignalChannelSize     int `yaml:"state_signal_channel_size"`
	DiscoveryEventsChannelSize int `yaml:"discovery_events_channel_size"`
	DeviceValidatedChannelSize int `yaml:"device_validated_channel_size"`
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

	// Discovery overrides
	if v := os.Getenv("NMS_DISCOVERY_HANDSHAKE_TIMEOUT_MS"); v != "" {
		fmt.Sscanf(v, "%d", &cfg.Discovery.HandshakeTimeoutMS)
	}

	// EventBus overrides
	if v := os.Getenv("NMS_EVENTBUS_POLL_JOBS_CHANNEL_SIZE"); v != "" {
		fmt.Sscanf(v, "%d", &cfg.Channel.PollJobsChannelSize)
	}
	if v := os.Getenv("NMS_EVENTBUS_METRIC_RESULTS_CHANNEL_SIZE"); v != "" {
		fmt.Sscanf(v, "%d", &cfg.Channel.MetricResultsChannelSize)
	}
	if v := os.Getenv("NMS_EVENTBUS_CACHE_EVENTS_CHANNEL_SIZE"); v != "" {
		fmt.Sscanf(v, "%d", &cfg.Channel.CacheEventsChannelSize)
	}
	if v := os.Getenv("NMS_EVENTBUS_STATE_SIGNAL_CHANNEL_SIZE"); v != "" {
		fmt.Sscanf(v, "%d", &cfg.Channel.StateSignalChannelSize)
	}
	if v := os.Getenv("NMS_EVENTBUS_DISCOVERY_EVENTS_CHANNEL_SIZE"); v != "" {
		fmt.Sscanf(v, "%d", &cfg.Channel.DiscoveryEventsChannelSize)
	}
	if v := os.Getenv("NMS_CHANNEL_DEVICE_VALIDATED_CHANNEL_SIZE"); v != "" {
		fmt.Sscanf(v, "%d", &cfg.Channel.DeviceValidatedChannelSize)
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

// GetConnString returns the PostgreSQL connection string in postgres:// URL format
func (d *DatabaseConfig) GetConnString() string {
	u := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(d.User, d.Password),
		Host:   fmt.Sprintf("%s:%d", d.Host, d.Port),
		Path:   d.DBName,
	}

	query := url.Values{}
	if d.SSLMode != "" {
		query.Set("sslmode", d.SSLMode)
	}
	u.RawQuery = query.Encode()

	return u.String()
}

// ApplyDefaults sets default values for pool configuration
func (p *PoolConfig) ApplyDefaults(isAPI bool) {
	if isAPI {
		// API pool defaults - lower connections, shorter lifetimes
		if p.MaxConns == 0 {
			p.MaxConns = 25
		}
		if p.MinConns == 0 {
			p.MinConns = 5
		}
		if p.MaxConnLifetimeMinutes == 0 {
			p.MaxConnLifetimeMinutes = 60
		}
		if p.MaxConnIdleTimeMinutes == 0 {
			p.MaxConnIdleTimeMinutes = 10
		}
		if p.HealthCheckPeriodSeconds == 0 {
			p.HealthCheckPeriodSeconds = 30
		}
	} else {
		// Metrics pool defaults - higher connections, longer lifetimes
		if p.MaxConns == 0 {
			p.MaxConns = 50
		}
		if p.MinConns == 0 {
			p.MinConns = 10
		}
		if p.MaxConnLifetimeMinutes == 0 {
			p.MaxConnLifetimeMinutes = 120
		}
		if p.MaxConnIdleTimeMinutes == 0 {
			p.MaxConnIdleTimeMinutes = 30
		}
		if p.HealthCheckPeriodSeconds == 0 {
			p.HealthCheckPeriodSeconds = 60
		}
	}
}

// GetMaxConnLifetime returns the max connection lifetime as a duration
func (p *PoolConfig) GetMaxConnLifetime() time.Duration {
	return time.Duration(p.MaxConnLifetimeMinutes) * time.Minute
}

// GetMaxConnIdleTime returns the max connection idle time as a duration
func (p *PoolConfig) GetMaxConnIdleTime() time.Duration {
	return time.Duration(p.MaxConnIdleTimeMinutes) * time.Minute
}

// GetHealthCheckPeriod returns the health check period as a duration
func (p *PoolConfig) GetHealthCheckPeriod() time.Duration {
	return time.Duration(p.HealthCheckPeriodSeconds) * time.Second
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

// GetTickInterval returns the tick interval as a duration
func (s *SchedulerConfig) GetTickInterval() time.Duration {
	return time.Duration(s.TickIntervalMS) * time.Millisecond
}

// GetLivenessTimeout returns the liveness timeout as a duration
func (s *SchedulerConfig) GetLivenessTimeout() time.Duration {
	return time.Duration(s.LivenessTimeoutMS) * time.Millisecond
}

// GetPluginTimeout returns the plugin timeout as a duration
func (s *SchedulerConfig) GetPluginTimeout() time.Duration {
	return time.Duration(s.PluginTimeoutMS) * time.Millisecond
}
