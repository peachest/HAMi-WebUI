package exporter

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"

	"golang.org/x/sync/errgroup"

	pb "vgpu/api/v1"
	"vgpu/internal/biz"
	"vgpu/internal/conf"
	"vgpu/internal/data/prom"
	"vgpu/internal/provider/mlu"
	"vgpu/internal/service"

	"github.com/google/wire"
)

// ProviderSet is service providers.
var ProviderSet = wire.NewSet(
	NewMetricsGenerator,
)

type MetricsGenerator struct {
	promClient       *prom.Client
	nodeUsecase      *biz.NodeUsecase
	podUsecase       *biz.PodUseCase
	monitorService   *service.MonitorService
	concurrencyLimit int
}

// roundToTwoDecimal 将浮点数保留两位小数
func roundToTwoDecimal(value float64) float64 {
	return float64(math.Round(value*100) / 100)
}

// roundToOneDecimal 将浮点数保留一位小数并进行四舍五入
func roundToOneDecimal(value float64) float64 {
	return math.Round(value*10) / 10
}

func NewMetricsGenerator(
	c *conf.Bootstrap,
	promClient *prom.Client,
	nodeUsecase *biz.NodeUsecase,
	podUsecase *biz.PodUseCase,
	monitorService *service.MonitorService,
) *MetricsGenerator {
	concurrency := int(c.ExporterConcurrencyLimit)
	if concurrency <= 0 {
		concurrency = 16
	}
	return &MetricsGenerator{
		promClient:       promClient,
		nodeUsecase:      nodeUsecase,
		podUsecase:       podUsecase,
		monitorService:   monitorService,
		concurrencyLimit: concurrency,
	}
}

func (s *MetricsGenerator) GenerateMetrics(ctx context.Context) error {
	reset()                         // 重置所有指标缓存值
	s.GenerateDeviceMetrics(ctx)    // 卡维度指标
	s.GenerateContainerMetrics(ctx) // 任务维度指标
	return nil
}

