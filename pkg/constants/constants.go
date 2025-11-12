package constants

// Stack modes.
const (
	ModeLocal  = "local"
	ModeHybrid = "hybrid"
)

// Infrastructure modes.
const (
	InfraModeLocal    = "local"
	InfraModeExternal = "external"
)

// Service names and prefixes.
const (
	ServiceLabBackend   = "lab-backend"
	ServiceLabFrontend  = "lab-frontend"
	ServicePrefixCBT    = "cbt-"
	ServicePrefixCBTAPI = "cbt-api-"
)

// Binary names.
const (
	BinaryCBT        = "cbt"
	BinaryCBTAPI     = "server"
	BinaryLabBackend = "lab-backend"
)

// Directory names.
const (
	DirBin           = "bin"
	DirConfigs       = "configs"
	DirCustomConfigs = "custom-configs"
	DirLogs          = "logs"
	DirPIDs          = "pids"
)

// Config file templates.
const (
	ConfigFileCBT        = "cbt-%s.yaml"
	ConfigFileCBTAPI     = "cbt-api-%s.yaml"
	ConfigFileLabBackend = "lab-backend.yaml"
)

// PID file template.
const (
	PIDFileTemplate = "%s.pid"
)

// Log file template.
const (
	LogFileTemplate = "%s.log"
)

// Service name helpers.
// ServiceName returns the full service name for a network-specific service.
func ServiceNameCBT(network string) string {
	return ServicePrefixCBT + network
}

// ServiceNameCBTAPI returns the full service name for a cbt-api instance.
func ServiceNameCBTAPI(network string) string {
	return ServicePrefixCBTAPI + network
}
