package channels

// EventChannelsConfig configures buffer sizes for event channels
type EventChannelsConfig struct {
	DiscoveryBufferSize       int
	MonitorStateBufferSize    int
	PluginBufferSize          int
	CacheBufferSize           int
	DeviceValidatedBufferSize int
}

// PollingPipelineConfig configures buffer sizes for polling pipeline channels
type PollingPipelineConfig struct {
	LivenessQueueSize int
	PluginQueueSize   int
	ResultQueueSize   int
	StateQueueSize    int
}

// Config aggregates all channel configuration
type Config struct {
	Events   EventChannelsConfig
	Pipeline PollingPipelineConfig
}

// DefaultConfig returns sensible default buffer sizes
func DefaultConfig() Config {
	return Config{
		Events: EventChannelsConfig{
			DiscoveryBufferSize:    50,
			MonitorStateBufferSize: 100,
			PluginBufferSize:       100,
			CacheBufferSize:        50,
		},
		Pipeline: PollingPipelineConfig{
			LivenessQueueSize: 50,
			PluginQueueSize:   200,
			ResultQueueSize:   500,
			StateQueueSize:    100,
		},
	}
}
