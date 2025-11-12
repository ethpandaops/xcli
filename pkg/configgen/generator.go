package configgen

import (
	"bytes"
	"embed"
	"fmt"
	"text/template"

	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/sirupsen/logrus"
)

//go:embed templates/*
var templatesFS embed.FS

// Generator generates service configuration files
type Generator struct {
	log logrus.FieldLogger
	cfg *config.LabConfig
}

// NewGenerator creates a new Generator instance
func NewGenerator(log logrus.FieldLogger, cfg *config.LabConfig) *Generator {
	return &Generator{
		log: log.WithField("component", "config-generator"),
		cfg: cfg,
	}
}

// GenerateCBTConfig generates CBT configuration for a network
func (g *Generator) GenerateCBTConfig(network string) (string, error) {
	// Assign metrics ports: 9100, 9101, 9102, etc. (leave room for other services)
	metricsPort := 9100
	redisDB := 0
	for i, net := range g.cfg.EnabledNetworks() {
		if net.Name == network {
			metricsPort = 9100 + i
			redisDB = i // mainnet=0, sepolia=1, holesky=2, etc.
			break
		}
	}

	data := map[string]interface{}{
		"Network":                    network,
		"MetricsPort":                metricsPort,
		"RedisDB":                    redisDB,
		"IsHybrid":                   g.cfg.Mode == "hybrid",
		"ExternalClickHouseURL":      g.cfg.Infrastructure.ClickHouse.Xatu.ExternalURL,
		"ExternalClickHouseDatabase": g.cfg.Infrastructure.ClickHouse.Xatu.ExternalDatabase,
		"ExternalClickHouseUsername": g.cfg.Infrastructure.ClickHouse.Xatu.ExternalUsername,
		"ExternalClickHousePassword": g.cfg.Infrastructure.ClickHouse.Xatu.ExternalPassword,
	}

	tmpl, err := template.New("cbt-config").ParseFS(templatesFS, "templates/cbt.yaml.tmpl")
	if err != nil {
		return "", fmt.Errorf("failed to parse CBT config template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "cbt-config", data); err != nil {
		return "", fmt.Errorf("failed to execute CBT config template: %w", err)
	}

	return buf.String(), nil
}

// GenerateCBTAPIConfig generates cbt-api configuration for a network
func (g *Generator) GenerateCBTAPIConfig(network string) (string, error) {
	port := g.cfg.GetCBTAPIPort(network)

	// Assign metrics ports: 9200, 9201, 9202, etc. (separate range from CBT engines)
	metricsPort := 9200
	for i, net := range g.cfg.EnabledNetworks() {
		if net.Name == network {
			metricsPort = 9200 + i
			break
		}
	}

	data := map[string]interface{}{
		"Network":     network,
		"Port":        port,
		"MetricsPort": metricsPort,
	}

	tmpl, err := template.New("cbt-api-config").ParseFS(templatesFS, "templates/cbt-api.yaml.tmpl")
	if err != nil {
		return "", fmt.Errorf("failed to parse cbt-api config template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "cbt-api-config", data); err != nil {
		return "", fmt.Errorf("failed to execute cbt-api config template: %w", err)
	}

	return buf.String(), nil
}

// GenerateLabBackendConfig generates lab-backend configuration
func (g *Generator) GenerateLabBackendConfig() (string, error) {
	networks := make([]map[string]interface{}, 0, len(g.cfg.Networks))
	for _, net := range g.cfg.EnabledNetworks() {
		networks = append(networks, map[string]interface{}{
			"Name": net.Name,
			"Port": g.cfg.GetCBTAPIPort(net.Name),
		})
	}

	data := map[string]interface{}{
		"Networks":     networks,
		"Port":         g.cfg.Ports.LabBackend,
		"FrontendPort": g.cfg.Ports.LabFrontend,
	}

	tmpl, err := template.New("lab-backend-config").ParseFS(templatesFS, "templates/lab-backend.yaml.tmpl")
	if err != nil {
		return "", fmt.Errorf("failed to parse lab-backend config template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "lab-backend-config", data); err != nil {
		return "", fmt.Errorf("failed to execute lab-backend config template: %w", err)
	}

	return buf.String(), nil
}