// 卡维度指标
func (s *MetricsGenerator) GenerateDeviceMetrics(ctx context.Context) error {
	deviceInfos, err := s.nodeUsecase.ListAllDevices(ctx)
	if err != nil {
		return err
	}
	g, _ := errgroup.WithContext(ctx)
	g.SetLimit(s.concurrencyLimit)
	for _, d := range deviceInfos {
		device := d
		g.Go(func() error {
			provider := device.Provider
			deviceAdditional, err := s.queryDeviceAdditional(ctx, provider, device.Id)
			driver, deviceNo := "", ""
			if err == nil && deviceAdditional != nil {
				driver = deviceAdditional.DriverVersion
				deviceNo = deviceAdditional.DeviceNo
			}
			HamiVgpuCount.WithLabelValues(device.NodeName, provider, device.Type, device.Id, driver, deviceNo).Set(float64(device.Count))
			HamiVmemorySize.WithLabelValues(device.NodeName, provider, device.Type, device.Id, driver, deviceNo).Set(float64(device.Devmem))
			HamiVcoreSize.WithLabelValues(device.NodeName, provider, device.Type, device.Id, driver, deviceNo).Set(100)
			HamiVCoreScaling.WithLabelValues(device.NodeName, provider, device.Type, device.Id, driver, deviceNo).Set(float64(device.Devcore) / 100)
			HamiCoreSize.WithLabelValues(device.NodeName, provider, device.Type, device.Id, driver, deviceNo).Set(float64(device.Devcore))
			if v, err := s.deviceMemUsed(ctx, provider, device.Id); err == nil {
				HamiMemoryUsed.WithLabelValues(device.NodeName, provider, device.Type, device.Id, driver, deviceNo).Set(float64(v))
				HamiMemoryUtil.WithLabelValues(device.NodeName, provider, device.Type, device.Id, driver, deviceNo).Set(roundToOneDecimal(100 * float64(v/float32(device.Devmem))))
			} else {
				HamiMemorySize.WithLabelValues(device.NodeName, provider, device.Type, device.Id, driver, deviceNo).Set(float64(device.Devmem))
			}
			if total, err := s.deviceMemTotal(ctx, provider, device.Id); err == nil && total > 0 {
				HamiVMemoryScaling.WithLabelValues(device.NodeName, provider, device.Type, device.Id, driver, deviceNo).Set(roundToOneDecimal(float64(float32(device.Devmem) / total)))
			}
			if util, err := s.deviceCoreUtil(ctx, provider, device.Id); err == nil {
				HamiCoreUsed.WithLabelValues(device.NodeName, provider, device.Type, device.Id, driver, deviceNo).Set(float64(util))
				HamiCoreUtil.WithLabelValues(device.NodeName, provider, device.Type, device.Id, driver, deviceNo).Set(float64(util))
			}
			if utilAvg, err := s.deviceCoreUtil(ctx, provider, device.Id); err == nil {
				HamiCoreUsedAvg.WithLabelValues(device.NodeName, provider, device.Type, device.Id, driver, deviceNo).Set(float64(utilAvg))
				HamiCoreUtilAvg.WithLabelValues(device.NodeName, provider, device.Type, device.Id, driver, deviceNo).Set(float64(utilAvg))
			}
			if t, err := s.gpuTemperature(ctx, provider, device.Id); err == nil {
				HamiDeviceTemperature.WithLabelValues(device.NodeName, provider, device.Type, device.Id, driver, deviceNo).Set(float64(t))
			}
			if mt, err := s.memoryTemperature(ctx, provider, device.Id); err == nil {
				HamiDeviceMemoryTemperature.WithLabelValues(device.NodeName, provider, device.Type, device.Id, driver, deviceNo).Set(float64(mt))
			}
			if p, err := s.gpuPower(ctx, provider, device.Id); err == nil {
				HamiDevicePower.WithLabelValues(device.NodeName, provider, device.Type, device.Id, driver, deviceNo).Set(float64(p))
			}
			if fs, err := s.fanSpeed(ctx, provider, device.Id); err == nil {
				switch provider {
				case biz.NvidiaGPUDevice:
					HamiDeviceFanSpeedP.WithLabelValues(device.NodeName, provider, device.Type, device.Id, driver, deviceNo).Set(float64(fs))
				case biz.CambriconGPUDevice:
					HamiDeviceFanSpeedR.WithLabelValues(device.NodeName, provider, device.Type, device.Id, driver, deviceNo).Set(float64(fs))
				}
			}
			if h, err := s.gpuHardwareHealth(ctx, provider, device.Id); err == nil {
				HamiDeviceHardwareHealth.WithLabelValues(device.NodeName, provider, device.Type, device.Id, driver, deviceNo).Set(float64(h))
			}
			return nil
		})
	}
	_ = g.Wait()
	return nil
}

