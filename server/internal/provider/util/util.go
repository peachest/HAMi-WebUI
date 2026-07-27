package util

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/go-kratos/kratos/v2/log"
	corev1 "k8s.io/api/core/v1"
)

const (
	// OneContainerMultiDeviceSplitSymbol this is when one container use multi device, use : symbol to join device info.
	OneContainerMultiDeviceSplitSymbol = ":"

	// OnePodMultiContainerSplitSymbol this is when one pod having multi container and more than one container use device, use ; symbol to join device info.
	OnePodMultiContainerSplitSymbol = ";"

	NvidiaGPUDevice     = "NVIDIA"
	HygonGPUDevice      = "DCU"
	CambriconGPUDevice  = "MLU"
	MetaxSGPUDevice     = "Metax-SGPU"
	AlibabaPPUDevice    = "PPU"

	// Ascend 前缀，所有 Ascend 型号（Ascend910A/910B/910B3/910B4/910B4-1/910C/310P 等）均以此为前缀。
	// 常量统一定义在 ascend 包中，此处仅保留前缀用于 DecodePodDevices 路由。
	AscendPrefix = "Ascend"

	DsmluProfileAndInstance = "CAMBRICON_DSMLU_PROFILE_INSTANCE"

	NVIDIAPriority = "nvidia.com/priority"
)

type ascendDeviceConfig struct {
	Usedmem   int32
	Usedcores int32
}

var (
	InRequestDevices    map[string]string
	SupportDevices      map[string]string
	ascendDeviceConfigs map[string]map[int32]ascendDeviceConfig
)

func init() {
	InRequestDevices = make(map[string]string)
	SupportDevices = make(map[string]string)
	ascendDeviceConfigs = map[string]map[int32]ascendDeviceConfig{
		// Ascend910A (chipName: 910B, aiCore=30, mem=32768)
		"Ascend910A": {
			2184:   {Usedmem: 2184, Usedcores: 7},   // vir02: 2/30
			4369:   {Usedmem: 4369, Usedcores: 13},  // vir04: 4/30
			8738:   {Usedmem: 8738, Usedcores: 27},  // vir08: 8/30
			17476:  {Usedmem: 17476, Usedcores: 53}, // vir16: 16/30
			32768:  {Usedmem: 32768, Usedcores: 100},
		},
		// Ascend910B2 (chipName: 910B2, aiCore=24, mem=65536)
		"Ascend910B2": {
			8192:  {Usedmem: 8192, Usedcores: 13},  // vir03_1c_8g: 3/24
			16384: {Usedmem: 16384, Usedcores: 25}, // vir06_1c_16g: 6/24
			32768: {Usedmem: 32768, Usedcores: 50}, // vir12_3c_32g: 12/24
			65536: {Usedmem: 65536, Usedcores: 100},
		},
		// Ascend910B (base-charts commonWord for 910B3, aiCore=20, mem=65536)
		"Ascend910B": {
			16384: {Usedmem: 16384, Usedcores: 25}, // vir05_1c_16g: 5/20
			32768: {Usedmem: 32768, Usedcores: 50}, // vir10_3c_32g: 10/20
			65536: {Usedmem: 65536, Usedcores: 100},
		},
		// Ascend910B3 (HAMi chart commonWord for 910B3, aiCore=20, mem=65536)
		"Ascend910B3": {
			16384: {Usedmem: 16384, Usedcores: 25}, // vir05_1c_16g: 5/20
			32768: {Usedmem: 32768, Usedcores: 50}, // vir10_3c_32g: 10/20
			65536: {Usedmem: 65536, Usedcores: 100},
		},
		// Ascend910B4 (chipName: 910B4, aiCore=20, mem=32768)
		"Ascend910B4": {
			8192:  {Usedmem: 8192, Usedcores: 25},  // vir05_1c_8g: 5/20
			16384: {Usedmem: 16384, Usedcores: 50}, // vir10_3c_16g: 10/20
			32768: {Usedmem: 32768, Usedcores: 100},
		},
		// Ascend910B4-1 (chipName: 910B4-1, aiCore=20, mem=65536)
		"Ascend910B4-1": {
			16384: {Usedmem: 16384, Usedcores: 25},  // vir05_1c_8g: 5/20
			32768: {Usedmem: 32768, Usedcores: 50},  // vir10_3c_16g: 10/20
			65536: {Usedmem: 65536, Usedcores: 100},
		},
		// Ascend910C (chipName: Ascend910, aiCore=20, mem=65536, superPod)
		"Ascend910C": {
			16384: {Usedmem: 16384, Usedcores: 25}, // vir05_1c_16g: 5/20
			32768: {Usedmem: 32768, Usedcores: 50}, // vir10_3c_32g: 10/20
			65536: {Usedmem: 65536, Usedcores: 100},
		},
		// Ascend310P (chipName: 310P3, aiCore=8, mem=24576)
		"Ascend310P": {
			3072:  {Usedmem: 3072, Usedcores: 13},  // vir01: 1/8
			6144:  {Usedmem: 6144, Usedcores: 25},  // vir02: 2/8
			12288: {Usedmem: 12288, Usedcores: 50}, // vir04: 4/8
			24576: {Usedmem: 24576, Usedcores: 100},
		},
	}
	initMLUDevice()
}

