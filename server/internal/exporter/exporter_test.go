package exporter

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"vgpu/internal/biz"
	"vgpu/internal/conf"
	"vgpu/internal/data/prom"
	"vgpu/internal/service"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
)

// ---- Fake repos ----

type fakePodRepo struct {
	containers []*biz.Container
}

func (f *fakePodRepo) ListAll(_ context.Context) ([]*biz.Container, error) {
	return f.containers, nil
}

func (f *fakePodRepo) FindOne(_ context.Context, _, _ string) (*biz.Container, error) {
	if len(f.containers) > 0 {
		return f.containers[0], nil
	}
	return nil, fmt.Errorf("not found")
}

type fakeNodeRepo struct {
	devices []*biz.DeviceInfo
}

func (f *fakeNodeRepo) ListAll(_ context.Context) ([]*biz.Node, error) {
	return nil, nil
}

func (f *fakeNodeRepo) GetNode(_ context.Context, _ string) (*biz.Node, error) {
	return nil, nil
}

func (f *fakeNodeRepo) ListAllDevices(_ context.Context) ([]*biz.DeviceInfo, error) {
	return f.devices, nil
}

func (f *fakeNodeRepo) FindDeviceByAliasId(_ string) (*biz.DeviceInfo, error) {
	return nil, nil
}

// ---- Mock Prometheus server ----

type mockPromHandler struct {
	responses map[string]string
}

