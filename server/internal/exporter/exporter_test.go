package exporter

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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
		fmt.Sprintf("avg(container_npu_utilization{exported_namespace=\"default\", pod_name=\"ascend-pod\", container_name=\"inference\"})"):
			buildPromVectorResponse([]model.Sample{
				{Value: 60, Timestamp: now},
			}),
		// deviceMemTotal for core_util calculation
		"avg(npu_chip_info_hbm_total_memory{vdie_id=\"" + chipUUID36Card0Chip0 + "\"})":
			buildPromVectorResponse([]model.Sample{
				{Value: 65536, Timestamp: now},
			}),
		"avg(npu_chip_info_hbm_total_memory{vdie_id=\"" + chipUUID36Card0Chip1 + "\"})":
			buildPromVectorResponse([]model.Sample{
				{Value: 65536, Timestamp: now},
			}),
		// taskMemoryUsed query
		fmt.Sprintf("avg(container_npu_used_memory{exported_namespace=\"default\", pod_name=\"ascend-pod\", container_name=\"inference\"})"):
			buildPromVectorResponse([]model.Sample{
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