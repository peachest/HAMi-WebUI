package ascend

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"vgpu/internal/data/prom"
	"vgpu/internal/provider/util"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/prometheus/common/model"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
)

type Ascend struct {
	prom *prom.Client
	log  *log.Helper

	nodeSelectors string
}

func NewAscend(prom *prom.Client, log *log.Helper, nodeSelectors string) *Ascend {
	return &Ascend{
		prom:          prom,
		log:           log,
		nodeSelectors: nodeSelectors,
	}
}

func (a *Ascend) GetNodeDevicePluginLabels() (labels.Selector, error) {
	return labels.Parse(a.nodeSelectors)
}

func (a *Ascend) GetProvider() string {
	return AscendDevice
}

type DeviceMeta struct {
	UUID   string
	Type   string
	Driver string
}

// mergeAscend910CDevices 将 Ascend910C 双芯片合并为单卡。
// 按 Index 顺序，2N 与 2N+1 配对，Aggregation: Devmem/Devcore 相加，Health 取与。
func (a *Ascend) mergeAscend910CDevices(devices []*util.DeviceInfo) []*util.DeviceInfo {
	var non910C []*util.DeviceInfo
	var pairs [][2]*util.DeviceInfo
	var cur []*util.DeviceInfo

	for _, d := range devices {
		if strings.HasPrefix(d.Type, "Ascend910C") {
			cur = append(cur, d)
		} else {
			non910C = append(non910C, d)
		}
	}

	// 创建用于查找的 map
	mergedIdx := make(map[*util.DeviceInfo]bool)

	// 按 Index 排序配对
	for i := 0; i < len(cur)-1; i += 2 {
		chip0 := cur[i]
		chip1 := cur[i+1]
		pairs = append(pairs, [2]*util.DeviceInfo{chip0, chip1})
		mergedIdx[chip0] = true
		mergedIdx[chip1] = true
	}

	// 处理奇数个 chip
	if len(cur)%2 != 0 {
		a.log.Warnf("Ascend910C has odd number of chips (%d), last chip (index=%d) will be kept unmerged", len(cur), cur[len(cur)-1].Index)
		non910C = append(non910C, cur[len(cur)-1])
	}

	for _, pair := range pairs {
		chip0, chip1 := pair[0], pair[1]
		merged := &util.DeviceInfo{
			ID:      chip0.ID + Ascend910CDeviceMergeSeparator + chip1.ID,
			AliasId: chip0.ID + Ascend910CDeviceMergeSeparator + chip1.ID,
			Index:   chip0.Index / 2, // 重新编号
			Count:   chip0.Count,
			Devmem:  chip0.Devmem + chip1.Devmem,
			Devcore: chip0.Devcore + chip1.Devcore,
			Type:    chip0.Type,
			Numa:    chip0.Numa,
			Mode:    chip0.Mode,
			Health:  chip0.Health && chip1.Health,
			Driver:  chip0.Driver,
		}
		non910C = append(non910C, merged)
	}

	return non910C
}

func (a *Ascend) GetDevicesFromPrometheus(node *corev1.Node) map[string]*util.DeviceInfo {
	device := make(map[string]*util.DeviceInfo)
	queryString := fmt.Sprintf("npu_chip_info_health_status{node=\"%s\"}", node.Name)
	vs, err := a.prom.Query(context.Background(), queryString)
	if err != nil {
		a.log.Warnf("query %s failed", queryString)
	} else {
		ds, ok := vs.(model.Vector)
		if !ok {
			a.log.Warnf("vectorValue: %v, failed", vs)
		} else {
			for _, d := range ds {
				id := d.Metric["id"]
				health := false
				if d.Value.Equal(1) {
					health = true
				}
				device[string(id)] = &util.DeviceInfo{
					ID:     string(d.Metric["vdie_id"]),
					Type:   string(d.Metric["model_name"]),
					Driver: "-",
					Health: health,
				}
			}
		}
	}
	return device
}

func (a *Ascend) FetchDevices(node *corev1.Node) (ret []*util.DeviceInfo, err error) {
	tmpDevice := a.GetDevicesFromPrometheus(node)
	for k, anno := range node.Annotations {
		if !strings.HasPrefix(k, AscendNodeRegisterAnnoPrefix) {
			continue
		}

		nodeDevices, err := util.UnMarshalNodeDevices(anno)
		if err != nil {
			// Fallback: 旧版逗号分隔格式（非 JSON）
			nodeDevices, err = util.DecodeNodeDevices(anno, a.log)
			if err != nil {
				return []*util.DeviceInfo{}, fmt.Errorf("failed to parse node annotation %s: %w", k, err)
			}
			// 旧格式可能不含 Index，按顺序赋值
			for j := range nodeDevices {
				if nodeDevices[j].Index == 0 {
					nodeDevices[j].Index = uint(j)
				}
			}
		}
		for i, nodedevice := range nodeDevices {
			nodeDevices[i].AliasId = nodedevice.ID
			if device, exists := tmpDevice[strconv.Itoa(i)]; exists {
				nodeDevices[i].ID = device.ID
			} else {
				log.Infof("Key %d not found in tmpDevice", i)
			}
		}
		ret = append(ret, nodeDevices...)
	}
	ret = a.mergeAscend910CDevices(ret)
	return
}
