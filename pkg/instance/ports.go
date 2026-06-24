package instance

import (
	"fmt"
	"sort"
	"strconv"

	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/ethpandaops/xcli/pkg/constants"
)

const (
	portStride = 1000

	DefaultCommandCenterPort = constants.DefaultCommandCenterPort
)

var xatuCBTBindAddressVars = []string{
	"CLICKHOUSE_CBT_01_HTTP_ADDRESS",
	"CLICKHOUSE_CBT_01_NATIVE_ADDRESS",
	"CLICKHOUSE_CBT_02_HTTP_ADDRESS",
	"CLICKHOUSE_CBT_02_NATIVE_ADDRESS",
	"CLICKHOUSE_XATU_01_HTTP_ADDRESS",
	"CLICKHOUSE_XATU_01_NATIVE_ADDRESS",
	"CLICKHOUSE_XATU_02_HTTP_ADDRESS",
	"CLICKHOUSE_XATU_02_NATIVE_ADDRESS",
	"REDIS_ADDRESS",
}

var namedPortFields = []struct {
	name string
	get  func(PortPlan) int
}{
	{"lab-backend", func(p PortPlan) int { return p.LabBackend }},
	{"lab-frontend", func(p PortPlan) int { return p.LabFrontend }},
	{"command-center", func(p PortPlan) int { return p.CommandCenter }},
	{"clickhouse-cbt-01-http", func(p PortPlan) int { return p.ClickHouseCBT01HTTP }},
	{"clickhouse-cbt-01-native", func(p PortPlan) int { return p.ClickHouseCBT01TCP }},
	{"clickhouse-cbt-02-http", func(p PortPlan) int { return p.ClickHouseCBT02HTTP }},
	{"clickhouse-cbt-02-native", func(p PortPlan) int { return p.ClickHouseCBT02TCP }},
	{"clickhouse-xatu-01-http", func(p PortPlan) int { return p.ClickHouseXatu01HTTP }},
	{"clickhouse-xatu-01-native", func(p PortPlan) int { return p.ClickHouseXatu01TCP }},
	{"clickhouse-xatu-02-http", func(p PortPlan) int { return p.ClickHouseXatu02HTTP }},
	{"clickhouse-xatu-02-native", func(p PortPlan) int { return p.ClickHouseXatu02TCP }},
	{"redis", func(p PortPlan) int { return p.Redis }},
	{"prometheus", func(p PortPlan) int { return p.Prometheus }},
	{"grafana", func(p PortPlan) int { return p.Grafana }},
}

var xatuCBTPortEnvFields = []struct {
	name string
	get  func(PortPlan) int
}{
	{"CLICKHOUSE_CBT_01_HTTP_PORT", func(p PortPlan) int { return p.ClickHouseCBT01HTTP }},
	{"CLICKHOUSE_CBT_01_NATIVE_PORT", func(p PortPlan) int { return p.ClickHouseCBT01TCP }},
	{"CLICKHOUSE_CBT_02_HTTP_PORT", func(p PortPlan) int { return p.ClickHouseCBT02HTTP }},
	{"CLICKHOUSE_CBT_02_NATIVE_PORT", func(p PortPlan) int { return p.ClickHouseCBT02TCP }},
	{"CLICKHOUSE_XATU_01_HTTP_PORT", func(p PortPlan) int { return p.ClickHouseXatu01HTTP }},
	{"CLICKHOUSE_XATU_01_NATIVE_PORT", func(p PortPlan) int { return p.ClickHouseXatu01TCP }},
	{"CLICKHOUSE_XATU_02_HTTP_PORT", func(p PortPlan) int { return p.ClickHouseXatu02HTTP }},
	{"CLICKHOUSE_XATU_02_NATIVE_PORT", func(p PortPlan) int { return p.ClickHouseXatu02TCP }},
	{"REDIS_PORT", func(p PortPlan) int { return p.Redis }},
}

// XatuCBTBindAddressVars returns the bind-address env vars xcli pins to localhost.
func XatuCBTBindAddressVars() []string {
	return append([]string(nil), xatuCBTBindAddressVars...)
}

