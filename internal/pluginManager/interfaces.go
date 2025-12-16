package pluginManager

import "context"

// PluginPoller can collect metrics from a device
type PluginPoller interface {
	Poll(ctx context.Context, tasks []PollTask) ([]PollResult, error)
}

// PluginRegistry manages plugin discovery and lookup
type PluginRegistry interface {
	Scan() error
	GetByID(id string) (*PluginInfo, bool)
	GetByPort(port int) []*PluginInfo
	GetByProtocol(protocol string) (*PluginInfo, error)
	List() []*PluginInfo
}
