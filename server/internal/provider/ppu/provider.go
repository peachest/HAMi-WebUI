package ppu

import (
	"vgpu/internal/biz"
	"vgpu/internal/provider/util"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
)

type PPU struct {
	labelSelector string
}

func NewPPU(labelSelector string) *PPU {
	return &PPU{
		labelSelector: labelSelector,
	}
}

func (p *PPU) GetNodeDevicePluginLabels() (labels.Selector, error) {
	return labels.Parse(p.labelSelector)
}

func (p *PPU) GetProvider() string {
	return biz.AlibabaPPUDevice
}

func (p *PPU) FetchDevices(node *corev1.Node) ([]*util.DeviceInfo, error) {
	deviceEncode, ok := node.Annotations[RegisterAnnos]
	if !ok {
		return []*util.DeviceInfo{}, nil
	}

	// Try JSON format first
	devices, err := util.UnMarshalNodeDevices(deviceEncode)
	if err != nil {
		// Fallback: old comma-separated format
		return util.DecodeNodeDevices(deviceEncode, nil)
	}
	return devices, nil
}

// Assert that PPU implements provider.Provider at compile time
var _ interface {
	GetNodeDevicePluginLabels() (labels.Selector, error)
	GetProvider() string
	FetchDevices(node *corev1.Node) ([]*util.DeviceInfo, error)
} = (*PPU)(nil)
