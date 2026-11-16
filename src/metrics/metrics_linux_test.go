//go:build linux

package metrics

import "testing"

func TestReadRootDiskUsage(t *testing.T) {
	usage, err := readRootDiskUsage()
	if err != nil {
		t.Fatal(err)
	}
	if usage.totalBytes == 0 {
		t.Fatal("la partición raíz reportó cero bytes totales")
	}
	if usage.usedBytes > usage.totalBytes {
		t.Fatalf("uso de disco inválido: %d bytes usados de %d", usage.usedBytes, usage.totalBytes)
	}
}
