# Global Config Refactoring Summary

## Overview
Refactored the configuration management to use a **global singleton pattern** instead of passing config objects around functions and structs. This eliminates parameter pollution and makes the code cleaner.

## Changes Made

### 1. Created Global Config Singleton
**File:** `/home/prem-modha/projects/NMSlite/internal/config/global.go`

Features:
- Thread-safe initialization using `sync.Once`
- Typed getter functions for each config section
- Panic-safe access (panics if not initialized)

### 2. Updated Main Application
**File:** `/home/prem-modha/projects/NMSlite/cmd/server/main.go`

Added initialization right after loading config:
```go
cfg := loadConfig()
config.InitGlobal(cfg) // Initialize global config singleton
```

### 3. Refactored Scheduler
**File:** `/home/prem-modha/projects/NMSlite/internal/poller/scheduler.go`

**Before:**
```go
func NewSchedulerImpl(
    querier dbgen.Querier,
    events *channels.EventChannels,
    pluginExecutor *plugins.Executor,
    pluginRegistry *plugins.Registry,
    credService *credentials.Service,
    resultWriter *ResultWriter,
    logger *slog.Logger,
    cfg config.SchedulerConfig, // ← Passed as parameter
) *SchedulerImpl {
    return &SchedulerImpl{
        tickInterval: cfg.GetTickInterval(),
        // ...
    }
}
```

**After:**
```go
func NewSchedulerImpl(
    querier dbgen.Querier,
    events *channels.EventChannels,
    pluginExecutor *plugins.Executor,
    pluginRegistry *plugins.Registry,
    credService *credentials.Service,
    resultWriter *ResultWriter,
    logger *slog.Logger,
    // ← No config parameter!
) *SchedulerImpl {
    cfg := config.GetScheduler() // ← Access global config
    return &SchedulerImpl{
        tickInterval: cfg.GetTickInterval(),
        // ...
    }
}
```

## Usage Guide

### Accessing Config Globally

Instead of passing config around, simply call the appropriate getter:

```go
// Get the entire config
cfg := config.Get()

// Or get specific sections
schedulerCfg := config.GetScheduler()
dbCfg := config.GetDatabase()
authCfg := config.GetAuth()
pollerCfg := config.GetPoller()
metricsCfg := config.GetMetrics()
discoveryCfg := config.GetDiscovery()
pluginsCfg := config.GetPlugins()
channelCfg := config.GetChannel()
loggingCfg := config.GetLogging()
serverCfg := config.GetServer()
tlsCfg := config.GetTLS()
corsCfg := config.GetCORS()
```

### Example Refactoring Pattern

**Before:**
```go
// In struct definition
type MyService struct {
    cfg config.MyConfig // ← Config stored as field
}

// In constructor
func NewMyService(cfg config.MyConfig) *MyService {
    return &MyService{cfg: cfg}
}

// In methods
func (s *MyService) DoSomething() {
    timeout := s.cfg.Timeout // ← Access from field
}
```

**After:**
```go
// In struct definition
type MyService struct {
    // ← No config field needed!
}

// In constructor
func NewMyService() *MyService { // ← No config parameter!
    return &MyService{}
}

// In methods
func (s *MyService) DoSomething() {
    cfg := config.GetScheduler() // ← Access globally
    timeout := cfg.Timeout
}
```

### Benefits

1. **Cleaner function signatures** - No need to pass config through multiple layers
2. **Less parameter pollution** - Constructors have fewer parameters
3. **Global accessibility** - Config available anywhere in the codebase
4. **Type safety** - Typed getter functions for each section
5. **Thread-safe** - Uses sync.Once for initialization
6. **Fail-fast** - Panics if accessed before initialization

### Potential Areas for Further Refactoring

You can apply the same pattern to other parts of your codebase:

1. **Database module** (`internal/database/database.go`):
   - `InitDB(ctx, cfg)` → `InitDB(ctx)` - use `config.GetDatabase()`
   - `RunMigrations(ctx, cfg)` → `RunMigrations(ctx)` - use `config.GetDatabase()`

2. **API router** (`internal/api/router.go`):
   - `NewRouter(cfg, ...)` → `NewRouter(...)` - use config getters internally

3. **BatchWriter** (`internal/poller/batch_writer.go`):
   - If it takes `*config.MetricsConfig`, use `config.GetMetrics()` instead

4. **Discovery Worker** - If it uses config, refactor similarly

### Important Notes

⚠️ **Initialization Order**
- `config.InitGlobal()` MUST be called before any code tries to access the config
- Currently called in `main.go` right after loading the config file
- In tests, call `config.InitGlobal(testConfig)` in setup

⚠️ **Testing**
- Each test should call `config.InitGlobal()` with test configuration
- The `sync.Once` ensures it's only initialized once per test run
- See example in `scheduler_test.go`

⚠️ **Thread Safety**
- The global config is read-only after initialization
- Safe for concurrent access
- Do not attempt to modify config after initialization
