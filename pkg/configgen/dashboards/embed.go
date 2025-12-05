// Package dashboards contains embedded default Grafana dashboards.
package dashboards

import "embed"

// DefaultDashboards contains all .json dashboard files from this directory.
// These are shipped with the xcli binary and deployed alongside custom dashboards.
//
//go:embed *.json
var DefaultDashboards embed.FS
