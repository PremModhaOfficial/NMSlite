package channels

// EventChannelsConfig configures buffer sizes for event channels
type EventChannelsConfig struct {
	DiscoveryBufferSize    int
	MonitorStateBufferSize int
	PluginBufferSize       int
	CacheBufferSize        int
}
