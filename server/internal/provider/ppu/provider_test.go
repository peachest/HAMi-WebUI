package ppu

import (
	"testing"

	"vgpu/internal/provider/util"

	corev1 "k8s.io/api/core/v1"
)

func TestGetProvider(t *testing.T) {
	p := &PPU{}
	want := "PPU"
	if got := p.GetProvider(); got != want {
		t.Errorf("GetProvider() = %q, want %q", got, want)
	}
}

func TestGetNodeDevicePluginLabels_Valid(t *testing.T) {
	p := &PPU{labelSelector: "ppu=on"}
	sel, err := p.GetNodeDevicePluginLabels()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sel.String() != "ppu=on" {
		t.Errorf("selector = %q, want %q", sel.String(), "ppu=on")
	}
}

func TestGetNodeDevicePluginLabels_Empty(t *testing.T) {
	// labels.Parse("") returns a Nothing selector, not an error
	p := &PPU{labelSelector: ""}
	sel, err := p.GetNodeDevicePluginLabels()
	if err != nil {
		t.Fatalf("expected no error for empty selector, got: %v", err)
	}
	if sel == nil {
		t.Error("selector is nil")
	}
}

func TestFetchDevices_NoAnnotation(t *testing.T) {
	p := &PPU{}
	node := &corev1.Node{}
	node.Name = "test-node"
	node.Annotations = map[string]string{}

	devices, err := p.FetchDevices(node)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(devices) != 0 {
		t.Errorf("expected 0 devices, got %d", len(devices))
	}
}

func TestFetchDevices_WithCSVAnnotation(t *testing.T) {
	p := &PPU{}
	node := &corev1.Node{}
	node.Name = "test-node"
	// Comma-separated format: UUID,Count,Devmem,Devcore,Type,Numa,Health
	node.Annotations = map[string]string{
		RegisterAnnos: "GPU-962d9630-a4ef-dc16-a50d-b2effb90239d,1,98304,100,PPU,0,true:GPU-962d9630-a4ef-dc16-a50d-b2effb90239e,1,98304,100,PPU,0,true:",
	}

	devices, err := p.FetchDevices(node)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(devices) != 2 {
		t.Fatalf("expected 2 devices, got %d", len(devices))
	}

	d0 := devices[0]
	if d0.ID != "GPU-962d9630-a4ef-dc16-a50d-b2effb90239d" {
		t.Errorf("device[0].ID = %q, want %q", d0.ID, "GPU-962d9630-a4ef-dc16-a50d-b2effb90239d")
	}
	if d0.Type != "PPU" {
		t.Errorf("device[0].Type = %q, want %q", d0.Type, "PPU")
	}
	if d0.Devmem != 98304 {
		t.Errorf("device[0].Devmem = %d, want 98304", d0.Devmem)
	}
	if d0.Devcore != 100 {
		t.Errorf("device[0].Devcore = %d, want 100", d0.Devcore)
	}
	if !d0.Health {
		t.Error("device[0].Health = false, want true")
	}

	d1 := devices[1]
	if d1.ID != "GPU-962d9630-a4ef-dc16-a50d-b2effb90239e" {
		t.Errorf("device[1].ID = %q, want %q", d1.ID, "GPU-962d9630-a4ef-dc16-a50d-b2effb90239e")
	}
	if d1.Type != "PPU" {
		t.Errorf("device[1].Type = %q, want %q", d1.Type, "PPU")
	}
}

func TestFetchDevices_WithJSONAnnotation(t *testing.T) {
	p := &PPU{}
	node := &corev1.Node{}
	node.Name = "test-node"
	node.Annotations = map[string]string{
		RegisterAnnos: `[{"id":"GPU-11111111-aaaa-bbbb-cccc-dddddddddddd","index":0,"count":1,"devmem":98304,"devcore":100,"type":"PPU","numa":0,"health":true}]`,
	}

	devices, err := p.FetchDevices(node)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(devices) != 1 {
		t.Fatalf("expected 1 device, got %d", len(devices))
	}

	d0 := devices[0]
	if d0.ID != "GPU-11111111-aaaa-bbbb-cccc-dddddddddddd" {
		t.Errorf("device[0].ID = %q, want %q", d0.ID, "GPU-11111111-aaaa-bbbb-cccc-dddddddddddd")
	}
	if d0.Devmem != 98304 {
		t.Errorf("device[0].Devmem = %d, want 98304", d0.Devmem)
	}
	if d0.Devcore != 100 {
		t.Errorf("device[0].Devcore = %d, want 100", d0.Devcore)
	}
	if !d0.Health {
		t.Error("device[0].Health = false, want true")
	}
}

func TestInitRegisters(t *testing.T) {
	if got, ok := util.InRequestDevices["PPU"]; !ok {
		t.Error("util.InRequestDevices[\"PPU\"] not registered")
	} else if got != PPURequestAnno {
		t.Errorf("util.InRequestDevices[\"PPU\"] = %q, want %q", got, PPURequestAnno)
	}

	if got, ok := util.SupportDevices["PPU"]; !ok {
		t.Error("util.SupportDevices[\"PPU\"] not registered")
	} else if got != PPUAllocatedAnno {
		t.Errorf("util.SupportDevices[\"PPU\"] = %q, want %q", got, PPUAllocatedAnno)
	}
}
