//go:build !linux

package metrics

import "fmt"

type Collector struct{}

func NewCollector() *Collector {
	return &Collector{}
}

func (*Collector) Snapshot() (Snapshot, error) {
	return Snapshot{}, fmt.Errorf("las métricas del servidor requieren Linux")
}