// 任务维度指标
func (s *MetricsGenerator) GenerateContainerMetrics(ctx context.Context) error {
	deviceInfos, err := s.nodeUsecase.ListAllDevices(ctx)
	if err != nil {
		return err
	}
	containers, err := s.podUsecase.ListAll(ctx)
	if err != nil {
		return err
	}
	g, _ := errgroup.WithContext(ctx)
	g.SetLimit(s.concurrencyLimit)
	for _, d := range deviceInfos {
		device := d
		for _, cont := range containers {
			c := cont
			g.Go(func() error {
				var vGPU int32 = 0
				var core int32 = 0
				var memory int32 = 0
				var provider = ""
				for _, cd := range c.ContainerDevices {
					if device.AliasId != "" && !strings.HasPrefix(cd.UUID, device.AliasId) {
						continue
					}
					vGPU = vGPU + 1
					core = core + cd.Usedcores
					memory = memory + cd.Usedmem
					provider = cd.Type
					if strings.HasPrefix(provider, biz.AscendGPUDevice) {
						provider = biz.AscendGPUDevice
					}
				}
				if provider == "" {
					return nil
				}
				HamiContainerVgpuAllocated.WithLabelValues(device.NodeName, provider, device.Type, device.Id, c.PodName, c.Name, c.Namespace, fmt.Sprintf("%s:%s", c.Name, c.PodUID)).Set(float64(vGPU))
				HamiContainerVmemoryAllocated.WithLabelValues(device.NodeName, provider, device.Type, device.Id, c.PodName, c.Name, c.Namespace, fmt.Sprintf("%s:%s", c.Name, c.PodUID)).Set(float64(memory))
				// fix metric value of Ascend GPU device core allocation, refer to AIP-108392
				if provider == biz.AscendGPUDevice {
					if deviceMemSize, err := s.deviceMemTotal(ctx, provider, device.Id); err == nil && deviceMemSize > 0 {
						perc := float32(memory) / deviceMemSize
						core = int32(float32(100) * perc)
					}
				}
				HamiContainerVcoreAllocated.WithLabelValues(device.NodeName, provider, device.Type, device.Id, c.PodName, c.Name, c.Namespace, fmt.Sprintf("%s:%s", c.Name, c.PodUID)).Set(float64(core))

				// 查询任务在当前设备下的算力利用率
				taskCoreUsed, err := s.taskCoreUsed(ctx, provider, c.Namespace, c.PodName, c.Name, c.PodUID, device.Id)
				fmt.Printf("GenerateContainerMetrics taskCoreUsed=%.2f, core=%d", taskCoreUsed, core)
				if err == nil {
					used := float64(0)
					util := float64(0)
					switch provider {
					case biz.NvidiaGPUDevice:
						used = float64(taskCoreUsed)
						util = roundToOneDecimal(100 * float64(taskCoreUsed) / float64(core))
					case biz.CambriconGPUDevice:
						used = float64(taskCoreUsed) / 100 * float64(core)
						util = float64(taskCoreUsed)
					case biz.HygonGPUDevice, biz.AscendGPUDevice:
						used = float64(taskCoreUsed)
						util = roundToOneDecimal(100 * float64(taskCoreUsed) / float64(core))
					case biz.MetaxGPUDevice:
						used = float64(taskCoreUsed)
						util = roundToOneDecimal(100 * float64(taskCoreUsed) / float64(core))
					default:
					}
					if cardCoreUtil, err := s.deviceCoreUtil(ctx, provider, device.Id); err == nil && used != 0 && cardCoreUtil > 95 {
						used = float64(cardCoreUtil) / 100 * float64(core)
						util = float64(cardCoreUtil)
					}
					HamiContainerCoreUsed.WithLabelValues(device.NodeName, provider, device.Type, device.Id, c.PodName, c.Name, c.Namespace).Set(used)
					HamiContainerCoreUtil.WithLabelValues(device.NodeName, provider, device.Type, device.Id, c.PodName, c.Name, c.Namespace).Set(util)
				}
				if taskMemoryUsed, err := s.taskMemoryUsed(ctx, provider, c.Namespace, c.PodName, c.Name, c.PodUID, device.Id); err == nil {
					switch provider {
					case biz.CambriconGPUDevice:
						taskMemoryUsed = float32((taskMemoryUsed/100)*float32(memory)) * 1024 * 1024
					case biz.AscendGPUDevice:
						taskMemoryUsed = float32(taskMemoryUsed) * 1024 * 1024
					case biz.MetaxGPUDevice:
						taskMemoryUsed = float32(taskMemoryUsed) * 1024
					default:
					}
					HamiContainerMemoryUsed.WithLabelValues(device.NodeName, provider, device.Type, device.Id, c.PodName, c.Name, c.Namespace).Set(float64(taskMemoryUsed / 1024 / 1024))
					HamiContainerMemoryUtil.WithLabelValues(device.NodeName, provider, device.Type, device.Id, c.PodName, c.Name, c.Namespace).Set(roundToOneDecimal(100 * float64(taskMemoryUsed/1024/1024) / float64(memory)))
				}
				return nil
			})
		}
	}
	_ = g.Wait()
	return nil
}

func (s *MetricsGenerator) queryInstantVal(ctx context.Context, query string) (float32, error) {
	res, err := s.monitorService.QueryInstant(ctx, &pb.QueryInstantRequest{
		Query: query,
	})
	if err != nil {
		return 0, err
	}
	if len(res.Data) > 0 {
		return res.Data[0].Value, nil
	}
	return 0, nil
}

