package metrics

type Snapshot struct {
	Timestamp       string  `json:"timestamp"`
	CPUPercent      float64 `json:"cpu_percent"`
	ActiveProcesses int     `json:"active_processes"`
	MemoryTotalMB   uint64  `json:"memory_total_mb"`
	MemoryUsedMB    uint64  `json:"memory_used_mb"`
	DiskTotalGB     uint64  `json:"disk_total_gb"`
	DiskUsedGB      uint64  `json:"disk_used_gb"`
}
