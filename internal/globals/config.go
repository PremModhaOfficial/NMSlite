// Package config
package globals

import (
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/url"
	"os"
	"slices"
	"strings"
	"sync"
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
	Plugins   PluginsConfig   `yaml:"pluginManager"`
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
	Host     string     `yaml:"host"`
	Port     int        `yaml:"port"`
	User     string     `yaml:"user"`
	Password string     `yaml:"password"`
	DBName   string     `yaml:"dbname"`
	SSLMode  string     `yaml:"ssl_mode"`
	Pool     PoolConfig `yaml:"pool"`
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

// ReadTimeout returns the read timeout as a duration
func (s *ServerConfig) ReadTimeout() time.Duration {
	return time.Duration(s.ReadTimeoutMS) * time.Millisecond
}

// WriteTimeout returns the write timeout as a duration
func (s *ServerConfig) WriteTimeout() time.Duration {
	return time.Duration(s.WriteTimeoutMS) * time.Millisecond
}

// ConnString returns the PostgreSQL connection string in postgres:// URL format
func (d *DatabaseConfig) ConnString() string {
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
func (p *PoolConfig) ApplyDefaults() {
	// Unified pool defaults - balanced for all operations
	if p.MaxConns == 0 {
		p.MaxConns = 50
	}
	if p.MinConns == 0 {
		p.MinConns = 10
	}
	if p.MaxConnLifetimeMinutes == 0 {
		p.MaxConnLifetimeMinutes = 90
	}
	if p.MaxConnIdleTimeMinutes == 0 {
		p.MaxConnIdleTimeMinutes = 20
	}
	if p.HealthCheckPeriodSeconds == 0 {
		p.HealthCheckPeriodSeconds = 45
	}
}

// MaxConnLifetime returns the max connection lifetime as a duration
func (p *PoolConfig) MaxConnLifetime() time.Duration {
	return time.Duration(p.MaxConnLifetimeMinutes) * time.Minute
}

// MaxConnIdleTime returns the max connection idle time as a duration
func (p *PoolConfig) MaxConnIdleTime() time.Duration {
	return time.Duration(p.MaxConnIdleTimeMinutes) * time.Minute
}

// HealthCheckPeriod returns the health check period as a duration
func (p *PoolConfig) HealthCheckPeriod() time.Duration {
	return time.Duration(p.HealthCheckPeriodSeconds) * time.Second
}

// JWTExpiry returns JWT expiry as duration
func (a *AuthConfig) JWTExpiry() time.Duration {
	return time.Duration(a.JWTExpiryHours) * time.Hour
}

// IsLogLevelValid checks if the log level is valid
func (l *LoggingConfig) IsLogLevelValid() bool {
	validLevels := []string{"debug", "info", "warn", "error"}
	return slices.Contains(validLevels, strings.ToLower(l.Level))
}

// TickInterval returns the tick interval as a duration
func (s *SchedulerConfig) TickInterval() time.Duration {
	return time.Duration(s.TickIntervalMS) * time.Millisecond
}

// LivenessTimeout returns the liveness timeout as a duration
func (s *SchedulerConfig) LivenessTimeout() time.Duration {
	return time.Duration(s.LivenessTimeoutMS) * time.Millisecond
}

// PluginTimeout returns the plugin timeout as a duration
func (s *SchedulerConfig) PluginTimeout() time.Duration {
	return time.Duration(s.PluginTimeoutMS) * time.Millisecond
}

// DumpExampleConfig writes an example configuration to the provided writer
func DumpExampleConfig(w io.Writer) error {
	example := &Config{
		Server: ServerConfig{
			Host:           "0.0.0.0",
			Port:           8080,
			ReadTimeoutMS:  30000,
			WriteTimeoutMS: 30000,
		},
		TLS: TLSConfig{
			Enabled:  false,
			CertFile: "./certs/server.crt",
			KeyFile:  "./certs/server.key",
		},
		CORS: CORSConfig{
			Enabled:        true,
			AllowedOrigins: []string{"http://localhost:3000", "https://yourdomain.com"},
			AllowedMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
			AllowedHeaders: []string{"Authorization", "Content-Type"},
			MaxAgeSeconds:  3600,
		},
		Database: DatabaseConfig{
			Host:     "localhost",
			Port:     5432,
			User:     "nmslite",
			Password: "changeme",
			DBName:   "nmslite",
			SSLMode:  "disable",
			Pool: PoolConfig{
				MaxConns:                 50,
				MinConns:                 10,
				MaxConnLifetimeMinutes:   90,
				MaxConnIdleTimeMinutes:   20,
				HealthCheckPeriodSeconds: 45,
			},
		},
		Auth: AuthConfig{
			AdminUsername:  "admin",
			AdminPassword:  "changeme",
			JWTSecret:      "your-secret-key-minimum-32-chars-required",
			JWTExpiryHours: 24,
			EncryptionKey:  "32-character-encryption-key!",
		},
		Poller: PollerConfig{
			WorkerPoolSize:       50,
			LivenessPoolSize:     500,
			LivenessTimeoutMS:    2000,
			LivenessBatchSize:    50,
			BatchFlushIntervalMS: 100,
			PluginTimeoutMS:      60000,
			DownThreshold:        3,
		},
		Scheduler: SchedulerConfig{
			TickIntervalMS:    10000,
			LivenessWorkers:   100,
			PluginWorkers:     50,
			LivenessTimeoutMS: 2000,
			PluginTimeoutMS:   60000,
			DownThreshold:     3,
		},
		Metrics: MetricsConfig{
			BatchSize:             100,
			FlushIntervalMS:       10,
			RetentionDays:         90,
			CompressionAfterHours: 1,
			MaxBufferSize:         10000,
			MaxMetricAgeMinutes:   5,
		},
		Discovery: DiscoveryConfig{
			MaxDiscoveryWorkers:  100,
			DefaultPortTimeoutMS: 1000,
			HandshakeTimeoutMS:   5000,
		},
		Plugins: PluginsConfig{
			Directory:           "./plugin_bins/",
			ScanIntervalSeconds: 60,
		},
		Channel: EventBusConfig{
			PollJobsChannelSize:        100,
			MetricResultsChannelSize:   100,
			CacheEventsChannelSize:     50,
			StateSignalChannelSize:     50,
			DiscoveryEventsChannelSize: 50,
			DeviceValidatedChannelSize: 100,
		},
		Logging: LoggingConfig{
			Level:    "info",
			Format:   "json",
			Output:   "stdout",
			FilePath: "/var/log/nms/nms.log",
		},
	}

	// Create a YAML node for custom formatting with comments
	var node yaml.Node
	if err := node.Encode(example); err != nil {
		return fmt.Errorf("failed to encode config: %w", err)
	}

	// Write header comment
	header := `# =============================================================================
# NMS Lite Example Configuration
# =============================================================================
# This is an example configuration file for NMS Lite.
# Copy this file to config.yaml and modify it according to your needs.
#
# Environment variable overrides follow the pattern: NMS_<SECTION>_<KEY>
# Example: NMS_DATABASE_HOST, NMS_AUTH_JWT_SECRET
# =============================================================================

`
	if _, err := fmt.Fprint(w, header); err != nil {
		return fmt.Errorf("failed to write header: %w", err)
	}

	// Encode to YAML
	encoder := yaml.NewEncoder(w)
	encoder.SetIndent(2)
	if err := encoder.Encode(&node); err != nil {
		return fmt.Errorf("failed to encode YAML: %w", err)
	}

	if err := encoder.Close(); err != nil {
		return fmt.Errorf("failed to close encoder: %w", err)
	}

	// Write footer comment
	footer := `
# =============================================================================
# Important Notes:
# =============================================================================
# 
# 1. Security:
#    - Change all default passwords before deploying to production
#    - Set NMS_AUTH_JWT_SECRET to a strong random string (min 32 chars)
#    - Set NMS_AUTH_ENCRYPTION_KEY to exactly 32 random characters
#    - Enable TLS in production environments
#
# 2. Database:
#    - Ensure PostgreSQL is running and accessible
#    - Create the database before starting the application
#    - Configure connection pools based on your workload
#
# 3. Performance:
#    - Adjust worker pool sizes based on your hardware
#    - Configure tick intervals based on monitoring frequency needs
#    - Tune batch sizes for optimal database write performance
#
# 4. Plugins:
#    - Place plugin binaries in the configured pluginManager directory
#    - Ensure pluginManager are executable and have correct permissions
#
# For more information, visit: https://github.com/nmslite/nmslite
# =============================================================================
`
	if _, err := fmt.Fprint(w, footer); err != nil {
		return fmt.Errorf("failed to write footer: %w", err)
	}

	return nil
}

// -------------------------------------------------------------------------
// Global Configuration Access
// -------------------------------------------------------------------------

var (
	globalConfig *Config
	once         sync.Once
	mu           sync.RWMutex
)

// InitGlobal initializes the global configuration singleton
// This should be called once at application startup
func InitGlobal() *Config {
	once.Do(func() {
		mu.Lock()
		defer mu.Unlock()
		globalConfig = loadConfig()
	})

	return globalConfig
}

func loadConfig() *Config {
	cfg, err := Load("config.yaml")
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}
	return cfg
}

// GetConfig returns the global configuration instance
// Panics if InitGlobal has not been called
func GetConfig() *Config {
	mu.RLock()
	defer mu.RUnlock()
	if globalConfig == nil {
		panic("globals.GetConfig() called before InitGlobal()")
	}
	return globalConfig
}

// SetGlobalConfigForTests sets the global configuration instance for testing purposes
func SetGlobalConfigForTests(cfg *Config) {
	mu.Lock()
	defer mu.Unlock()
	globalConfig = cfg
}

// InitLogger initializes the global logger based on configuration
func InitLogger(cfg LoggingConfig) *slog.Logger {
	var handler slog.Handler

	// Set log level
	var level slog.Level
	switch cfg.Level {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{
		Level: level,
	}

	// Set format
	if cfg.Format == "json" {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	logger := slog.New(handler)
	slog.SetDefault(logger)

	return logger
}
