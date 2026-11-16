//go:build linux

package metrics

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	// rootFilesystem fija la medición a la partición raíz del servidor.
	rootFilesystem = "/"
	bytesPerGB     = 1024 * 1024 * 1024
)

type Collector struct {
	previousTotal uint64
	previousIdle  uint64
}

func NewCollector() *Collector {
	return &Collector{}
}

func (c *Collector) Snapshot() (Snapshot, error) {
	total, idle, err := readCPU()
	if err != nil {
		return Snapshot{}, err
	}

	usage := 0.0
	if c.previousTotal != 0 && total > c.previousTotal {
		totalDelta := total - c.previousTotal
		idleDelta := idle - c.previousIdle
		usage = float64(totalDelta-idleDelta) / float64(totalDelta) * 100
	}
	c.previousTotal, c.previousIdle = total, idle

	processes, err := readProcessCount()
	if err != nil {
		return Snapshot{}, err
	}
	totalMemory, availableMemory, err := readMemory()
	if err != nil {
		return Snapshot{}, err
	}

	disk, err := readRootDiskUsage()
	if err != nil {
		return Snapshot{}, err
	}

	return Snapshot{
		Timestamp:       time.Now().UTC().Format(time.RFC3339),
		CPUPercent:      usage,
		ActiveProcesses: processes,
		MemoryTotalMB:   totalMemory / 1024,
		MemoryUsedMB:    (totalMemory - availableMemory) / 1024,
		DiskTotalGB:     disk.totalBytes / bytesPerGB,
		DiskUsedGB:      disk.usedBytes / bytesPerGB,
	}, nil
}

type diskUsage struct {
	totalBytes uint64
	usedBytes  uint64
}

// readRootDiskUsage mide exclusivamente la partición raíz ("/") mediante
// syscall.Statfs_t. No usa el directorio de trabajo ni rutas configurables,
// para que el reporte siempre describa el mismo sistema de archivos.
func readRootDiskUsage() (diskUsage, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(rootFilesystem, &stat); err != nil {
		return diskUsage{}, fmt.Errorf("leer espacio en disco de %q: %w", rootFilesystem, err)
	}
	if stat.Bsize <= 0 {
		return diskUsage{}, fmt.Errorf("tamaño de bloque no válido en %q", rootFilesystem)
	}
	if stat.Bavail > stat.Blocks {
		return diskUsage{}, fmt.Errorf("datos de disco no válidos en %q", rootFilesystem)
	}

	blockSize := uint64(stat.Bsize)
	totalBytes := stat.Blocks * blockSize
	freeBytes := stat.Bavail * blockSize
	return diskUsage{
		totalBytes: totalBytes,
		usedBytes:  totalBytes - freeBytes,
	}, nil
}

func readCPU() (uint64, uint64, error) {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return 0, 0, fmt.Errorf("leer uso de CPU: %w", err)
	}
	line := strings.SplitN(string(data), "\n", 2)[0]
	fields := strings.Fields(line)
	if len(fields) < 5 || fields[0] != "cpu" {
		return 0, 0, fmt.Errorf("formato no válido en /proc/stat")
	}
	var total, idle uint64
	for i, field := range fields[1:] {
		value, err := strconv.ParseUint(field, 10, 64)
		if err != nil {
			return 0, 0, fmt.Errorf("valor de CPU no válido: %w", err)
		}
		total += value
		if i == 3 || i == 4 {
			idle += value
		}
	}
	return total, idle, nil
}

func readProcessCount() (int, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return 0, fmt.Errorf("contar procesos: %w", err)
	}
	count := 0
	for _, entry := range entries {
		if entry.IsDir() {
			if _, err := strconv.Atoi(entry.Name()); err == nil {
				count++
			}
		}
	}
	return count, nil
}

func readMemory() (uint64, uint64, error) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, 0, fmt.Errorf("leer memoria: %w", err)
	}
	var total, available uint64
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		value, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			continue
		}
		switch strings.TrimSuffix(fields[0], ":") {
		case "MemTotal":
			total = value
		case "MemAvailable":
			available = value
		}
	}
	if total == 0 || available > total {
		return 0, 0, fmt.Errorf("datos de memoria no válidos en /proc/meminfo")
	}
	return total, available, nil
}