// DefaultPortPlan returns the slot-0 defaults before config overrides.
func DefaultPortPlan() PortPlan {
	return PortPlan{
		LabBackend:           constants.DefaultLabBackendPort,
		LabFrontend:          constants.DefaultLabFrontendPort,
		CBTBase:              constants.DefaultCBTBasePort,
		CBTAPIBase:           constants.DefaultCBTAPIBasePort,
		CBTFrontendBase:      constants.DefaultCBTFrontendBasePort,
		CBTMetricsBase:       constants.DefaultCBTMetricsBasePort,
		CBTAPIMetricsBase:    constants.DefaultCBTAPIMetricsBasePort,
		ClickHouseXatu01HTTP: constants.DefaultClickHouseXatuHTTPPort,
		ClickHouseXatu01TCP:  constants.DefaultClickHouseXatuNativePort,
		ClickHouseXatu02HTTP: constants.DefaultClickHouseXatuHTTPPort + 1,
		ClickHouseXatu02TCP:  constants.DefaultClickHouseXatuNativePort + 1,
		ClickHouseCBT01HTTP:  constants.DefaultClickHouseCBTHTTPPort,
		ClickHouseCBT01TCP:   constants.DefaultClickHouseCBTNativePort,
		ClickHouseCBT02HTTP:  constants.DefaultClickHouseCBTHTTPPort + 1,
		ClickHouseCBT02TCP:   constants.DefaultClickHouseCBTNativePort + 1,
		Redis:                constants.DefaultRedisPort,
		Prometheus:           constants.DefaultPrometheusPort,
		Grafana:              constants.DefaultGrafanaPort,
		CommandCenter:        constants.DefaultCommandCenterPort,
	}
}

// BuildPortPlan returns the preferred ports for a slot.
func BuildPortPlan(labCfg *config.LabConfig, slot int) (PortPlan, error) {
	if labCfg == nil {
		return PortPlan{}, fmt.Errorf("lab config is required")
	}
	if slot < 0 {
		return PortPlan{}, fmt.Errorf("slot must be non-negative")
	}

	offset := slot * portStride
	defaults := DefaultPortPlan()
	cbtHTTP := withDefault(labCfg.Infrastructure.ClickHouseCBTPort, defaults.ClickHouseCBT01HTTP)
	xatuHTTP := withDefault(labCfg.Infrastructure.ClickHouseXatuPort, defaults.ClickHouseXatu01HTTP)
	redis := withDefault(labCfg.Infrastructure.RedisPort, labCfg.Infrastructure.Redis.Port, defaults.Redis)
	prometheus := withDefault(labCfg.Infrastructure.Observability.PrometheusPort, defaults.Prometheus)
	grafana := withDefault(labCfg.Infrastructure.Observability.GrafanaPort, defaults.Grafana)

	cbtBase := withDefault(labCfg.Ports.CBTBase, defaults.CBTBase)
	cbtAPIBase := withDefault(labCfg.Ports.CBTAPIBase, defaults.CBTAPIBase)
	cbtFrontendBase := withDefault(labCfg.Ports.CBTFrontendBase, defaults.CBTFrontendBase)
	cbtMetricsBase := defaults.CBTMetricsBase + offset
	cbtAPIMetricsBase := defaults.CBTAPIMetricsBase + offset

	plan := PortPlan{
		Slot:                 slot,
		LabBackend:           withDefault(labCfg.Ports.LabBackend, defaults.LabBackend) + offset,
		LabFrontend:          withDefault(labCfg.Ports.LabFrontend, defaults.LabFrontend) + offset,
		CBTBase:              cbtBase + offset,
		CBTAPIBase:           cbtAPIBase + offset,
		CBTFrontendBase:      cbtFrontendBase + offset,
		CBTMetricsBase:       cbtMetricsBase,
		CBTAPIMetricsBase:    cbtAPIMetricsBase,
		ClickHouseXatu01HTTP: xatuHTTP + offset,
		ClickHouseXatu01TCP:  defaults.ClickHouseXatu01TCP + offset,
		ClickHouseXatu02HTTP: xatuHTTP + 1 + offset,
		ClickHouseXatu02TCP:  defaults.ClickHouseXatu02TCP + offset,
		ClickHouseCBT01HTTP:  cbtHTTP + offset,
		ClickHouseCBT01TCP:   defaults.ClickHouseCBT01TCP + offset,
		ClickHouseCBT02HTTP:  cbtHTTP + 1 + offset,
		ClickHouseCBT02TCP:   defaults.ClickHouseCBT02TCP + offset,
		Redis:                redis + offset,
		Prometheus:           prometheus + offset,
		Grafana:              grafana + offset,
		CommandCenter:        defaults.CommandCenter + offset,
		Networks:             make(map[string]NetworkPortPlan, len(labCfg.EnabledNetworks())),
	}

	for i, network := range labCfg.EnabledNetworks() {
		plan.Networks[network.Name] = NetworkPortPlan{
			CBT:           cbtBase + network.PortOffset + offset,
			CBTAPI:        cbtAPIBase + network.PortOffset + offset,
			CBTFrontend:   cbtFrontendBase + network.PortOffset + offset,
			CBTMetrics:    cbtMetricsBase + i,
			CBTAPIMetrics: cbtAPIMetricsBase + i,
		}
	}

	if duplicates := plan.DuplicatePorts(); len(duplicates) > 0 {
		return PortPlan{}, fmt.Errorf("port plan has duplicate ports: %v", duplicates)
	}

	return plan, nil
}