// 卡显存已使用量
func (s *MetricsGenerator) deviceMemUsed(ctx context.Context, provider, deviceUUID string) (float32, error) {
	query := ""
	switch provider {
	case biz.NvidiaGPUDevice:
		query = fmt.Sprintf("avg(DCGM_FI_DEV_FB_USED{UUID=\"%s\"})", deviceUUID)
	case biz.CambriconGPUDevice:
		query = fmt.Sprintf("avg(mlu_memory_used{uuid=\"%s\"})", deviceUUID)
	case biz.AscendGPUDevice:
		query = fmt.Sprintf("avg(npu_chip_info_hbm_used_memory{vdie_id=\"%s\"})", deviceUUID)
	case biz.HygonGPUDevice:
		query = fmt.Sprintf("avg(dcu_usedmemory_bytes{device_id=\"%s\"})", deviceUUID)
	default:
		return 0, errors.New("provider not exists")
	}
	val, err := s.queryInstantVal(ctx, query)
	if err != nil {
		return val, err
	}
	if provider == biz.CambriconGPUDevice {
		val = val / mlu.CambriconMemUnit
	} else if provider == biz.HygonGPUDevice {
		val = val / 1024 / 1024
	}
	return val, err
}

// 卡显存总量
func (s *MetricsGenerator) deviceMemTotal(ctx context.Context, provider, deviceUUID string) (float32, error) {
	query := ""
	switch provider {
	case biz.NvidiaGPUDevice:
		query = fmt.Sprintf("avg(DCGM_FI_DEV_FB_FREE{UUID=\"%s\"})+avg(DCGM_FI_DEV_FB_USED{UUID=\"%s\"})", deviceUUID, deviceUUID)
	case biz.CambriconGPUDevice:
		query = fmt.Sprintf("avg(mlu_memory_total{uuid=\"%s\"})", deviceUUID)
	case biz.AscendGPUDevice:
		query = fmt.Sprintf("avg(npu_chip_info_hbm_total_memory{vdie_id=\"%s\"})", deviceUUID)
	case biz.HygonGPUDevice:
		query = fmt.Sprintf("avg(dcu_memorycap_bytes{device_id=\"%s\"})", deviceUUID)
	default:
		return 0, errors.New("provider not exists")
	}
	val, err := s.queryInstantVal(ctx, query)
	if err != nil {
		return val, err
	}
	if provider == biz.CambriconGPUDevice {
		val = val / 1024
	} else if provider == biz.HygonGPUDevice {
		val = val / 1024 / 1024
	}
	return val, err
}

// 卡算力利用率
func (s *MetricsGenerator) deviceCoreUtil(ctx context.Context, provider, deviceUUID string) (float32, error) {
	query := ""
	switch provider {
	case biz.NvidiaGPUDevice:
		// query = fmt.Sprintf("avg(avg_over_time(DCGM_FI_DEV_GPU_UTIL{UUID=\"%s\"}[1m]))", deviceUUID)
		query = fmt.Sprintf("DCGM_FI_DEV_GPU_UTIL{UUID=\"%s\"}", deviceUUID)
		// query = fmt.Sprintf("(%s * (sum_over_time(%s[5m:]) / count_over_time(( %s !=0)[5m:])) / %s) > 0 or %s", queryTemplate, queryTemplate, queryTemplate, queryTemplate, queryTemplate)
	case biz.CambriconGPUDevice:
		query = fmt.Sprintf("avg(mlu_utilization{uuid=\"%s\"})", deviceUUID)
	case biz.AscendGPUDevice:
		query = fmt.Sprintf("avg(npu_chip_info_utilization{vdie_id=\"%s\"})", deviceUUID)
	case biz.HygonGPUDevice:
		query = fmt.Sprintf("avg(dcu_utilizationrate{device_id=\"%s\"})", deviceUUID)
	default:
		return 0, errors.New("provider not exists")
	}
	return s.queryInstantVal(ctx, query)
}