func initMLUDevice() {
}

func EncodeNodeDevices(dlist []*DeviceInfo, log *log.Helper) string {
	tmp := ""
	for _, val := range dlist {
		tmp += val.ID + "," + strconv.FormatInt(int64(val.Count), 10) + "," + strconv.Itoa(int(val.Devmem)) + "," + strconv.Itoa(int(val.Devcore)) + "," + val.Type + "," + strconv.Itoa(val.Numa) + "," + strconv.FormatBool(val.Health) + OneContainerMultiDeviceSplitSymbol
	}
	log.Infof("Encoded node Devices: %s", tmp)
	return tmp
}

// DecodeNodeDevices decodes the node devices from a string.
func DecodeNodeDevices(str string, log *log.Helper) ([]*DeviceInfo, error) {
	if !strings.Contains(str, OneContainerMultiDeviceSplitSymbol) {
		log.Warn("Node annotations not decode successfully")
		return []*DeviceInfo{}, errors.New("node annotations not decode successfully")
	}
	tmp := strings.Split(str, OneContainerMultiDeviceSplitSymbol)
	var retval []*DeviceInfo
	for _, val := range tmp {
		if strings.Contains(val, ",") {
			items := strings.Split(val, ",")
			if len(items) >= 7 || len(items) == 9 {
				count, _ := strconv.ParseInt(items[1], 10, 32)
				devmem, _ := strconv.ParseInt(items[2], 10, 32)
				devcore, _ := strconv.ParseInt(items[3], 10, 32)
				health, _ := strconv.ParseBool(items[6])
				numa, _ := strconv.Atoi(items[5])
				mode := "hami-core"
				index := 0
				if len(items) == 9 {
					index, _ = strconv.Atoi(items[7])
					mode = items[8]
				}
				i := DeviceInfo{
					ID:      items[0],
					AliasId: items[0],
					Count:   int32(count),
					Devmem:  int32(devmem),
					Devcore: int32(devcore),
					Type:    items[4],
					Numa:    numa,
					Health:  health,
					Mode:    mode,
					Index:   uint(index),
				}
				retval = append(retval, &i)
			} else {
				return []*DeviceInfo{}, errors.New("node annotations not decode successfully")
			}
		}
	}
	return retval, nil
}