// WithDefaults fills zero fields in p from fallback.
func (p PortPlan) WithDefaults(fallback PortPlan) PortPlan {
	if p.LabBackend == 0 {
		p.LabBackend = fallback.LabBackend
	}
	if p.LabFrontend == 0 {
		p.LabFrontend = fallback.LabFrontend
	}
	if p.CBTBase == 0 {
		p.CBTBase = fallback.CBTBase
	}
	if p.CBTAPIBase == 0 {
		p.CBTAPIBase = fallback.CBTAPIBase
	}
	if p.CBTFrontendBase == 0 {
		p.CBTFrontendBase = fallback.CBTFrontendBase
	}
	if p.CBTMetricsBase == 0 {
		p.CBTMetricsBase = fallback.CBTMetricsBase
	}
	if p.CBTAPIMetricsBase == 0 {
		p.CBTAPIMetricsBase = fallback.CBTAPIMetricsBase
	}
	if p.ClickHouseXatu01HTTP == 0 {
		p.ClickHouseXatu01HTTP = fallback.ClickHouseXatu01HTTP
	}
	if p.ClickHouseXatu01TCP == 0 {
		p.ClickHouseXatu01TCP = fallback.ClickHouseXatu01TCP
	}
	if p.ClickHouseXatu02HTTP == 0 {
		p.ClickHouseXatu02HTTP = fallback.ClickHouseXatu02HTTP
	}
	if p.ClickHouseXatu02TCP == 0 {
		p.ClickHouseXatu02TCP = fallback.ClickHouseXatu02TCP
	}
	if p.ClickHouseCBT01HTTP == 0 {
		p.ClickHouseCBT01HTTP = fallback.ClickHouseCBT01HTTP
	}
	if p.ClickHouseCBT01TCP == 0 {
		p.ClickHouseCBT01TCP = fallback.ClickHouseCBT01TCP
	}
	if p.ClickHouseCBT02HTTP == 0 {
		p.ClickHouseCBT02HTTP = fallback.ClickHouseCBT02HTTP
	}
	if p.ClickHouseCBT02TCP == 0 {
		p.ClickHouseCBT02TCP = fallback.ClickHouseCBT02TCP
	}
	if p.Redis == 0 {
		p.Redis = fallback.Redis
	}
	if p.Prometheus == 0 {
		p.Prometheus = fallback.Prometheus
	}
	if p.Grafana == 0 {
		p.Grafana = fallback.Grafana
	}
	if p.CommandCenter == 0 {
		p.CommandCenter = fallback.CommandCenter
	}
	p.Networks = networkPortsWithDefaults(p.Networks, fallback.Networks)

	return p
}