// 任务算力利用率
func (s *MetricsGenerator) taskCoreUsed(ctx context.Context, provider, namespace, pod, container, podUUID, deviceUUID string) (float32, error) {
	query := ""
	switch provider {
	case biz.NvidiaGPUDevice:
		// query = fmt.Sprintf("avg(Device_utilization_desc_of_container{deviceuuid=\"%s\", podnamespace=\"%s\", podname=\"%s\", ctrname=\"%s\"})", deviceUUID, namespace, pod, container)
		//		queryTemplate := `last_over_time((Device_utilization_desc_of_container{deviceuuid="%s", podnamespace="%s", podname="%s", ctrname="%s"} != 0)[1m:])
		// or
		// last_over_time(Device_utilization_desc_of_container{deviceuuid="%s", podnamespace="%s", podname="%s", ctrname="%s"}[1m:])`
		//		query = fmt.Sprintf(queryTemplate, deviceUUID, namespace, pod, container, deviceUUID, namespace, pod, container)
		queryTemplate := fmt.Sprintf("Device_utilization_desc_of_container{deviceuuid=\"%s\", podnamespace=\"%s\", podname=\"%s\", ctrname=\"%s\"}", deviceUUID, namespace, pod, container)
		query = fmt.Sprintf("sum_over_time(%s[1m]) == 0 or (sum_over_time(%s[10m:]) / count_over_time(( %s !=0)[10m:])) ", queryTemplate, queryTemplate, queryTemplate)
		// query = queryTemplate
	case biz.CambriconGPUDevice:
		query = fmt.Sprintf("avg(mlu_utilization * on(uuid) group_right mlu_container{namespace=\"%s\",pod=\"%s\",container=\"%s\",type=\"mlu370.smlu.vcore\"})", namespace, pod, container)
	case biz.AscendGPUDevice:
		query = fmt.Sprintf("avg(container_npu_utilization{exported_namespace=\"%s\", pod_name=\"%s\", container_name=\"%s\"})", namespace, pod, container)
	case biz.HygonGPUDevice:
		// vdcu
		query = fmt.Sprintf("avg(vdcu_utilizationrate{dcu_pod_name=\"%s\", container=\"%s\"})", pod, container)
		if v, err := s.queryInstantVal(ctx, query); err == nil && v > 0 {
			return v, nil
		} else if err != nil {
			return 0, err
		}
		// dcu
		query = fmt.Sprintf("avg(dcu_utilizationrate{dcu_pod_name=\"%s\", container=\"%s\"})", pod, container)
	case biz.MetaxGPUDevice:
		// sgpu
		query = fmt.Sprintf("avg(mx_sgpu_usage{exported_pod=\"%s\", exported_container=\"%s\", exported_namespace=\"%s\"})", pod, container, namespace)
		if v, err := s.queryInstantVal(ctx, query); err == nil && v > 0 {
			return v, nil
		} else if err != nil {
			return 0, err
		}
		// gpu
		query = fmt.Sprintf("avg(mx_gpu_usage{exported_pod=\"%s\", exported_container=\"%s\", exported_namespace=\"%s\"})", pod, container, namespace)
	default:
		return 0, errors.New("provider not exists")
	}
	return s.queryInstantVal(ctx, query)
}