// DecodeContainerDevices decodes the container devices from a string.
func DecodeContainerDevices(str, priority string) (ContainerDevices, error) {
	if len(str) == 0 {
		return ContainerDevices{}, nil
	}
	cd := strings.Split(str, OneContainerMultiDeviceSplitSymbol)
	contdev := ContainerDevices{}
	tmpdev := ContainerDevice{}
	if len(str) == 0 {
		return ContainerDevices{}, nil
	}
	for i, val := range cd {
		if strings.Contains(val, ",") {
			tmpstr := strings.Split(val, ",")
			if len(tmpstr) < 4 {
				return ContainerDevices{}, fmt.Errorf("pod annotation format error; information missing, please do not use nodeName field in task")
			}
			tmpdev.Idx = i
			tmpdev.UUID = tmpstr[0]
			tmpdev.Type = tmpstr[1]
			devmem, _ := strconv.ParseInt(tmpstr[2], 10, 32)
			tmpdev.Usedmem = int32(devmem)
			devcores, _ := strconv.ParseInt(tmpstr[3], 10, 32)
			if devcores == 0 {
				tmpdev.Usedcores = 100
			} else {
				tmpdev.Usedcores = int32(devcores)
			}
			tmpdev.Priority = priority
			contdev = append(contdev, tmpdev)
		}
	}
	return contdev, nil
}

func DecodeDCUContainerDevices(str, priority, nodeName string) (ContainerDevices, error) {
	if len(str) == 0 {
		return ContainerDevices{}, nil
	}
	cd := strings.Split(str, OneContainerMultiDeviceSplitSymbol)
	contdev := ContainerDevices{}
	tmpdev := ContainerDevice{}
	if len(str) == 0 {
		return ContainerDevices{}, nil
	}
	for i, val := range cd {
		if strings.Contains(val, ",") {
			tmpstr := strings.Split(val, ",")
			if len(tmpstr) < 4 {
				return ContainerDevices{}, fmt.Errorf("pod annotation format error; information missing, please do not use nodeName field in task")
			}
			cardIdx := strings.Split(tmpstr[0], "-")
			if len(cardIdx) < 2 {
				log.Log(log.LevelWarn, "DecodeDCUContainerDevices: length of cardIdx is less than 2, skipping device")
				continue
			}
			tmpdev.Idx = i
			tmpdev.UUID = fmt.Sprintf("%s-dcu-%s", nodeName, cardIdx[1])
			tmpdev.Type = tmpstr[1]
			devmem, _ := strconv.ParseInt(tmpstr[2], 10, 32)
			tmpdev.Usedmem = int32(devmem)
			devcores, _ := strconv.ParseInt(tmpstr[3], 10, 32)
			if devcores == 0 {
				tmpdev.Usedcores = 100
			} else {
				tmpdev.Usedcores = int32(devcores)
			}
			tmpdev.Priority = priority
			contdev = append(contdev, tmpdev)
		}
	}
	return contdev, nil
}

func DecodeNpuContainerDevices(str string) (ContainerDevices, error) {
	if len(str) == 0 {
		return ContainerDevices{}, nil
	}
	cd := strings.Split(str, OneContainerMultiDeviceSplitSymbol)
	contdev := ContainerDevices{}
	tmpdev := ContainerDevice{}
	if len(str) == 0 {
		return ContainerDevices{}, nil
	}
	for i, val := range cd {
		if strings.Contains(val, ",") {
			tmpstr := strings.Split(val, ",")
			if len(tmpstr) < 4 {
				return ContainerDevices{}, fmt.Errorf("pod annotation format error; information missing, please do not use nodeName field in task")
			}
			tmpdev.Idx = i
			tmpdev.UUID = tmpstr[0]
			tmpdev.Type = tmpstr[1]
			devmem, _ := strconv.ParseInt(tmpstr[2], 10, 32)
			tmpdev.Usedmem = int32(devmem)
			tmpdev.Usedcores = 100

			if configs, exists := ascendDeviceConfigs[tmpdev.Type]; exists {
				if config, ok := configs[tmpdev.Usedmem]; ok {
					tmpdev.Usedcores = config.Usedcores
				}
			}

			contdev = append(contdev, tmpdev)
		}
	}
	return contdev, nil
}

