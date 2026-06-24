package instance

import (
	"testing"

	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/stretchr/testify/require"
)

func TestBuildPortPlanSlotZeroUsesCurrentDefaults(t *testing.T) {
	labCfg := config.DefaultLab()

	plan, err := BuildPortPlan(labCfg, 0)
	require.NoError(t, err)

	require.Equal(t, 0, plan.Slot)
	require.Equal(t, 8080, plan.LabBackend)
	require.Equal(t, 5173, plan.LabFrontend)
	require.Equal(t, 19280, plan.CommandCenter)
	require.Equal(t, 8123, plan.ClickHouseCBT01HTTP)
	require.Equal(t, 9000, plan.ClickHouseCBT01TCP)
	require.Equal(t, 8124, plan.ClickHouseCBT02HTTP)
	require.Equal(t, 9001, plan.ClickHouseCBT02TCP)
	require.Equal(t, 8125, plan.ClickHouseXatu01HTTP)
	require.Equal(t, 9002, plan.ClickHouseXatu01TCP)
	require.Equal(t, 8126, plan.ClickHouseXatu02HTTP)
	require.Equal(t, 9003, plan.ClickHouseXatu02TCP)
	require.Equal(t, 6380, plan.Redis)
	require.Equal(t, 9090, plan.Prometheus)
	require.Equal(t, 3000, plan.Grafana)

	mainnet := plan.Networks["mainnet"]
	require.Equal(t, 8081, mainnet.CBT)
	require.Equal(t, 8091, mainnet.CBTAPI)
	require.Equal(t, 8085, mainnet.CBTFrontend)
	require.Equal(t, 9100, mainnet.CBTMetrics)
	require.Equal(t, 9200, mainnet.CBTAPIMetrics)

	require.Empty(t, plan.DuplicatePorts())
}

func TestBuildPortPlanAppliesStrideAcrossFullPlan(t *testing.T) {
	labCfg := config.DefaultLab()

	slot0, err := BuildPortPlan(labCfg, 0)
	require.NoError(t, err)
	slot2, err := BuildPortPlan(labCfg, 2)
	require.NoError(t, err)

	require.Equal(t, slot0.LabBackend+2*portStride, slot2.LabBackend)
	require.Equal(t, slot0.CommandCenter+2*portStride, slot2.CommandCenter)
	require.Equal(t, slot0.ClickHouseCBT01HTTP+2*portStride, slot2.ClickHouseCBT01HTTP)
	require.Equal(t, slot0.ClickHouseCBT02TCP+2*portStride, slot2.ClickHouseCBT02TCP)
	require.Equal(t, slot0.ClickHouseXatu01HTTP+2*portStride, slot2.ClickHouseXatu01HTTP)
	require.Equal(t, slot0.ClickHouseXatu02TCP+2*portStride, slot2.ClickHouseXatu02TCP)
	require.Equal(t, slot0.Redis+2*portStride, slot2.Redis)
	require.Equal(t, slot0.Prometheus+2*portStride, slot2.Prometheus)
	require.Equal(t, slot0.Grafana+2*portStride, slot2.Grafana)

	require.Equal(t, slot0.Networks["mainnet"].CBT+2*portStride, slot2.Networks["mainnet"].CBT)
	require.Equal(t, slot0.Networks["mainnet"].CBTAPI+2*portStride, slot2.Networks["mainnet"].CBTAPI)
	require.Equal(t, slot0.Networks["mainnet"].CBTFrontend+2*portStride, slot2.Networks["mainnet"].CBTFrontend)
	require.Equal(t, slot0.Networks["mainnet"].CBTMetrics+2*portStride, slot2.Networks["mainnet"].CBTMetrics)
	require.Equal(t, slot0.Networks["mainnet"].CBTAPIMetrics+2*portStride, slot2.Networks["mainnet"].CBTAPIMetrics)
}