func (h *mockPromHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/api/v1/query" {
		http.NotFound(w, r)
		return
	}
	query := r.URL.Query().Get("query")
	if r.Method == "POST" {
		query = r.PostFormValue("query")
	}
	if body, ok := h.responses[query]; ok {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	} else {
		// Return empty result for unknown queries
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[]}}`))
	}
}

func buildPromVectorResponse(samples []model.Sample) string {
	var result []map[string]interface{}
	for _, s := range samples {
		metric := make(map[string]string)
		for k, v := range s.Metric {
			metric[string(k)] = string(v)
		}
		result = append(result, map[string]interface{}{
			"metric": metric,
			"value":  []interface{}{float64(s.Timestamp.Unix()), s.Value.String()},
		})
	}
	wrapper := map[string]interface{}{
		"status": "success",
		"data": map[string]interface{}{
			"resultType": "vector",
			"result":     result,
		},
	}
	body, _ := json.Marshal(wrapper)
	return string(body)
}

// ---- Test helpers ----

func newTestMetricsGenerator(t *testing.T, promURL string, containers []*biz.Container, devices []*biz.DeviceInfo) *MetricsGenerator {
	t.Helper()
	promClient, err := prom.NewClient(promURL, time.Second*5, "")
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	podUC := biz.NewPodUseCase(&fakePodRepo{containers: containers}, log.DefaultLogger)
	nodeUC := biz.NewNodeUsecase(&fakeNodeRepo{devices: devices}, log.DefaultLogger)
	monitorSvc := service.NewMonitorService(promClient, nodeUC, podUC)
	gen := NewMetricsGenerator(
		&conf.Bootstrap{},
		promClient,
		nodeUC,
		podUC,
		monitorSvc,
	)
	return gen
}

func readMetricValue(metricName string, labels map[string]string) float64 {
	// Collect all metrics and find matching label set
	registry := prometheus.DefaultRegisterer.(*prometheus.Registry)
	families, err := registry.Gather()
	if err != nil {
		return -1
	}
	for _, f := range families {
		if f.GetName() != metricName {
			continue
		}
		for _, m := range f.GetMetric() {
			ml := m.GetLabel()
			if len(ml) != len(labels) {
				continue
			}
			match := true
			for _, l := range ml {
				if v, ok := labels[l.GetName()]; !ok || v != l.GetValue() {
					match = false
					break
				}
			}
			if match {
				return m.GetGauge().GetValue()
			}
		}
	}
	return -1
}

func readMetricAnyLabels(metricName string, partialLabels map[string]string) float64 {
	registry := prometheus.DefaultRegisterer.(*prometheus.Registry)
	families, err := registry.Gather()
	if err != nil {
		return -1
	}
	for _, f := range families {
		if f.GetName() != metricName {
			continue
		}
		for _, m := range f.GetMetric() {
			ml := m.GetLabel()
			match := true
			for _, l := range ml {
				if v, ok := partialLabels[l.GetName()]; ok && v != l.GetValue() {
					match = false
					break
				}
			}
			if match {
				return m.GetGauge().GetValue()
			}
		}
	}
	return -1
}

// reset clears all prometheus metrics between tests.
func resetTestMetrics() {
	reset()
}

func approxEqual(a, b, epsilon float64) bool {
	diff := a - b
	if diff < 0 {
		diff = -diff
	}
	return diff < epsilon
}

// ---- Real 910C device UUIDs (node=qh-jqxxpt-36, cart0-chip0, cart0-chip1) ----

const (
	chipUUID36Card0Chip0 = "6ECA2A6C-0100F7B1-2DBC9D33-C08A8B56-144301E3"
	chipUUID36Card0Chip1 = "6ECA2A6C-0120B7B1-2DBC9D33-C08A8B56-164301E3"
	chipUUID36Card1Chip0 = "77F0F664-0140E874-27098EF3-A50A8B56-144301E3"
	chipUUID36Card1Chip1 = "77F0F66C-01008874-29098EF3-A50A8B56-164301E3"
)

// Real values from hami-webui-metric.txt for qh-jqxxpt-36
// ascend-0 (card0-chip0): power=245, core_util_avg=30, temperature=46
// ascend-1 (card0-chip1): power=245.6, core_util_avg=29, temperature=44
// ascend-2 (card1-chip0): power=246.8, core_util_avg=27, temperature=45
// ascend-3 (card1-chip1): power=242.4, core_util_avg=27, temperature=44
//
// Expected merged results:
// card0: power=490.6 (sum), core_util_avg=29.5 (avg), temperature=46 (max)
// card1: power=489.2 (sum), core_util_avg=27 (avg), temperature=45 (max)

// ---- Integration Tests ----

func TestAscend910C_GenerateDeviceMetrics_Merged_MemUsed(t *testing.T) {
	mergedUUID := chipUUID36Card0Chip0 + "---" + chipUUID36Card0Chip1
	devices := []*biz.DeviceInfo{
		{
			Id:       mergedUUID,
			AliasId:  mergedUUID,
			Count:    1,
			Devmem:   65536,
			Devcore:  100,
			Type:     "Ascend910C",
			NodeName: "qh-jqxxpt-36",
			Provider: "Ascend",
			Health:   true,
		},
	}
	containers := []*biz.Container{}

	memUsedQ := fmt.Sprintf("avg(npu_chip_info_hbm_used_memory{vdie_id=\"%s\"})", chipUUID36Card0Chip0)
	memUsedQ2 := fmt.Sprintf("avg(npu_chip_info_hbm_used_memory{vdie_id=\"%s\"})", chipUUID36Card0Chip1)
	memTotalQ := fmt.Sprintf("avg(npu_chip_info_hbm_total_memory{vdie_id=\"%s\"})", chipUUID36Card0Chip0)
	memTotalQ2 := fmt.Sprintf("avg(npu_chip_info_hbm_total_memory{vdie_id=\"%s\"})", chipUUID36Card0Chip1)
	coreUtilQ := fmt.Sprintf("avg(npu_chip_info_utilization{vdie_id=\"%s\"})", chipUUID36Card0Chip0)
	coreUtilQ2 := fmt.Sprintf("avg(npu_chip_info_utilization{vdie_id=\"%s\"})", chipUUID36Card0Chip1)
	tempQ := fmt.Sprintf("avg(npu_chip_info_temperature{vdie_id=\"%s\"})", chipUUID36Card0Chip0)
	tempQ2 := fmt.Sprintf("avg(npu_chip_info_temperature{vdie_id=\"%s\"})", chipUUID36Card0Chip1)
	powerQ := fmt.Sprintf("avg(npu_chip_info_power{vdie_id=\"%s\"})", chipUUID36Card0Chip0)
	powerQ2 := fmt.Sprintf("avg(npu_chip_info_power{vdie_id=\"%s\"})", chipUUID36Card0Chip1)

	// Additional query for device info uses first chip only
	additionalQ := fmt.Sprintf("npu_chip_info_power{vdie_id=\"%s\"}", chipUUID36Card0Chip0)

	// Health query
	healthQ := fmt.Sprintf("npu_chip_info_health_status{node=\"qh-jqxxpt-36\"}")

	now := model.Now()
	mockResponses := map[string]string{
		memUsedQ: buildPromVectorResponse([]model.Sample{
			{Metric: model.Metric{"vdie_id": model.LabelValue(chipUUID36Card0Chip0)}, Value: 32000, Timestamp: now},
		}),
		memUsedQ2: buildPromVectorResponse([]model.Sample{
			{Metric: model.Metric{"vdie_id": model.LabelValue(chipUUID36Card0Chip1)}, Value: 30000, Timestamp: now},
		}),
		memTotalQ: buildPromVectorResponse([]model.Sample{
			{Metric: model.Metric{"vdie_id": model.LabelValue(chipUUID36Card0Chip0)}, Value: 65536, Timestamp: now},
		}),
		memTotalQ2: buildPromVectorResponse([]model.Sample{
			{Metric: model.Metric{"vdie_id": model.LabelValue(chipUUID36Card0Chip1)}, Value: 65536, Timestamp: now},
		}),
		coreUtilQ: buildPromVectorResponse([]model.Sample{
			{Metric: model.Metric{"vdie_id": model.LabelValue(chipUUID36Card0Chip0)}, Value: 30, Timestamp: now},
		}),
		coreUtilQ2: buildPromVectorResponse([]model.Sample{
			{Metric: model.Metric{"vdie_id": model.LabelValue(chipUUID36Card0Chip1)}, Value: 29, Timestamp: now},
		}),
		tempQ: buildPromVectorResponse([]model.Sample{
			{Metric: model.Metric{"vdie_id": model.LabelValue(chipUUID36Card0Chip0)}, Value: 46, Timestamp: now},
		}),
		tempQ2: buildPromVectorResponse([]model.Sample{
			{Metric: model.Metric{"vdie_id": model.LabelValue(chipUUID36Card0Chip1)}, Value: 44, Timestamp: now},
		}),
		powerQ: buildPromVectorResponse([]model.Sample{
			{Metric: model.Metric{"vdie_id": model.LabelValue(chipUUID36Card0Chip0)}, Value: 245, Timestamp: now},
		}),
		powerQ2: buildPromVectorResponse([]model.Sample{
			{Metric: model.Metric{"vdie_id": model.LabelValue(chipUUID36Card0Chip1)}, Value: 245.6, Timestamp: now},
		}),
		additionalQ: buildPromVectorResponse([]model.Sample{
			{
				Metric: model.Metric{
					"vdie_id": model.LabelValue(chipUUID36Card0Chip0),
					"id":      model.LabelValue("0"),
				},
				Value: 245, Timestamp: now,
			},
		}),
		healthQ: buildPromVectorResponse([]model.Sample{
			{Metric: model.Metric{"id": "0", "vdie_id": model.LabelValue(chipUUID36Card0Chip0)}, Value: 1, Timestamp: now},
			{Metric: model.Metric{"id": "1", "vdie_id": model.LabelValue(chipUUID36Card0Chip1)}, Value: 1, Timestamp: now},
		}),
	}

	mux := http.NewServeMux()
	mux.Handle("/api/v1/query", &mockPromHandler{responses: mockResponses})
	server := httptest.NewServer(mux)
	defer server.Close()

	gen := newTestMetricsGenerator(t, server.URL, containers, devices)
	resetTestMetrics()

	err := gen.GenerateMetrics(context.Background())
	if err != nil {
		t.Fatalf("GenerateMetrics failed: %v", err)
	}

	// mem_used = 32000 + 30000 = 62000
	if want, got := float64(62000), readMetricAnyLabels("hami_memory_used", map[string]string{"deviceuuid": mergedUUID}); want != got {
		t.Errorf("hami_memory_used: want %v, got %v", want, got)
	}

	// mem_total via virtual memory size = Devmem (65536)
	if want, got := float64(65536), readMetricAnyLabels("hami_vmemory_size", map[string]string{"deviceuuid": mergedUUID}); want != got {
		t.Errorf("hami_vmemory_size: want %v, got %v", want, got)
	}

	// core_used (util) = avg(30, 29) = 29.5
	if want, got := float64(29.5), readMetricAnyLabels("hami_core_used", map[string]string{"deviceuuid": mergedUUID}); !approxEqual(want, got, 0.1) {
		t.Errorf("hami_core_used: want %v, got %v", want, got)
	}

	// temperature = max(46, 44) = 46
	if want, got := float64(46), readMetricAnyLabels("hami_device_temperature", map[string]string{"deviceuuid": mergedUUID}); want != got {
		t.Errorf("hami_device_temperature: want %v, got %v", want, got)
	}

	// power = 245 + 245.6 = 490.6
	if want, got := float64(490.6), readMetricAnyLabels("hami_device_power", map[string]string{"deviceuuid": mergedUUID}); !approxEqual(want, got, 0.1) {
		t.Errorf("hami_device_power: want %v, got %v", want, got)
	}

	// vgpu count should be present (count=1)
	if got := readMetricAnyLabels("hami_vgpu_count", map[string]string{"deviceuuid": mergedUUID}); got != 1 {
		t.Errorf("hami_vgpu_count: want 1, got %v", got)
	}
}

func TestAscend910C_GenerateDeviceMetrics_Merged_OddChip(t *testing.T) {
	mergedUUID0 := chipUUID36Card0Chip0 + "---" + chipUUID36Card0Chip1
	// Only 3 chips: should merge first two, leave third as-is
	chip2UUID := chipUUID36Card1Chip0
	devices := []*biz.DeviceInfo{
		{
			Id: mergedUUID0, AliasId: mergedUUID0,
			Count: 1, Devmem: 65536, Devcore: 100,
			Type: "Ascend910C", NodeName: "qh-jqxxpt-36", Provider: "Ascend", Health: true,
		},
		{
			Id: chip2UUID, AliasId: chip2UUID,
			Count: 1, Devmem: 65536, Devcore: 100,
			Type: "Ascend910C", NodeName: "qh-jqxxpt-36", Provider: "Ascend", Health: true,
		},
	}
	containers := []*biz.Container{}

	memUsedQ0 := fmt.Sprintf("avg(npu_chip_info_hbm_used_memory{vdie_id=\"%s\"})", chipUUID36Card0Chip0)
	memUsedQ1 := fmt.Sprintf("avg(npu_chip_info_hbm_used_memory{vdie_id=\"%s\"})", chipUUID36Card0Chip1)
	memTotalQ0 := fmt.Sprintf("avg(npu_chip_info_hbm_total_memory{vdie_id=\"%s\"})", chipUUID36Card0Chip0)
	memTotalQ1 := fmt.Sprintf("avg(npu_chip_info_hbm_total_memory{vdie_id=\"%s\"})", chipUUID36Card0Chip1)
	healthQ := fmt.Sprintf("npu_chip_info_health_status{node=\"qh-jqxxpt-36\"}")

	now := model.Now()
	mockResponses := map[string]string{
		memUsedQ0: buildPromVectorResponse([]model.Sample{
			{Metric: model.Metric{"vdie_id": model.LabelValue(chipUUID36Card0Chip0)}, Value: 32000, Timestamp: now},
		}),
		memUsedQ1: buildPromVectorResponse([]model.Sample{
			{Metric: model.Metric{"vdie_id": model.LabelValue(chipUUID36Card0Chip1)}, Value: 30000, Timestamp: now},
		}),
		memTotalQ0: buildPromVectorResponse([]model.Sample{
			{Metric: model.Metric{"vdie_id": model.LabelValue(chipUUID36Card0Chip0)}, Value: 65536, Timestamp: now},
		}),
		memTotalQ1: buildPromVectorResponse([]model.Sample{
			{Metric: model.Metric{"vdie_id": model.LabelValue(chipUUID36Card0Chip1)}, Value: 65536, Timestamp: now},
		}),
		healthQ: buildPromVectorResponse([]model.Sample{
			{Metric: model.Metric{"id": "0", "vdie_id": model.LabelValue(chipUUID36Card0Chip0)}, Value: 1, Timestamp: now},
			{Metric: model.Metric{"id": "1", "vdie_id": model.LabelValue(chipUUID36Card0Chip1)}, Value: 1, Timestamp: now},
		}),
		// chip2 (unpaired) queries — no extra mock needed, will return empty
	}

	mux := http.NewServeMux()
	mux.Handle("/api/v1/query", &mockPromHandler{responses: mockResponses})
	server := httptest.NewServer(mux)
	defer server.Close()

	gen := newTestMetricsGenerator(t, server.URL, containers, devices)
	resetTestMetrics()

	err := gen.GenerateMetrics(context.Background())
	if err != nil {
		t.Fatalf("GenerateMetrics failed: %v", err)
	}

	// Merged device should be present
	if got := readMetricAnyLabels("hami_memory_used", map[string]string{"deviceuuid": mergedUUID0}); got <= 0 {
		t.Errorf("merged device should have memory_used > 0, got %v", got)
	}
}

func TestAscend910C_GenerateContainerMetrics_Merged_MatchAlias(t *testing.T) {
	mergedUUID := chipUUID36Card0Chip0 + "---" + chipUUID36Card0Chip1
	devices := []*biz.DeviceInfo{
		{
			Id:       mergedUUID,
			AliasId:  mergedUUID,
			Count:    1,
			Devmem:   65536,
			Devcore:  100,
			Type:     "Ascend910C",
			NodeName: "qh-jqxxpt-36",
			Provider: "Ascend",
			Health:   true,
		},
	}
	containers := []*biz.Container{
		{
			Name:      "inference",
			PodName:   "ascend-pod",
			Namespace: "default",
			PodUID:    "pod-ascend-1",
			NodeName:  "qh-jqxxpt-36",
			ContainerDevices: biz.ContainerDevices{
				// Container allocated to chip0 of merged device
				{UUID: chipUUID36Card0Chip0, Type: "Ascend910C", Usedmem: 16384, Usedcores: 50},
			},
		},
	}

	now := model.Now()
	healthQ := fmt.Sprintf("npu_chip_info_health_status{node=\"qh-jqxxpt-36\"}")
	mockResponses := map[string]string{
		healthQ: buildPromVectorResponse([]model.Sample{
			{Metric: model.Metric{"id": "0", "vdie_id": model.LabelValue(chipUUID36Card0Chip0)}, Value: 1, Timestamp: now},
			{Metric: model.Metric{"id": "1", "vdie_id": model.LabelValue(chipUUID36Card0Chip1)}, Value: 1, Timestamp: now},
		}),
		// taskCoreUsed query — pod+container based, not vdie_id based
		fmt.Sprintf("avg(container_npu_utilization{exported_namespace=\"default\", pod_name=\"ascend-pod\", container_name=\"inference\"})"): buildPromVectorResponse([]model.Sample{
			{Value: 60, Timestamp: now},
		}),
		// deviceMemTotal for core_util calculation
		"avg(npu_chip_info_hbm_total_memory{vdie_id=\"" + chipUUID36Card0Chip0 + "\"})": buildPromVectorResponse([]model.Sample{
			{Value: 65536, Timestamp: now},
		}),
		"avg(npu_chip_info_hbm_total_memory{vdie_id=\"" + chipUUID36Card0Chip1 + "\"})": buildPromVectorResponse([]model.Sample{
			{Value: 65536, Timestamp: now},
		}),
		// taskMemoryUsed query
		fmt.Sprintf("avg(container_npu_used_memory{exported_namespace=\"default\", pod_name=\"ascend-pod\", container_name=\"inference\"})"): buildPromVectorResponse([]model.Sample{
			{Value: 2048, Timestamp: now},
		}),
	}

	mux := http.NewServeMux()
	mux.Handle("/api/v1/query", &mockPromHandler{responses: mockResponses})
	server := httptest.NewServer(mux)
	defer server.Close()

	gen := newTestMetricsGenerator(t, server.URL, containers, devices)
	resetTestMetrics()

	err := gen.GenerateContainerMetrics(context.Background())
	if err != nil {
		t.Fatalf("GenerateContainerMetrics failed: %v", err)
	}

	// Container allocated to chip0 should match merged device via AliasId
	// Allocation metrics should use the merged device UUID
	if got := readMetricAnyLabels("hami_container_vgpu_allocated", map[string]string{
		"container_name": "inference",
		"pod_name":       "ascend-pod",
		"namespace_name": "default",
	}); got != 1 {
		t.Errorf("hami_container_vgpu_allocated: want 1, got %v", got)
	}
	if got := readMetricAnyLabels("hami_container_vmemory_allocated", map[string]string{
		"container_name": "inference",
		"pod_name":       "ascend-pod",
		"namespace_name": "default",
	}); got != 16384 {
		t.Errorf("hami_container_vmemory_allocated: want 16384, got %v", got)
	}

	// 910C is a non-split model — should use fallback (taskCoreUsed/taskMemoryUsed),
	// not proportional split. Verify core/memory metrics ARE set via fallback.
	// deviceMemTotal = 65536+65536 = 131072, perc = 16384/131072 = 0.125, core = 12
	// taskCoreUsed = 60 (from container_npu_utilization)
	if got := readMetricAnyLabels("hami_container_core_used", map[string]string{
		"container_name": "inference",
		"pod_name":       "ascend-pod",
		"namespace_name": "default",
	}); got != 60 {
		t.Errorf("hami_container_core_used: want 60 (fallback, not proportional split), got %v", got)
	}
	// taskMemoryUsed = 2048 MB (from container_npu_used_memory)
	if got := readMetricAnyLabels("hami_container_memory_used", map[string]string{
		"container_name": "inference",
		"pod_name":       "ascend-pod",
		"namespace_name": "default",
	}); got != 2048 {
		t.Errorf("hami_container_memory_used: want 2048 (fallback, not proportional split), got %v", got)
	}
}

// ---- Ascend vnpu Container Metrics Tests ----

func TestAscend910B_Proportional_SingleContainer_Success(t *testing.T) {
	// Single container on card — proportional split with ratio=1.0
	// Should use pre-calculated card-level metrics (deviceCoreUtil).
	devices := []*biz.DeviceInfo{
		{
			Id: "npu-uuid-1", AliasId: "npu-uuid-1",
			Count: 1, Devmem: 65536, Devcore: 100,
			Type: "Ascend910B", NodeName: "node-1", Provider: "Ascend", Health: true,
		},
	}
	// Usedmem=11264, Devmem=65536 → perc=11264/65536 → adjusted core=int32(100*perc)=17
	containers := []*biz.Container{
		{
			Name: "ctr", PodName: "pod-1", Namespace: "ns-1", PodUID: "pod-uid-1", NodeName: "node-1",
			ContainerDevices: biz.ContainerDevices{
				{UUID: "npu-uuid-1", Type: "Ascend910B", Usedmem: 11264, Usedcores: 25},
			},
		},
	}

	now := model.Now()
	totalQ := fmt.Sprintf("avg(npu_chip_info_hbm_total_memory{vdie_id=\"%s\"})", "npu-uuid-1")
	coreUtilQ := fmt.Sprintf("avg(npu_chip_info_utilization{vdie_id=\"%s\"})", "npu-uuid-1")
	memUsedQ := fmt.Sprintf("avg(npu_chip_info_hbm_used_memory{vdie_id=\"%s\"})", "npu-uuid-1")

	mockResponses := map[string]string{
		totalQ: buildPromVectorResponse([]model.Sample{
			{Value: 65536, Timestamp: now},
		}),
		coreUtilQ: buildPromVectorResponse([]model.Sample{
			{Value: 30, Timestamp: now},
		}),
		memUsedQ: buildPromVectorResponse([]model.Sample{
			{Value: 14305.1, Timestamp: now},
		}),
	}

	mux := http.NewServeMux()
	mux.Handle("/api/v1/query", &mockPromHandler{responses: mockResponses})
	server := httptest.NewServer(mux)
	defer server.Close()

	gen := newTestMetricsGenerator(t, server.URL, containers, devices)
	resetTestMetrics()
	err := gen.GenerateContainerMetrics(context.Background())
	if err != nil {
		t.Fatalf("GenerateContainerMetrics failed: %v", err)
	}

	// ratio=11264/11264=1.0, cardUtil=30
	// core_used = 30 × 1.0 = 30, core_util = core_used = 30
	wantUsed := float64(30)
	wantUtil := float64(30)
	if got := readMetricAnyLabels("hami_container_core_used", map[string]string{
		"pod_name": "pod-1", "container_name": "ctr", "namespace_name": "ns-1",
	}); got != wantUsed {
		t.Errorf("hami_container_core_used: want %v, got %v", wantUsed, got)
	}
	if got := readMetricAnyLabels("hami_container_core_util", map[string]string{
		"pod_name": "pod-1", "container_name": "ctr", "namespace_name": "ns-1",
	}); !approxEqual(wantUtil, got, 0.1) {
		t.Errorf("hami_container_core_util: want ~%v (=core_used), got %v", wantUtil, got)
	}
}

func TestAscend910B_Proportional_ZeroFallback_ToCardMetric(t *testing.T) {
	// Single container on card — cardUtil returns non-zero, should be used
	// (not 0 from vnpu path which is no longer relevant)
	devices := []*biz.DeviceInfo{
		{
			Id: "npu-uuid-1", AliasId: "npu-uuid-1",
			Count: 1, Devmem: 65536, Devcore: 100,
			Type: "Ascend910B", NodeName: "node-1", Provider: "Ascend", Health: true,
		},
	}
	containers := []*biz.Container{
		{
			Name: "ctr", PodName: "pod-1", Namespace: "ns-1", PodUID: "pod-uid-1", NodeName: "node-1",
			ContainerDevices: biz.ContainerDevices{
				{UUID: "npu-uuid-1", Type: "Ascend910B", Usedmem: 11264, Usedcores: 25},
			},
		},
	}

	now := model.Now()
	totalQ := fmt.Sprintf("avg(npu_chip_info_hbm_total_memory{vdie_id=\"%s\"})", "npu-uuid-1")
	coreUtilQ := fmt.Sprintf("avg(npu_chip_info_utilization{vdie_id=\"%s\"})", "npu-uuid-1")
	memUsedQ := fmt.Sprintf("avg(npu_chip_info_hbm_used_memory{vdie_id=\"%s\"})", "npu-uuid-1")

	mockResponses := map[string]string{
		totalQ: buildPromVectorResponse([]model.Sample{
			{Value: 65536, Timestamp: now},
		}),
		coreUtilQ: buildPromVectorResponse([]model.Sample{
			{Value: 30, Timestamp: now},
		}),
		memUsedQ: buildPromVectorResponse([]model.Sample{
			{Value: 14305.1, Timestamp: now},
		}),
	}

	mux := http.NewServeMux()
	mux.Handle("/api/v1/query", &mockPromHandler{responses: mockResponses})
	server := httptest.NewServer(mux)
	defer server.Close()

	gen := newTestMetricsGenerator(t, server.URL, containers, devices)
	resetTestMetrics()
	err := gen.GenerateContainerMetrics(context.Background())
	if err != nil {
		t.Fatalf("GenerateContainerMetrics failed: %v", err)
	}

	// Pre-calculated: cardUtil=30, ratio=1.0 → core_used=30 (not 0)
	// Adjusted core=17
	if got := readMetricAnyLabels("hami_container_core_used", map[string]string{
		"pod_name": "pod-1", "container_name": "ctr", "namespace_name": "ns-1",
	}); got != 30 {
		t.Errorf("hami_container_core_used: want 30 (cardUtil=30*ratio=1), got %v", got)
	}
}

func TestAscend_vnpu_taskCoreUsed_Empty_Fallback(t *testing.T) {
	// Vnpu query returns no data (non-split scenario), fallback to container_npu_utilization
	devices := []*biz.DeviceInfo{
		{
			Id: "npu-uuid-1", AliasId: "npu-uuid-1",
			Count: 1, Devmem: 65536, Devcore: 100,
			Type: "Ascend910B", NodeName: "node-1", Provider: "Ascend", Health: true,
		},
	}
	// Usedmem=33792, Devmem=65536 → adjusted core=int32(100*33792/65536)=51
	containers := []*biz.Container{
		{
			Name: "ctr", PodName: "pod-1", Namespace: "ns-1", PodUID: "pod-uid-1", NodeName: "node-1",
			ContainerDevices: biz.ContainerDevices{
				{UUID: "npu-uuid-1", Type: "Ascend910B", Usedmem: 33792, Usedcores: 100},
			},
		},
	}

	now := model.Now()
	totalQ := fmt.Sprintf("avg(npu_chip_info_hbm_total_memory{vdie_id=\"%s\"})", "npu-uuid-1")
	coreUtilQ := fmt.Sprintf("avg(npu_chip_info_utilization{vdie_id=\"%s\"})", "npu-uuid-1")

	mockResponses := map[string]string{
		// vnpu query returns empty result
		fmt.Sprintf("avg(vnpu_pod_aicore_utilization{exported_namespace=\"%s\", pod_name=\"%s\", container_name=\"%s\"})", "ns-1", "pod-1", "ctr"): buildPromVectorResponse(nil), // empty result
		// container query (fallback) has data
		fmt.Sprintf("avg(container_npu_utilization{exported_namespace=\"%s\", pod_name=\"%s\", container_name=\"%s\"})", "ns-1", "pod-1", "ctr"): buildPromVectorResponse([]model.Sample{
			{Value: 50, Timestamp: now},
		}),
		totalQ: buildPromVectorResponse([]model.Sample{
			{Value: 65536, Timestamp: now},
		}),
		coreUtilQ: buildPromVectorResponse([]model.Sample{
			{Value: 50, Timestamp: now},
		}),
	}

	mux := http.NewServeMux()
	mux.Handle("/api/v1/query", &mockPromHandler{responses: mockResponses})
	server := httptest.NewServer(mux)
	defer server.Close()

	gen := newTestMetricsGenerator(t, server.URL, containers, devices)
	resetTestMetrics()
	err := gen.GenerateContainerMetrics(context.Background())
	if err != nil {
		t.Fatalf("GenerateContainerMetrics failed: %v", err)
	}

	// Fallback to container value: core_used = 50, core_util = 100 * 50 / 51 ≈ 98.0
	// 比例拆分：cardUtil=50, ratio=33792/33792=1.0
	// core_used = 50 × 1.0 = 50, core_util = core_used = 50
	wantUsed := float64(50)
	wantUtil := float64(50)
	if got := readMetricAnyLabels("hami_container_core_used", map[string]string{
		"pod_name": "pod-1", "container_name": "ctr", "namespace_name": "ns-1",
	}); !approxEqual(wantUsed, got, 0.1) {
		t.Errorf("hami_container_core_used: want %v, got %v", wantUsed, got)
	}
	if got := readMetricAnyLabels("hami_container_core_util", map[string]string{
		"pod_name": "pod-1", "container_name": "ctr", "namespace_name": "ns-1",
	}); !approxEqual(wantUtil, got, 0.1) {
		t.Errorf("hami_container_core_util: want ~%v (=core_used), got %v", wantUtil, got)
	}
}

func TestAscend910B_Proportional_MultiContainer_CoreMemory(t *testing.T) {
	// Two containers with different allocations on same card
	// ctr-a: 16384 MB (25%), ctr-b: 32768 MB (50%), total=49152 MB
	// Verify proportional split with unequal allocations
	deviceUUID := "npu-multi-1"
	devices := []*biz.DeviceInfo{
		{
			Id: deviceUUID, AliasId: deviceUUID,
			Count: 1, Devmem: 65536, Devcore: 100,
			Type: "Ascend910B", NodeName: "node-1", Provider: "Ascend", Health: true,
		},
	}
	containers := []*biz.Container{
		{
			Name: "ctr-a", PodName: "pod-a", Namespace: "ns-1", PodUID: "pod-uid-a", NodeName: "node-1",
			ContainerDevices: biz.ContainerDevices{
				{UUID: deviceUUID, Type: "Ascend910B", Usedmem: 16384, Usedcores: 25},
			},
		},
		{
			Name: "ctr-b", PodName: "pod-b", Namespace: "ns-1", PodUID: "pod-uid-b", NodeName: "node-1",
			ContainerDevices: biz.ContainerDevices{
				{UUID: deviceUUID, Type: "Ascend910B", Usedmem: 32768, Usedcores: 50},
			},
		},
	}

	now := model.Now()
	coreUtilQ := fmt.Sprintf("avg(npu_chip_info_utilization{vdie_id=\"%s\"})", deviceUUID)
	memUsedQ := fmt.Sprintf("avg(npu_chip_info_hbm_used_memory{vdie_id=\"%s\"})", deviceUUID)
	memTotalQ := fmt.Sprintf("avg(npu_chip_info_hbm_total_memory{vdie_id=\"%s\"})", deviceUUID)

	// Card util=90, card mem=28610.2 MB (~28 GB)
	// totalMemoryOnCard = 16384+32768 = 49152
	// ctr-a: ratio=16384/49152≈0.333, core_used=90*0.333≈30, mem_used=28610.2*0.333≈9529 MB
	// ctr-b: ratio=32768/49152≈0.667, core_used=90*0.667≈60, mem_used=28610.2*0.667≈19073 MB
	mockResponses := map[string]string{
		coreUtilQ: buildPromVectorResponse([]model.Sample{
			{Value: 90, Timestamp: now},
		}),
		memUsedQ: buildPromVectorResponse([]model.Sample{
			{Value: 28610.2, Timestamp: now},
		}),
		memTotalQ: buildPromVectorResponse([]model.Sample{
			{Value: 65536, Timestamp: now},
		}),
	}

	mux := http.NewServeMux()
	mux.Handle("/api/v1/query", &mockPromHandler{responses: mockResponses})
	server := httptest.NewServer(mux)
	defer server.Close()

	gen := newTestMetricsGenerator(t, server.URL, containers, devices)
	resetTestMetrics()

	err := gen.GenerateContainerMetrics(context.Background())
	if err != nil {
		t.Fatalf("GenerateContainerMetrics failed: %v", err)
	}

	labelsA := map[string]string{"pod_name": "pod-a", "container_name": "ctr-a", "namespace_name": "ns-1"}
	labelsB := map[string]string{"pod_name": "pod-b", "container_name": "ctr-b", "namespace_name": "ns-1"}

	// ctr-a: core_used = 90 * (16384/49152) = 30
	if got := readMetricAnyLabels("hami_container_core_used", labelsA); !approxEqual(30, got, 1.0) {
		t.Errorf("ctr-a core_used: want ~30, got %v", got)
	}
	// ctr-b: core_used = 90 * (32768/49152) = 60
	if got := readMetricAnyLabels("hami_container_core_used", labelsB); !approxEqual(60, got, 1.0) {
		t.Errorf("ctr-b core_used: want ~60, got %v", got)
	}

	// ctr-a: memory_used = 28610.2 * 0.333 = 9529 MB
	wantMemA := roundToOneDecimal(float64(30000000000) / 1024 / 1024 * float64(16384) / float64(49152))
	if got := readMetricAnyLabels("hami_container_memory_used", labelsA); !approxEqual(wantMemA, got, 1.0) {
		t.Errorf("ctr-a memory_used: want ~%v, got %v", wantMemA, got)
	}
	// ctr-b: memory_used = 28610.2 * 0.667 = 19073 MB
	wantMemB := roundToOneDecimal(float64(30000000000) / 1024 / 1024 * float64(32768) / float64(49152))
	if got := readMetricAnyLabels("hami_container_memory_used", labelsB); !approxEqual(wantMemB, got, 1.0) {
		t.Errorf("ctr-b memory_used: want ~%v, got %v", wantMemB, got)
	}

	// Sum of memory should equal full card memory
	if got := readMetricAnyLabels("hami_container_memory_used", labelsA) + readMetricAnyLabels("hami_container_memory_used", labelsB); !approxEqual(float64(30000000000)/1024/1024, got, 2.0) {
		t.Errorf("memory_used sum: want ~%v, got %v (sum=%v+%v)", float64(30000000000)/1024/1024, got,
			readMetricAnyLabels("hami_container_memory_used", labelsA),
			readMetricAnyLabels("hami_container_memory_used", labelsB))
	}
}

func TestAscend910C_NonAscendDevice_Unchanged(t *testing.T) {
	// Ascend310P devices should NOT be merged
	devices := []*biz.DeviceInfo{
		{
			Id: "chip-uuid-1", AliasId: "chip-uuid-1",
			Count: 1, Devmem: 16384, Devcore: 25,
			Type: "Ascend310P", NodeName: "node-1", Provider: "Ascend", Health: true,
		},
		{
			Id: "chip-uuid-2", AliasId: "chip-uuid-2",
			Count: 1, Devmem: 16384, Devcore: 25,
			Type: "Ascend310P", NodeName: "node-1", Provider: "Ascend", Health: true,
		},
	}
	containers := []*biz.Container{}

	now := model.Now()
	healthQ := fmt.Sprintf("npu_chip_info_health_status{node=\"node-1\"}")
	mockResponses := map[string]string{
		healthQ: buildPromVectorResponse([]model.Sample{
			{Metric: model.Metric{"id": "0", "vdie_id": "chip-uuid-1"}, Value: 1, Timestamp: now},
			{Metric: model.Metric{"id": "1", "vdie_id": "chip-uuid-2"}, Value: 1, Timestamp: now},
		}),
	}

	mux := http.NewServeMux()
	mux.Handle("/api/v1/query", &mockPromHandler{responses: mockResponses})
	server := httptest.NewServer(mux)
	defer server.Close()

	gen := newTestMetricsGenerator(t, server.URL, containers, devices)
	resetTestMetrics()

	err := gen.GenerateMetrics(context.Background())
	if err != nil {
		t.Fatalf("GenerateMetrics failed: %v", err)
	}

	// Both Ascend310P devices should still be present separately
	// Use partial label match since provider may be empty in test
	if got := readMetricAnyLabels("hami_vgpu_count", map[string]string{"deviceuuid": "chip-uuid-1"}); got != 1 {
		t.Errorf("Ascend310P chip1 not found or count wrong: got %v", got)
	}
	if got := readMetricAnyLabels("hami_vgpu_count", map[string]string{"deviceuuid": "chip-uuid-2"}); got != 1 {
		t.Errorf("Ascend310P chip2 not found or count wrong: got %v", got)
	}
}

func TestAscend910C_DeviceInfo_MatchAlias(t *testing.T) {
	mergedUUID := chipUUID36Card0Chip0 + "---" + chipUUID36Card0Chip1
	d := &biz.DeviceInfo{
		AliasId: mergedUUID,
		Type:    "Ascend910C",
	}

	if !d.MatchAlias(chipUUID36Card0Chip0) {
		t.Error("MatchAlias should match chip0 UUID of merged device")
	}
	if !d.MatchAlias(chipUUID36Card0Chip1) {
		t.Error("MatchAlias should match chip1 UUID of merged device")
	}
	if d.MatchAlias("unrelated-uuid") {
		t.Error("MatchAlias should NOT match unrelated UUID")
	}
	if !d.ExactAlias(mergedUUID) {
		t.Error("ExactAlias should match the full combined AliasId")
	}
}

func TestAscend910C_GenerateDeviceMetrics_QueryFails_Graceful(t *testing.T) {
	mergedUUID := chipUUID36Card0Chip0 + "---" + chipUUID36Card0Chip1
	devices := []*biz.DeviceInfo{
		{
			Id: mergedUUID, AliasId: mergedUUID,
			Count: 1, Devmem: 65536, Devcore: 100,
			Type: "Ascend910C", NodeName: "qh-jqxxpt-36", Provider: "Ascend", Health: true,
		},
	}
	containers := []*biz.Container{}

	// No mock at all — all queries return empty
	mux := http.NewServeMux()
	mux.Handle("/api/v1/query", &mockPromHandler{responses: map[string]string{}})
	server := httptest.NewServer(mux)
	defer server.Close()

	gen := newTestMetricsGenerator(t, server.URL, containers, devices)
	resetTestMetrics()

	err := gen.GenerateMetrics(context.Background())
	if err != nil {
		t.Fatalf("GenerateMetrics should not fail on empty mock data: %v", err)
	}

	// Device metrics should still be present with 0 values
	if got := readMetricAnyLabels("hami_vgpu_count", map[string]string{"deviceuuid": mergedUUID}); got != 1 {
		t.Errorf("hami_vgpu_count should be 1 even on empty query, got %v", got)
	}
}

// ---- PPU Device Metrics Tests ----
// PPU uses DCGM-compatible metric names identical to NVIDIA.

func TestPPU_GenerateDeviceMetrics_MemoryAndCore(t *testing.T) {
	uuid := "GPU-ppu-0001-1234-5678-9abcdef01234"
	devices := []*biz.DeviceInfo{
		{
			Id:       uuid,
			AliasId:  uuid,
			Count:    1,
			Devmem:   98304,
			Devcore:  100,
			Type:     "PPU",
			NodeName: "ppu-node-1",
			Provider: "PPU",
			Health:   true,
		},
	}
	containers := []*biz.Container{}

	now := model.Now()
	memUsedQ := fmt.Sprintf("avg(DCGM_FI_DEV_FB_USED{UUID=\"%s\"})", uuid)
	coreUtilQ := fmt.Sprintf("DCGM_FI_DEV_GPU_UTIL{UUID=\"%s\"}", uuid)

	mockResponses := map[string]string{
		memUsedQ: buildPromVectorResponse([]model.Sample{
			{Metric: model.Metric{"UUID": model.LabelValue(uuid)}, Value: 45000, Timestamp: now},
		}),
		coreUtilQ: buildPromVectorResponse([]model.Sample{
			{Value: 35, Timestamp: now},
		}),
	}

	mux := http.NewServeMux()
	mux.Handle("/api/v1/query", &mockPromHandler{responses: mockResponses})
	server := httptest.NewServer(mux)
	defer server.Close()

	gen := newTestMetricsGenerator(t, server.URL, containers, devices)
	resetTestMetrics()

	err := gen.GenerateMetrics(context.Background())
	if err != nil {
		t.Fatalf("GenerateMetrics failed: %v", err)
	}

	if got := readMetricAnyLabels("hami_memory_used", map[string]string{"deviceuuid": uuid}); got != 45000 {
		t.Errorf("hami_memory_used: want 45000, got %v", got)
	}
	if got := readMetricAnyLabels("hami_core_used", map[string]string{"deviceuuid": uuid}); got != 35 {
		t.Errorf("hami_core_used: want 35, got %v", got)
	}
	if got := readMetricAnyLabels("hami_core_util", map[string]string{"deviceuuid": uuid}); got != 35 {
		t.Errorf("hami_core_util: want 35, got %v", got)
	}
	if got := readMetricAnyLabels("hami_vgpu_count", map[string]string{"deviceuuid": uuid}); got != 1 {
		t.Errorf("hami_vgpu_count: want 1, got %v", got)
	}
}

func TestPPU_GenerateDeviceMetrics_TempPowerHealth(t *testing.T) {
	uuid := "GPU-ppu-0002-1234-5678-9abcdef01235"
	devices := []*biz.DeviceInfo{
		{
			Id:       uuid,
			AliasId:  uuid,
			Count:    1,
			Devmem:   98304,
			Devcore:  100,
			Type:     "PPU",
			NodeName: "ppu-node-1",
			Provider: "PPU",
			Health:   true,
		},
	}
	containers := []*biz.Container{}

	now := model.Now()
	tempQ := fmt.Sprintf("avg(DCGM_FI_DEV_GPU_TEMP{UUID=\"%s\"})", uuid)
	memTempQ := fmt.Sprintf("avg(DCGM_FI_DEV_MEMORY_TEMP{UUID=\"%s\"})", uuid)
	powerQ := fmt.Sprintf("avg(DCGM_FI_DEV_POWER_USAGE{UUID=\"%s\"})", uuid)
	xidQ := fmt.Sprintf("avg(DCGM_FI_DEV_XID_ERRORS{UUID=\"%s\"})", uuid)
	additionalQ := fmt.Sprintf("DCGM_FI_DEV_POWER_USAGE{UUID=\"%s\"}", uuid)

	mockResponses := map[string]string{
		tempQ: buildPromVectorResponse([]model.Sample{
			{Value: 72, Timestamp: now},
		}),
		memTempQ: buildPromVectorResponse([]model.Sample{
			{Value: 58, Timestamp: now},
		}),
		powerQ: buildPromVectorResponse([]model.Sample{
			{
				Metric: model.Metric{
					"UUID":   model.LabelValue(uuid),
					"device": "ppu0",
				},
				Value: 280, Timestamp: now,
			},
		}),
		additionalQ: buildPromVectorResponse([]model.Sample{
			{
				Metric: model.Metric{
					"UUID":   model.LabelValue(uuid),
					"device": "ppu0",
				},
				Value: 280, Timestamp: now,
			},
		}),
		xidQ: buildPromVectorResponse([]model.Sample{
			{Value: 0, Timestamp: now},
		}),
	}

	mux := http.NewServeMux()
	mux.Handle("/api/v1/query", &mockPromHandler{responses: mockResponses})
	server := httptest.NewServer(mux)
	defer server.Close()

	gen := newTestMetricsGenerator(t, server.URL, containers, devices)
	resetTestMetrics()

	err := gen.GenerateMetrics(context.Background())
	if err != nil {
		t.Fatalf("GenerateMetrics failed: %v", err)
	}

	if got := readMetricAnyLabels("hami_device_temperature", map[string]string{"deviceuuid": uuid}); got != 72 {
		t.Errorf("hami_device_temperature: want 72, got %v", got)
	}
	if got := readMetricAnyLabels("hami_device_memory_temperature", map[string]string{"deviceuuid": uuid}); got != 58 {
		t.Errorf("hami_device_memory_temperature: want 58, got %v", got)
	}
	if got := readMetricAnyLabels("hami_device_power", map[string]string{"deviceuuid": uuid}); got != 280 {
		t.Errorf("hami_device_power: want 280, got %v", got)
	}
	// 验证 driver_version 标签
	if got := readMetricAnyLabels("hami_device_power", map[string]string{
		"deviceuuid": uuid, "driver_version": "暂无",
	}); got != 280 {
		t.Errorf("hami_device_power with driver_version=暂无: want 280, got %v", got)
	}
	if got := readMetricAnyLabels("hami_device_hardware_health", map[string]string{"deviceuuid": uuid}); got != 0 {
		t.Errorf("hami_device_hardware_health: want 0, got %v", got)
	}
}

func TestPPU_GenerateContainerMetrics_CoreMemory(t *testing.T) {
	uuid := "GPU-ppu-container-test-uuid"
	devices := []*biz.DeviceInfo{
		{
			Id:       uuid,
			AliasId:  uuid,
			Count:    1,
			Devmem:   98304,
			Devcore:  100,
			Type:     "PPU",
			NodeName: "ppu-node-1",
			Provider: "PPU",
			Health:   true,
		},
	}
	containers := []*biz.Container{
		{
			Name:      "inference",
			PodName:   "ppu-pod",
			Namespace: "default",
			PodUID:    "pod-ppu-1",
			NodeName:  "ppu-node-1",
			ContainerDevices: biz.ContainerDevices{
				{UUID: uuid, Type: "PPU", Usedmem: 32768, Usedcores: 50},
			},
		},
	}

	now := model.Now()
	coreQuery := fmt.Sprintf("DCGM_FI_DEV_GPU_UTIL{UUID=\"%s\"}", uuid)
	memQuery := fmt.Sprintf("avg(DCGM_FI_DEV_FB_USED{UUID=\"%s\"})", uuid)

	mockResponses := map[string]string{
		coreQuery: buildPromVectorResponse([]model.Sample{
			{Value: 12, Timestamp: now}, // GPU_UTIL = 12%
		}),
		memQuery: buildPromVectorResponse([]model.Sample{
			{Value: 2, Timestamp: now}, // FB_USED = 2 MiB
		}),
	}

	mux := http.NewServeMux()
	mux.Handle("/api/v1/query", &mockPromHandler{responses: mockResponses})
	server := httptest.NewServer(mux)
	defer server.Close()

	gen := newTestMetricsGenerator(t, server.URL, containers, devices)
	resetTestMetrics()

	err := gen.GenerateContainerMetrics(context.Background())
	if err != nil {
		t.Fatalf("GenerateContainerMetrics failed: %v", err)
	}

	if got := readMetricAnyLabels("hami_container_vgpu_allocated", map[string]string{
		"container_name": "inference", "pod_name": "ppu-pod", "namespace_name": "default",
	}); got != 1 {
		t.Errorf("hami_container_vgpu_allocated: want 1, got %v", got)
	}
	if got := readMetricAnyLabels("hami_container_vmemory_allocated", map[string]string{
		"container_name": "inference", "pod_name": "ppu-pod", "namespace_name": "default",
	}); got != 32768 {
		t.Errorf("hami_container_vmemory_allocated: want 32768, got %v", got)
	}
	// GPU_UTIL = 12%, so core_used = 12
	if got := readMetricAnyLabels("hami_container_core_used", map[string]string{
		"pod_name": "ppu-pod", "container_name": "inference", "namespace_name": "default",
	}); got != 12 {
		t.Errorf("hami_container_core_used: want 12, got %v", got)
	}
	// roundToOneDecimal(100 * 12 / 50) = roundToOneDecimal(24) = 24
	if got := readMetricAnyLabels("hami_container_core_util", map[string]string{
		"pod_name": "ppu-pod", "container_name": "inference", "namespace_name": "default",
	}); got != 24 {
		t.Errorf("hami_container_core_util: want 24, got %v", got)
	}
	// FB_USED = 2 MiB, ×1024×1024 → bytes, /1024/1024 → 2
	if got := readMetricAnyLabels("hami_container_memory_used", map[string]string{
		"pod_name": "ppu-pod", "container_name": "inference", "namespace_name": "default",
	}); got != 2 {
		t.Errorf("hami_container_memory_used: want 2, got %v", got)
	}
	// roundToOneDecimal(100 * 2 / 32768) = roundToOneDecimal(0.006) = 0
	if got := readMetricAnyLabels("hami_container_memory_util", map[string]string{
		"pod_name": "ppu-pod", "container_name": "inference", "namespace_name": "default",
	}); got != 0 {
		t.Errorf("hami_container_memory_util: want 0, got %v", got)
	}
}

// Verify queryDeviceAdditional uses DCGM_FI_DEV_POWER_USAGE and extracts DeviceNo from device label
func TestPPU_QueryDeviceAdditional(t *testing.T) {
	uuid := "GPU-driver-test-uuid"
	now := model.Now()
	additionalQ := fmt.Sprintf("DCGM_FI_DEV_POWER_USAGE{UUID=\"%s\"}", uuid)

	mockResponses := map[string]string{
		additionalQ: buildPromVectorResponse([]model.Sample{
			{
				Metric: model.Metric{
					"UUID":      model.LabelValue(uuid),
					"device":    "ppu0",
					"modelName": "PPU-ZW610E",
					"Hostname":  "ppu01",
				},
				Value: 68.02, Timestamp: now,
			},
		}),
	}

	mux := http.NewServeMux()
	mux.Handle("/api/v1/query", &mockPromHandler{responses: mockResponses})
	server := httptest.NewServer(mux)
	defer server.Close()

	gen := newTestMetricsGenerator(t, server.URL, nil, nil)

	info, err := gen.queryDeviceAdditional(context.Background(), "PPU", uuid)
	if err != nil {
		t.Fatalf("queryDeviceAdditional failed: %v", err)
	}
	if info.DriverVersion != "暂无" {
		t.Errorf("DriverVersion: want %q, got %q", "暂无", info.DriverVersion)
	}
	if info.DeviceNo != "ppu0" {
		t.Errorf("DeviceNo: want ppu0, got %q", info.DeviceNo)
	}
}

// Verify PPU fanSpeed returns gracefully (fan not supported by PPU DCGM)
func TestPPU_FanSpeedUnsupported(t *testing.T) {
	uuid := "GPU-fan-test-uuid"
	devices := []*biz.DeviceInfo{
		{
			Id: uuid, AliasId: uuid,
			Count: 1, Devmem: 98304, Devcore: 100,
			Type: "PPU", NodeName: "ppu-node-1", Provider: "PPU", Health: true,
		},
	}
	containers := []*biz.Container{}

	now := model.Now()
	memUsedQ := fmt.Sprintf("avg(DCGM_FI_DEV_FB_USED{UUID=\"%s\"})", uuid)
	memFreeQ := fmt.Sprintf("avg(DCGM_FI_DEV_FB_FREE{UUID=\"%s\"})", uuid)
	// Fan query returns empty — no DCGM_FI_DEV_FAN_SPEED support in PPU
	fanQ := fmt.Sprintf("avg(DCGM_FI_DEV_FAN_SPEED{UUID=\"%s\"})", uuid)

	mockResponses := map[string]string{
		memUsedQ: buildPromVectorResponse([]model.Sample{
			{Metric: model.Metric{"UUID": model.LabelValue(uuid)}, Value: 10000, Timestamp: now},
		}),
		memFreeQ: buildPromVectorResponse([]model.Sample{
			{Metric: model.Metric{"UUID": model.LabelValue(uuid)}, Value: 88304, Timestamp: now},
		}),
		fanQ: buildPromVectorResponse(nil), // no data
	}

	mux := http.NewServeMux()
	mux.Handle("/api/v1/query", &mockPromHandler{responses: mockResponses})
	server := httptest.NewServer(mux)
	defer server.Close()

	gen := newTestMetricsGenerator(t, server.URL, containers, devices)
	resetTestMetrics()

	err := gen.GenerateMetrics(context.Background())
	if err != nil {
		t.Fatalf("GenerateMetrics failed: %v", err)
	}

	// Fan speed metric should NOT be set for PPU (unsupported)
	// hami_memory_used should still work
	if got := readMetricAnyLabels("hami_memory_used", map[string]string{"deviceuuid": uuid}); got != 10000 {
		t.Errorf("hami_memory_used: want 10000, got %v", got)
	}
}

// ---- Ascend proportional split Container Metrics Tests ----

func TestAscend910B_MultiContainer_ProportionalSplit(t *testing.T) {
	deviceUUID := "npu-uuid-1"
	devices := []*biz.DeviceInfo{
		{
			Id: deviceUUID, AliasId: deviceUUID,
			Count: 1, Devmem: 65536, Devcore: 100,
			Type: "Ascend910B", NodeName: "node-1", Provider: "Ascend", Health: true,
		},
	}
	// Two containers each allocated 16GB on the same card
	// Usedmem=16384 each, total=32768
	containers := []*biz.Container{
		{
			Name: "ctr-a", PodName: "pod-1", Namespace: "ns-1", PodUID: "pod-uid-1", NodeName: "node-1",
			ContainerDevices: biz.ContainerDevices{
				{UUID: deviceUUID, Type: "Ascend910B", Usedmem: 16384, Usedcores: 25},
			},
		},
		{
			Name: "ctr-b", PodName: "pod-2", Namespace: "ns-1", PodUID: "pod-uid-2", NodeName: "node-1",
			ContainerDevices: biz.ContainerDevices{
				{UUID: deviceUUID, Type: "Ascend910B", Usedmem: 16384, Usedcores: 25},
			},
		},
	}

	now := model.Now()
	// PromQL queries needed:
	// deviceCoreUtil: avg(npu_chip_info_utilization{vdie_id="<uuid>"})
	// deviceMemUsed: avg(npu_chip_info_hbm_used_memory{vdie_id="<uuid>"})
	// deviceMemTotal (for core correction): avg(npu_chip_info_hbm_total_memory{vdie_id="<uuid>"})
	coreUtilQ := fmt.Sprintf("avg(npu_chip_info_utilization{vdie_id=\"%s\"})", deviceUUID)
	memUsedQ := fmt.Sprintf("avg(npu_chip_info_hbm_used_memory{vdie_id=\"%s\"})", deviceUUID)
	memTotalQ := fmt.Sprintf("avg(npu_chip_info_hbm_total_memory{vdie_id=\"%s\"})", deviceUUID)

	// Card util=60%, card mem used=28610.2 MB (~28 GB)
	// Each container ratio=16384/32768=0.5
	// taskCoreUsed=60*0.5=30, taskMemoryUsed=28610.2*0.5≈14305 MB
	// core correction: perc=16384/65536=0.25, core=int32(25)
	mockResponses := map[string]string{
		coreUtilQ: buildPromVectorResponse([]model.Sample{
			{Value: 60, Timestamp: now},
		}),
		memUsedQ: buildPromVectorResponse([]model.Sample{
			{Value: 28610.2, Timestamp: now},
		}),
		memTotalQ: buildPromVectorResponse([]model.Sample{
			{Value: 65536, Timestamp: now},
		}),
	}

	mux := http.NewServeMux()
	mux.Handle("/api/v1/query", &mockPromHandler{responses: mockResponses})
	server := httptest.NewServer(mux)
	defer server.Close()

	gen := newTestMetricsGenerator(t, server.URL, containers, devices)
	resetTestMetrics()

	err := gen.GenerateContainerMetrics(context.Background())
	if err != nil {
		t.Fatalf("GenerateContainerMetrics failed: %v", err)
	}

	// Expected: cardUtil=60, ratio=0.5, so core_used=30 for each
	// core_util = core_used = 30 (比例拆分：util = used，不除以 core)
	wantCoreUsed := float64(30)
	wantCoreUtilA := float64(30)

	labelsA := map[string]string{"pod_name": "pod-1", "container_name": "ctr-a", "namespace_name": "ns-1"}
	labelsB := map[string]string{"pod_name": "pod-2", "container_name": "ctr-b", "namespace_name": "ns-1"}

	// core_used = 30 for both
	if got := readMetricAnyLabels("hami_container_core_used", labelsA); got != wantCoreUsed {
		t.Errorf("ctr-a hami_container_core_used: want %v, got %v", wantCoreUsed, got)
	}
	if got := readMetricAnyLabels("hami_container_core_used", labelsB); got != wantCoreUsed {
		t.Errorf("ctr-b hami_container_core_used: want %v, got %v", wantCoreUsed, got)
	}

	// core_util = 30 for both (= core_used)
	if got := readMetricAnyLabels("hami_container_core_util", labelsA); !approxEqual(wantCoreUtilA, got, 0.1) {
		t.Errorf("ctr-a hami_container_core_util: want %v, got %v", wantCoreUtilA, got)
	}
	if got := readMetricAnyLabels("hami_container_core_util", labelsB); !approxEqual(wantCoreUtilA, got, 0.1) {
		t.Errorf("ctr-b hami_container_core_util: want %v, got %v", wantCoreUtilA, got)
	}

	// memory: card mem used = 28610.2 MB
	// ratio=0.5, taskMemoryUsed = 28610.2 * 0.5 = 14305.1 MB
	// after *1024*1024/1024/1024 NOOP: 14305.1
	// memory_util = 100 * 14305.1 / 16384 = 87.3 -> roundToOneDecimal = 87.3
	wantMemUsed := roundToOneDecimal(float64(30000000000) / 1024 / 1024 * 0.5)
	wantMemUtil := roundToOneDecimal(100 * wantMemUsed / float64(16384))

	if got := readMetricAnyLabels("hami_container_memory_used", labelsA); !approxEqual(wantMemUsed, got, 0.1) {
		t.Errorf("ctr-a hami_container_memory_used: want %v, got %v", wantMemUsed, got)
	}
	if got := readMetricAnyLabels("hami_container_memory_used", labelsB); !approxEqual(wantMemUsed, got, 0.1) {
		t.Errorf("ctr-b hami_container_memory_used: want %v, got %v", wantMemUsed, got)
	}
	if got := readMetricAnyLabels("hami_container_memory_util", labelsA); !approxEqual(wantMemUtil, got, 0.1) {
		t.Errorf("ctr-a hami_container_memory_util: want ~%v, got %v", wantMemUtil, got)
	}
	if got := readMetricAnyLabels("hami_container_memory_util", labelsB); !approxEqual(wantMemUtil, got, 0.1) {
		t.Errorf("ctr-b hami_container_memory_util: want ~%v, got %v", wantMemUtil, got)
	}
}

func TestAscend910B_SingleContainer_FullCard(t *testing.T) {
	deviceUUID := "npu-uuid-1"
	devices := []*biz.DeviceInfo{
		{
			Id: deviceUUID, AliasId: deviceUUID,
			Count: 1, Devmem: 65536, Devcore: 100,
			Type: "Ascend910B", NodeName: "node-1", Provider: "Ascend", Health: true,
		},
	}
	// Single container allocated 100% of the card (33792 MB)
	containers := []*biz.Container{
		{
			Name: "ctr", PodName: "pod-1", Namespace: "ns-1", PodUID: "pod-uid-1", NodeName: "node-1",
			ContainerDevices: biz.ContainerDevices{
				{UUID: deviceUUID, Type: "Ascend910B", Usedmem: 33792, Usedcores: 100},
			},
		},
	}

	now := model.Now()
	coreUtilQ := fmt.Sprintf("avg(npu_chip_info_utilization{vdie_id=\"%s\"})", deviceUUID)
	memUsedQ := fmt.Sprintf("avg(npu_chip_info_hbm_used_memory{vdie_id=\"%s\"})", deviceUUID)
	memTotalQ := fmt.Sprintf("avg(npu_chip_info_hbm_total_memory{vdie_id=\"%s\"})", deviceUUID)

	mockResponses := map[string]string{
		coreUtilQ: buildPromVectorResponse([]model.Sample{
			{Value: 80, Timestamp: now},
		}),
		memUsedQ: buildPromVectorResponse([]model.Sample{
			{Value: 19073.5, Timestamp: now},
		}),
		memTotalQ: buildPromVectorResponse([]model.Sample{
			{Value: 65536, Timestamp: now},
		}),
	}

	mux := http.NewServeMux()
	mux.Handle("/api/v1/query", &mockPromHandler{responses: mockResponses})
	server := httptest.NewServer(mux)
	defer server.Close()

	gen := newTestMetricsGenerator(t, server.URL, containers, devices)
	resetTestMetrics()

	err := gen.GenerateContainerMetrics(context.Background())
	if err != nil {
		t.Fatalf("GenerateContainerMetrics failed: %v", err)
	}

	labels := map[string]string{"pod_name": "pod-1", "container_name": "ctr", "namespace_name": "ns-1"}

	// Single container: ratio = 33792/33792 = 1.0
	// core_used = 80 * 1.0 = 80
	// core correction: perc = 33792/65536 = 0.5156, core = int32(51)
	// util = 100 * 80 / 51 ≈ 156.9
	if got := readMetricAnyLabels("hami_container_core_used", labels); got != 80 {
		t.Errorf("core_used: want 80, got %v", got)
	}

	// memory: 20000000000/1024/1024 * 1.0 ≈ 19073.5 MB
	wantMemUsed := roundToOneDecimal(float64(20000000000) / 1024 / 1024 * 1.0)
	if got := readMetricAnyLabels("hami_container_memory_used", labels); !approxEqual(wantMemUsed, got, 0.1) {
		t.Errorf("memory_used: want %v, got %v", wantMemUsed, got)
	}
}

func TestAscend910B_NoMatchingContainer_Fallback(t *testing.T) {
	// Device has no matching containers at all — totalMemoryOnCard=0
	// Should skip proportional split and fall through to else branch
	// (else branch calls taskCoreUsed/taskMemoryUsed, which will fail because
	//  container_npu_* queries are not mocked — resulting in err != nil and no metrics set)
	deviceUUID := "npu-uuid-1"
	devices := []*biz.DeviceInfo{
		{
			Id: deviceUUID, AliasId: deviceUUID,
			Count: 1, Devmem: 65536, Devcore: 100,
			Type: "Ascend910B", NodeName: "node-1", Provider: "Ascend", Health: true,
		},
	}
	// Container with a DIFFERENT UUID — won't match
	containers := []*biz.Container{
		{
			Name: "ctr", PodName: "pod-1", Namespace: "ns-1", PodUID: "pod-uid-1", NodeName: "node-1",
			ContainerDevices: biz.ContainerDevices{
				{UUID: "other-uuid", Type: "Ascend910B", Usedmem: 16384, Usedcores: 25},
			},
		},
	}

	now := model.Now()
	coreUtilQ := fmt.Sprintf("avg(npu_chip_info_utilization{vdie_id=\"%s\"})", deviceUUID)
	memUsedQ := fmt.Sprintf("avg(npu_chip_info_hbm_used_memory{vdie_id=\"%s\"})", deviceUUID)
	memTotalQ := fmt.Sprintf("avg(npu_chip_info_hbm_total_memory{vdie_id=\"%s\"})", deviceUUID)

	// core correction in goroutine calls deviceMemTotal, so we need that mock
	mockResponses := map[string]string{
		coreUtilQ: buildPromVectorResponse([]model.Sample{
			{Value: 60, Timestamp: now},
		}),
		memUsedQ: buildPromVectorResponse([]model.Sample{
			{Value: 28610.2, Timestamp: now},
		}),
		memTotalQ: buildPromVectorResponse([]model.Sample{
			{Value: 65536, Timestamp: now},
		}),
	}

	mux := http.NewServeMux()
	mux.Handle("/api/v1/query", &mockPromHandler{responses: mockResponses})
	server := httptest.NewServer(mux)
	defer server.Close()

	gen := newTestMetricsGenerator(t, server.URL, containers, devices)
	resetTestMetrics()

	// GenerateMetrics starts with reset() then GenerateContainerMetrics
	// Use GenerateContainerMetrics directly
	err := gen.GenerateContainerMetrics(context.Background())
	if err != nil {
		t.Fatalf("GenerateContainerMetrics failed: %v", err)
	}

	labels := map[string]string{"pod_name": "pod-1", "container_name": "ctr", "namespace_name": "ns-1"}

	// Container UUID "other-uuid" does not match device UUID "npu-uuid-1"
	// So provider remains "" and goroutine returns nil without setting any metrics
	// readMetricAnyLabels returns -1 when no metric with matching labels exists
	if got := readMetricAnyLabels("hami_container_core_used", labels); got != -1 {
		t.Errorf("core_used: want -1 (no metric set for unmatched container), got %v", got)
	}
	if got := readMetricAnyLabels("hami_container_memory_used", labels); got != -1 {
		t.Errorf("memory_used: want -1 (no metric set for unmatched container), got %v", got)
	}
}

func TestAscend910B_CardQueryFails_SkipCoreMemoryMetrics(t *testing.T) {
	// Split model (910B) with card-level query failure → ascendCardQueriesOK = false
	// Core/memory metrics should be SKIPPED (not set) to avoid whole-card values
	// producing >100% utilization (ADR-0001).
	// Allocation metrics should still be set.
	deviceUUID := "npu-uuid-skip"
	devices := []*biz.DeviceInfo{
		{
			Id: deviceUUID, AliasId: deviceUUID,
			Count: 1, Devmem: 65536, Devcore: 100,
			Type: "Ascend910B", NodeName: "node-1", Provider: "Ascend", Health: true,
		},
	}
	containers := []*biz.Container{
		{
			Name: "ctr", PodName: "pod-1", Namespace: "ns-1", PodUID: "pod-uid-1", NodeName: "node-1",
			ContainerDevices: biz.ContainerDevices{
				{UUID: deviceUUID, Type: "Ascend910B", Usedmem: 16384, Usedcores: 25},
			},
		},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/query", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("query")
		if r.Method == "POST" {
			query = r.PostFormValue("query")
		}
		// Card-level queries return HTTP 500 → queryInstantVal returns error
		// → ascendCardQueriesOK = false → ascendSkipMetrics = true
		if strings.Contains(query, "npu_chip_info_utilization") || strings.Contains(query, "npu_chip_info_hbm_used_memory") {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		// Other queries (e.g. deviceMemTotal) return empty result
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[]}}`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	gen := newTestMetricsGenerator(t, server.URL, containers, devices)
	resetTestMetrics()

	err := gen.GenerateContainerMetrics(context.Background())
	if err != nil {
		t.Fatalf("GenerateContainerMetrics failed: %v", err)
	}

	labels := map[string]string{"pod_name": "pod-1", "container_name": "ctr", "namespace_name": "ns-1"}

	// Allocation metrics should be set (unaffected by card query failure)
	if got := readMetricAnyLabels("hami_container_vgpu_allocated", labels); got != 1 {
		t.Errorf("hami_container_vgpu_allocated: want 1, got %v", got)
	}
	if got := readMetricAnyLabels("hami_container_vmemory_allocated", labels); got != 16384 {
		t.Errorf("hami_container_vmemory_allocated: want 16384, got %v", got)
	}

	// Core/memory metrics should NOT be set (skipped per ADR-0001)
	// readMetricAnyLabels returns -1 when no metric with matching labels exists
	if got := readMetricAnyLabels("hami_container_core_used", labels); got != -1 {
		t.Errorf("hami_container_core_used: want -1 (skipped), got %v", got)
	}
	if got := readMetricAnyLabels("hami_container_core_util", labels); got != -1 {
		t.Errorf("hami_container_core_util: want -1 (skipped), got %v", got)
	}
	if got := readMetricAnyLabels("hami_container_memory_used", labels); got != -1 {
		t.Errorf("hami_container_memory_used: want -1 (skipped), got %v", got)
	}
	if got := readMetricAnyLabels("hami_container_memory_util", labels); got != -1 {
		t.Errorf("hami_container_memory_util: want -1 (skipped), got %v", got)
	}
}