// DecodeMLUContainerDevices decodes the mlu container devices from a string.
func DecodeMLUContainerDevices(str, nodeName string) (ContainerDevices, error) {
	if len(str) == 0 {
		return ContainerDevices{}, nil
	}
	contdev := ContainerDevices{}
	tmpdev := ContainerDevice{}
	if strings.Contains(str, "_") {
		//fmt.Println("cd is ", val)
		tmpstr := strings.Split(str, "_")
		if len(tmpstr) < 3 {
			return ContainerDevices{}, fmt.Errorf("pod annotation format error; information missing, please do not use nodeName field in task")
		}
		tmpdev.Type = "MLU"
		devcores, _ := strconv.ParseInt(tmpstr[1], 10, 32)
		devmem, _ := strconv.ParseInt(tmpstr[2], 10, 32)
		tmpdev.Usedmem = int32(devmem) * 1024
		index, _ := strconv.ParseInt(tmpstr[5], 10, 32)
		tmpdev.Idx = int(index)
		tmpdev.UUID = fmt.Sprintf("%s-cambricon-mlu-%d", nodeName, index)
		if devcores == 0 {
			tmpdev.Usedcores = 100
		} else {
			tmpdev.Usedcores = int32(devcores)
		}
		contdev = append(contdev, tmpdev)
	}
	return contdev, nil
}

func GetContainerPriorities(pod *corev1.Pod) []string {
	var priorities []string

	nvidiaPriority := corev1.ResourceName(NVIDIAPriority)
	for _, ctr := range pod.Spec.Containers {
		priority := ""
		if limitPriority, ok := ctr.Resources.Limits[nvidiaPriority]; ok {
			priority = limitPriority.String()
		} else if requestPriority, ok := ctr.Resources.Requests[nvidiaPriority]; ok {
			priority = requestPriority.String()
		}
		priorities = append(priorities, priority)
	}

	return priorities
}

func DecodePodDevices(pod *corev1.Pod, log *log.Helper) (PodDevices, error) {
	checklist := SupportDevices

	priorities := GetContainerPriorities(pod)

	annos := pod.Annotations
	if len(annos) == 0 {
		return PodDevices{}, nil
	}
	nodeName := annos[AssignedNodeAnnotations]
	pd := make(PodDevices)
	for devType, devs := range checklist {
		str, ok := annos[devs]
		if !ok {
			continue
		}
		pd[devType] = make(PodSingleDevice, 0)
		if strings.HasPrefix(devType, AscendPrefix) {
			for _, s := range strings.Split(str, OnePodMultiContainerSplitSymbol) {
				cd, err := DecodeNpuContainerDevices(s)
				if err != nil {
					return PodDevices{}, nil
				}
				if len(cd) == 0 {
					continue
				}
				pd[devType] = append(pd[devType], cd)
			}
			continue
		}
		switch devType {
		case CambriconGPUDevice:
			instance := annos[DsmluProfileAndInstance]
			cd, err := DecodeMLUContainerDevices(fmt.Sprintf("%s_%s_%s", str, instance, nodeName), nodeName)
			if err != nil {
				return PodDevices{}, nil
			}
			if len(cd) == 0 {
				continue
			}
			pd[devType] = append(pd[devType], cd)
		case NvidiaGPUDevice, MetaxSGPUDevice, AlibabaPPUDevice:
			for i, s := range strings.Split(str, OnePodMultiContainerSplitSymbol) {
				if s == "" {
					continue
				}
				cd, err := DecodeContainerDevices(s, priorities[i])
				if err != nil {
					return PodDevices{}, nil
				}
				if len(cd) == 0 {
					continue
				}
				pd[devType] = append(pd[devType], cd)
			}
		case HygonGPUDevice:
			for i, s := range strings.Split(str, OnePodMultiContainerSplitSymbol) {
				if s == "" {
					continue
				}
				cd, err := DecodeDCUContainerDevices(s, priorities[i], nodeName)
				if err != nil {
					return PodDevices{}, nil
				}
				if len(cd) == 0 {
					continue
				}
				pd[devType] = append(pd[devType], cd)
			}
		}
	}
	log.Infof("Decoded pod annos: poddevices %v", pd)
	return pd, nil
}

func UnMarshalNodeDevices(str string) ([]*DeviceInfo, error) {
	var dlist []*DeviceInfo
	err := json.Unmarshal([]byte(str), &dlist)
	return dlist, err
}
