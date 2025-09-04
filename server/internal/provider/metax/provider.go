package metax

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"vgpu/internal/data/prom"
	"vgpu/internal/provider/util"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/prometheus/common/model"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
)

type Metax struct {
	prom *prom.Client
	log  *log.Helper

	nodeSelectors string
}

func NewMetax(prom *prom.Client, log *log.Helper, nodeSelectors string) *Metax {
	return &Metax{
		prom:          prom,
		log:           log,
		nodeSelectors: nodeSelectors,
	}
}

func (h *Metax) GetNodeDevicePluginLabels() (labels.Selector, error) {
	return labels.Parse(h.nodeSelectors)
}

func (h *Metax) GetProvider() string {
	return MetaxSGPUDevice
}

type DeviceMeta struct {
	UUID   string
	Type   string
	Driver string
}

func DecodeNodeDevices(encoded string, log *log.Helper) ([]*util.DeviceInfo, error) {
	devs := make([]*DeviceInfo, 0)
	err := json.Unmarshal([]byte(encoded), &devs)
	if err != nil {
		log.Errorw("failed to unmarshal metax node devices", err, "encoded", encoded)
		return nil, err
	}
	ret := make([]*util.DeviceInfo, 0, len(devs))
	for _, d := range devs {
		ret = append(ret, &util.DeviceInfo{
			ID:      d.Uuid,
			AliasId: d.Uuid,
			Count:   int32(d.TotalDevCount),
			Devmem:  int32(d.TotalVRam),
			Devcore: int32(d.TotalCompute),
			Type:    d.Model,
			Numa:    d.Numa,
			Mode:    "sgpu",
			Health:  true,
		})
	}
	return ret, nil
}

func (h *Metax) GetDevicesFromPrometheus(node *corev1.Node) map[string]*util.DeviceInfo {
	deviceMap := make(map[string]*util.DeviceInfo)
	queryString := fmt.Sprintf("mx_board_core_temp{Hostname=\"%s\"}", node.Name)

	vs, err := h.prom.Query(context.Background(), queryString)
	if err != nil {
		h.log.Warnf("Failed to query %s: %v", queryString, err)
		return deviceMap
	}

	vector, ok := vs.(model.Vector)
	if !ok {
		h.log.Warnf("Unexpected result type: %v", vs)
		return deviceMap
	}

	for _, sample := range vector {
		minorNumber := string(sample.Metric["deviceId"])
		index, _ := strconv.Atoi(minorNumber)
		id := string(sample.Metric["uuid"])
		driver := string(sample.Metric["driver_version"])
		deviceMap[id] = &util.DeviceInfo{
			ID:     id,
			Index:  uint(index),
			Driver: driver,
		}
	}

	return deviceMap
}

func (h *Metax) FetchDevices(node *corev1.Node) ([]*util.DeviceInfo, error) {
	devEncoded, ok := node.Annotations[RegisterAnnos]
	if !ok {
		return []*util.DeviceInfo{}, errors.New("annos not found " + RegisterAnnos)
	}
	nodedevices, err := DecodeNodeDevices(devEncoded, h.log)
	if err != nil {
		h.log.Errorw("failed to decode metax node devices", err, "node", node.Name, "device annotation", devEncoded)
		return []*util.DeviceInfo{}, err
	}
	if len(nodedevices) == 0 {
		h.log.Infow("event", "no metax gpu device found", "node", node.Name, "device annotation", devEncoded)
		return []*util.DeviceInfo{}, errors.New("no gpu found on node")
	}
	devDecoded := util.EncodeNodeDevices(nodedevices, h.log)
	h.log.Infow("event", "metax nodes device information", "node", node.Name, "nodedevices", devDecoded)
	devDetail := h.GetDevicesFromPrometheus(node)
	for _, nodedevice := range nodedevices {
		id := nodedevice.ID
		if device, exists := devDetail[id]; exists {
			nodedevice.Index = device.Index
			nodedevice.Driver = device.Driver
		} else {
			h.log.Warnf("Device ID %s not found in devDetail", id)
			continue
		}
	}
	return nodedevices, nil
}