// XatuCBTEnv returns the project and host-port env vars for xatu-cbt infra.
func (p PortPlan) XatuCBTEnv(projectName string) map[string]string {
	p = p.WithDefaults(DefaultPortPlan())
	env := map[string]string{
		"XATU_CBT_PROJECT_NAME": projectName,
	}
	for _, field := range xatuCBTPortEnvFields {
		env[field.name] = strconv.Itoa(field.get(p))
	}
	for _, name := range xatuCBTBindAddressVars {
		env[name] = "127.0.0.1"
	}

	return env
}

// AllPorts returns every concrete port in this plan, sorted ascending.
func (p PortPlan) AllPorts() []int {
	namedPorts := p.NamedPorts()
	ports := make([]int, 0, len(namedPorts))
	for _, port := range namedPorts {
		ports = append(ports, port)
	}

	filtered := ports[:0]
	for _, port := range ports {
		if port > 0 {
			filtered = append(filtered, port)
		}
	}

	sort.Ints(filtered)

	return filtered
}

// NamedPorts returns the named concrete ports in this plan.
func (p PortPlan) NamedPorts() map[string]int {
	ports := make(map[string]int, len(namedPortFields)+len(p.Networks)*5)
	for _, field := range namedPortFields {
		ports[field.name] = field.get(p)
	}

	for network, networkPorts := range p.Networks {
		ports["cbt-"+network] = networkPorts.CBT
		ports["cbt-api-"+network] = networkPorts.CBTAPI
		ports["cbt-frontend-"+network] = networkPorts.CBTFrontend
		ports["cbt-metrics-"+network] = networkPorts.CBTMetrics
		ports["cbt-api-metrics-"+network] = networkPorts.CBTAPIMetrics
	}

	return ports
}

func networkPortsWithDefaults(
	ports map[string]NetworkPortPlan,
	fallback map[string]NetworkPortPlan,
) map[string]NetworkPortPlan {
	if len(ports) == 0 && len(fallback) == 0 {
		return ports
	}

	merged := make(map[string]NetworkPortPlan, len(ports)+len(fallback))
	for name, network := range fallback {
		merged[name] = network
	}
	for name, network := range ports {
		fallbackNetwork := merged[name]
		if network.CBT == 0 {
			network.CBT = fallbackNetwork.CBT
		}
		if network.CBTAPI == 0 {
			network.CBTAPI = fallbackNetwork.CBTAPI
		}
		if network.CBTFrontend == 0 {
			network.CBTFrontend = fallbackNetwork.CBTFrontend
		}
		if network.CBTMetrics == 0 {
			network.CBTMetrics = fallbackNetwork.CBTMetrics
		}
		if network.CBTAPIMetrics == 0 {
			network.CBTAPIMetrics = fallbackNetwork.CBTAPIMetrics
		}
		merged[name] = network
	}

	return merged
}

// DuplicatePorts returns ports that appear more than once in the full plan.
func (p PortPlan) DuplicatePorts() []int {
	counts := make(map[int]int)
	for _, port := range p.AllPorts() {
		counts[port]++
	}

	duplicates := make([]int, 0)
	for port, count := range counts {
		if count > 1 {
			duplicates = append(duplicates, port)
		}
	}
	sort.Ints(duplicates)

	return duplicates
}

// Overlaps returns ports present in both plans.
func (p PortPlan) Overlaps(other PortPlan) []int {
	seen := make(map[int]bool)
	for _, port := range p.AllPorts() {
		seen[port] = true
	}

	overlap := make([]int, 0)
	for _, port := range other.AllPorts() {
		if seen[port] {
			overlap = append(overlap, port)
		}
	}

	sort.Ints(overlap)

	return overlap
}

func withDefault(values ...int) int {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}

	return 0
}