// 任务显存使用量
func (s *MetricsGenerator) taskMemoryUsed(ctx context.Context, provider, namespace, pod, container, podUUID, deviceUUID string) (float32, error) {
	query := ""
	switch provider {
	case biz.NvidiaGPUDevice:
		query = fmt.Sprintf("avg(vGPU_device_memory_usage_in_bytes{deviceuuid=\"%s\", podnamespace=\"%s\", podname=\"%s\", ctrname=\"%s\"})", deviceUUID, namespace, pod, container)
	case biz.CambriconGPUDevice:
		query = fmt.Sprintf("avg(mlu_memory_utilization * on(uuid) group_right mlu_container{namespace=\"%s\",pod=\"%s\",container=\"%s\",type=\"mlu370.smlu.vmemory\"})", namespace, pod, container)
	case biz.AscendGPUDevice:
		query = fmt.Sprintf("avg(container_npu_used_memory{exported_namespace=\"%s\", pod_name=\"%s\", container_name=\"%s\"})", namespace, pod, container)
	case biz.HygonGPUDevice:
		// vdcu
		query = fmt.Sprintf("avg(vdcu_usedmemory_bytes{dcu_pod_name=\"%s\", container=\"%s\"})", pod, container)
		if v, err := s.queryInstantVal(ctx, query); err == nil && v > 0 {
			return v, nil
		} else if err != nil {
			return 0, err
		}
		// dcu
		query = fmt.Sprintf("avg(dcu_usedmemory_bytes{dcu_pod_name=\"%s\", container=\"%s\"})", pod, container)
	case biz.MetaxGPUDevice:
		// sgpu
		query = fmt.Sprintf("avg(mx_sgpu_used_memory{exported_pod=\"%s\", exported_container=\"%s\", exported_namespace=\"%s\"})", pod, container, namespace)
		if v, err := s.queryInstantVal(ctx, query); err == nil && v > 0 {
			return v, nil
		} else if err != nil {
			return 0, err
		}
		// gpu
		query = fmt.Sprintf("avg(mx_memory_used{exported_pod=\"%s\", exported_container=\"%s\", exported_namespace=\"%s\",  type=\"vram\"})", pod, container, namespace)
	default:
		return 0, errors.New("provider not exists")
	}
	return s.queryInstantVal(ctx, query)
}

// GPU温度
func (s *MetricsGenerator) gpuTemperature(ctx context.Context, provider, deviceUUID string) (float32, error) {
	query := ""
	switch provider {
	case biz.NvidiaGPUDevice:
		query = fmt.Sprintf("avg(DCGM_FI_DEV_GPU_TEMP{UUID=\"%s\"})", deviceUUID)
	case biz.CambriconGPUDevice:
		query = fmt.Sprintf("avg(mlu_temperature{uuid=\"%s\"})", deviceUUID)
	case biz.AscendGPUDevice:
		query = fmt.Sprintf("avg(npu_chip_info_temperature{vdie_id=\"%s\"})", deviceUUID)
	case biz.HygonGPUDevice:
		query = fmt.Sprintf("avg(dcu_temp{device_id=\"%s\"})", deviceUUID)
	default:
		return 0, errors.New("provider not exists")
	}
	return s.queryInstantVal(ctx, query)
}

// 显存温度
func (s *MetricsGenerator) memoryTemperature(ctx context.Context, provider, deviceUUID string) (float32, error) {
	query := ""
	switch provider {
	case biz.NvidiaGPUDevice:
		query = fmt.Sprintf("avg(DCGM_FI_DEV_MEMORY_TEMP{UUID=\"%s\"})", deviceUUID)
	case biz.CambriconGPUDevice:
		query = fmt.Sprintf("avg(mlu_memory_temperature{uuid=\"%s\"})", deviceUUID)
	case biz.AscendGPUDevice:
		query = fmt.Sprintf("avg(npu_chip_info_temperature{vdie_id=\"%s\"})", deviceUUID)
	default:
		return 0, errors.New("provider not exists")
	}
	return s.queryInstantVal(ctx, query)
}

// 功耗
func (s *MetricsGenerator) gpuPower(ctx context.Context, provider, deviceUUID string) (float32, error) {
	query := ""
	switch provider {
	case biz.NvidiaGPUDevice:
		query = fmt.Sprintf("avg(DCGM_FI_DEV_POWER_USAGE{UUID=\"%s\"})", deviceUUID)
	case biz.CambriconGPUDevice:
		query = fmt.Sprintf("avg(mlu_power_usage{uuid=\"%s\"})", deviceUUID)
	case biz.AscendGPUDevice:
		query = fmt.Sprintf("avg(npu_chip_info_power{vdie_id=\"%s\"})", deviceUUID)
	case biz.HygonGPUDevice:
		query = fmt.Sprintf("avg(dcu_power_usage{device_id=\"%s\"})", deviceUUID)
	default:
		return 0, errors.New("provider not exists")
	}
	return s.queryInstantVal(ctx, query)
}

