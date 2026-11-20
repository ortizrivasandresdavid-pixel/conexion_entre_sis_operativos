package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
	"unicode"

	"operativos/src/metrics"
	"operativos/src/sockets"
)

const (
	refreshInterval = time.Second
	maxHistoryItems = 250
	historyLines    = 8
)

type activityState struct {
	reports       []string
	commands      []string
	reportCount   int
	commandCount  int
	activeClients int
}

type inputLine struct {
	value string
	err   error
}

func main() {
	configFlag := flag.String("config", "", "archivo de configuración del servidor")
	eventFlag := flag.String("events", "", "archivo JSONL de eventos (sobrescribe EventFile)")
	flag.Parse()

	configPath, err := findConfigPath(*configFlag)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	config, err := loadServerConfig(configPath, *eventFlag)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := runMonitor(ctx, config, os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

func runMonitor(ctx context.Context, config serverConfig, input io.Reader, output io.Writer) error {
	if file, ok := input.(*os.File); !ok || !isTerminal(file) {
		return errors.New("el monitor requiere una terminal interactiva")
	}
	if file, ok := output.(*os.File); !ok || !isTerminal(file) {
		return errors.New("la salida del monitor debe ser una terminal interactiva")
	}

	_, noColor := os.LookupEnv("NO_COLOR")
	colors := !noColor
	if colors {
		_, _ = fmt.Fprint(output, "\x1b[?25l")
		defer func() {
			_, _ = fmt.Fprint(output, "\x1b[0m\x1b[?25h\n")
		}()
	}

	collector := metrics.NewCollector()
	tailer := newEventTail(config.EventFile)
	activities := activityState{}
	inputLines := make(chan inputLine)
	go readInput(ctx, input, inputLines)

	ticker := time.NewTicker(refreshInterval)
	defer ticker.Stop()

	for {
		snapshot, metricsErr := collector.Snapshot()
		active := isPortListening(config.Port)
		events, eventErr := tailer.ReadNew()
		activities.apply(events)
		if err := renderMonitor(output, config, snapshot, metricsErr, active, eventErr, activities, colors); err != nil {
			return fmt.Errorf("actualizar terminal: %w", err)
		}

		select {
		case <-ctx.Done():
			return nil
		case line, ok := <-inputLines:
			if !ok {
				return nil
			}
			if line.err != nil {
				return fmt.Errorf("leer entrada: %w", line.err)
			}
			switch strings.ToLower(strings.TrimSpace(line.value)) {
			case "q", "salir":
				return nil
			case "c", "limpiar":
				activities.reports = nil
				activities.commands = nil
			}
		case <-ticker.C:
		}
	}
}

func readInput(ctx context.Context, input io.Reader, output chan<- inputLine) {
	defer close(output)
	scanner := bufio.NewScanner(input)
	for scanner.Scan() {
		select {
		case output <- inputLine{value: scanner.Text()}:
		case <-ctx.Done():
			return
		}
	}
	if err := scanner.Err(); err != nil {
		select {
		case output <- inputLine{err: err}:
		case <-ctx.Done():
		}
	}
}

func isTerminal(file *os.File) bool {
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func (s *activityState) apply(events []sockets.GUIEvent) {
	for _, event := range events {
		s.activeClients = event.ActiveClients
		switch event.Type {
		case "report_sent":
			if event.Status == "enviado" {
				s.reportCount++
			}
			s.reports = appendBounded(s.reports, formatReport(event))
		case "command_received":
			s.commandCount++
			s.commands = appendBounded(s.commands, formatCommand(event))
		}
	}
}

func renderMonitor(
	output io.Writer,
	config serverConfig,
	snapshot metrics.Snapshot,
	metricsErr error,
	active bool,
	eventErr error,
	activities activityState,
	colors bool,
) error {
	var builder strings.Builder
	green, brightGreen, yellow, red, reset := "", "", "", "", ""
	if colors {
		green = "\x1b[32m"
		brightGreen = "\x1b[92m"
		yellow = "\x1b[33m"
		red = "\x1b[31m"
		reset = "\x1b[0m"
	}
	builder.WriteString("\x1b[2J\x1b[H")
	fmt.Fprintf(&builder, "%sOPERATIVOS // MONITOR DEL SERVIDOR%s\n", brightGreen, reset)
	fmt.Fprintf(&builder, "Escucha: %s  |  Clientes: %d  |  Período de reporte: %d s\n",
		formatAddress(config.ListenIP, config.Port), activities.activeClients, config.Period)
	if active {
		fmt.Fprintf(&builder, "Estado: %sSERVIDOR ACTIVO%s\n\n", green, reset)
	} else {
		fmt.Fprintf(&builder, "Estado: %sESPERANDO EVENTOS%s\n\n", yellow, reset)
	}

	builder.WriteString("RECURSOS DEL SERVIDOR\n")
	if metricsErr != nil {
		fmt.Fprintf(&builder, "  No se pudieron leer las métricas: %s\n", singleLine(metricsErr.Error()))
	} else {
		fmt.Fprintf(&builder, "  CPU     %s %5.1f%%\n", progressBar(snapshot.CPUPercent), clampPercent(snapshot.CPUPercent))
		memoryPercent := percentage(snapshot.MemoryUsedMB, snapshot.MemoryTotalMB)
		fmt.Fprintf(&builder, "  RAM     %s %d / %d MB (%.1f%%)\n",
			progressBar(memoryPercent), snapshot.MemoryUsedMB, snapshot.MemoryTotalMB, memoryPercent)
		diskPercent := percentage(snapshot.DiskUsedGB, snapshot.DiskTotalGB)
		fmt.Fprintf(&builder, "  Disco / %s %d / %d GB (%.1f%%)\n",
			progressBar(diskPercent), snapshot.DiskUsedGB, snapshot.DiskTotalGB, diskPercent)
		fmt.Fprintf(&builder, "  Procesos activos: %d\n", snapshot.ActiveProcesses)
	}

	if eventErr != nil {
		fmt.Fprintf(&builder, "\n%sFlujo de eventos no disponible: %s%s\n", red, singleLine(eventErr.Error()), reset)
	} else {
		fmt.Fprintf(&builder, "\n%sFlujo de eventos activo%s\n", green, reset)
	}

	fmt.Fprintf(&builder, "\nREPORTES ENVIADOS: %d\n", activities.reportCount)
	writeHistory(&builder, activities.reports)
	fmt.Fprintf(&builder, "\nCOMANDOS RECIBIDOS: %d\n", activities.commandCount)
	writeHistory(&builder, activities.commands)
	fmt.Fprintf(&builder, "\nComandos: %sc%s + Enter limpia el historial visible  |  %sq%s + Enter sale  |  Ctrl+C sale\n",
		brightGreen, reset, brightGreen, reset)
	_, err := io.WriteString(output, builder.String())
	return err
}

func writeHistory(output *strings.Builder, history []string) {
	start := len(history) - historyLines
	if start < 0 {
		start = 0
	}
	if start == len(history) {
		output.WriteString("  (sin eventos)\n")
		return
	}
	for _, line := range history[start:] {
		fmt.Fprintf(output, "  %s\n", line)
	}
}

func progressBar(value float64) string {
	const width = 20
	filled := int(math.Round(clampPercent(value) / 100 * width))
	return "[" + strings.Repeat("#", filled) + strings.Repeat("-", width-filled) + "]"
}

func appendBounded(items []string, value string) []string {
	items = append(items, value)
	if len(items) > maxHistoryItems {
		copy(items, items[len(items)-maxHistoryItems:])
		items = items[:maxHistoryItems]
	}
	return items
}

func formatReport(event sockets.GUIEvent) string {
	line := fmt.Sprintf("[%s] Reporte #%d %s a %s@%s", event.Clock, event.Report, event.Status, event.User, event.ClientIP)
	if event.Error != "" {
		line += ": " + singleLine(event.Error)
	}
	return line
}

func formatCommand(event sockets.GUIEvent) string {
	return fmt.Sprintf("[%s] %s@%s -> %s", event.Clock, event.User, event.ClientIP, singleLine(event.Command))
}

func singleLine(value string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, strings.ReplaceAll(strings.ReplaceAll(value, "\r", `\r`), "\n", `\n`))
}

func percentage(used, total uint64) float64 {
	if total == 0 {
		return 0
	}
	return clampPercent(float64(used) / float64(total) * 100)
}

func clampPercent(value float64) float64 {
	if math.IsNaN(value) || value < 0 {
		return 0
	}
	if value > 100 {
		return 100
	}
	return value
}
