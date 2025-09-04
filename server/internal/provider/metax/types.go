package metax

type DeviceInfo struct {
	Uuid          string `json:"uuid"`
	Bdf           string `json:"bdf"`
	Model         string `json:"model"`
	TotalDevCount int    `json:"totalDevCount"`
	TotalCompute  int    `json:"totalCompute"`
	TotalVRam     int    `json:"totalVRam"`
	Numa          int    `json:"numa"`
	Healthy       bool   `json:"healthy"`
	QosPolicy     string `json:"qosPolicy,omitempty"`
}