// 硬件健康
func (s *MetricsGenerator) gpuHardwareHealth(ctx context.Context, provider, deviceUUID string) (float32, error) {
	query := ""
	switch provider {
	case biz.NvidiaGPUDevice:
		query = fmt.Sprintf("avg(DCGM_FI_DEV_XID_ERRORS{UUID=\"%s\"})", deviceUUID)
	default:
		return 0, nil
	}
	return s.queryInstantVal(ctx, query)
}

// 风扇转速
func (s *MetricsGenerator) fanSpeed(ctx context.Context, provider, deviceUUID string) (float32, error) {
	query := ""
	switch provider {
	case biz.NvidiaGPUDevice:
		query = fmt.Sprintf("avg(DCGM_FI_DEV_FAN_SPEED{UUID=\"%s\"})", deviceUUID)
	case biz.CambriconGPUDevice:
		query = fmt.Sprintf("avg(mlu_fan_speed{uuid=\"%s\"})", deviceUUID)
	case biz.AscendGPUDevice:
		query = fmt.Sprintf("avg(npu_chip_link_speed{vdie_id=\"%s\"})", deviceUUID)
	default:
		return 0, errors.New("provider not exists")
	}
	return s.queryInstantVal(ctx, query)
}

type DeviceAdditionalInfo struct {
	DriverVersion string // 驱动版本
	DeviceNo      string // 设备号
}

// 设备的驱动版本、设备号等信息
func (s *MetricsGenerator) queryDeviceAdditional(ctx context.Context, provider, deviceUUID string) (*DeviceAdditionalInfo, error) {
	query := ""
	switch provider {
	case biz.NvidiaGPUDevice:
		query = fmt.Sprintf("DCGM_FI_DEV_POWER_USAGE{UUID=\"%s\"}", deviceUUID)
	case biz.AscendGPUDevice:
		query = fmt.Sprintf("npu_chip_info_power{vdie_id=\"%s\"}", deviceUUID)
	case biz.CambriconGPUDevice:
		query = fmt.Sprintf("mlu_power_usage{uuid=\"%s\"}", deviceUUID)
	case biz.HygonGPUDevice:
		query = fmt.Sprintf("dcu_power_usage{device_id=\"%s\"}", deviceUUID)
	default:
		return nil, errors.New("provider not exists")
	}
	res, err := s.monitorService.QueryInstant(ctx, &pb.QueryInstantRequest{
		Query: query,
	})
	if err != nil {
		return nil, err
	}
	if len(res.Data) > 0 {
		sample := res.Data[0]
		metric := sample.Metric
		info := &DeviceAdditionalInfo{}
		switch provider {
		case biz.NvidiaGPUDevice:
			info.DriverVersion = metric["DCGM_FI_DRIVER_VERSION"]
			info.DeviceNo = metric["device"]
		case biz.CambriconGPUDevice:
			info.DriverVersion = metric["driver"]
			info.DeviceNo = metric["sn"]
		case biz.AscendGPUDevice:
			info.DriverVersion = "暂无"
			info.DeviceNo = "ascend-" + metric["id"]
		case biz.HygonGPUDevice:
			info.DriverVersion = "暂无"
			info.DeviceNo = "dcu-" + metric["minor_number"]
		}
		return info, nil
	}
	return nil, fmt.Errorf("unknown error")
}

func (s *MetricsGenerator) systemComponentHealth(ctx context.Context, componentType, componentName string) (float32, error) {
	query := ""
	switch componentType {
	case biz.ComponentTypeDeployment:
		query = fmt.Sprintf("ALERTS{alertname=\"KubeDeploymentReplicasMismatch\",deployment=\"%s\"} ", componentName)
	case biz.ComponentTypeStatefulSet:
		query = fmt.Sprintf("ALERTS{alertname=\"KubeStatefulSetReplicasMismatch\", statefulset=\"%s\"}", componentName)
	case biz.ComponentTypeDaemonSet:
		query = fmt.Sprintf("(kube_daemonset_status_desired_number_scheduled{daemonset=\"%s\"} > kube_daemonset_status_number_available{daemonset=\"%s\"})", componentName, componentName)
	default:
		return 0, errors.New("componentType not exists")
	}
	return s.queryInstantVal(ctx, query)
}
