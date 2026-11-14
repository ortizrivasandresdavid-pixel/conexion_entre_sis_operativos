package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"operativos/src/shell"
)

func main() {
	input := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("Oper# ")
		if !input.Scan() {
			break
		}

		command := strings.TrimSpace(input.Text())
		if command == "exit" || command == "bye" {
			break
		}

		output, err := shell.Execute(context.Background(), command)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
		fmt.Print(output)
	}
	if err := input.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "Error al leer la consola:", err)
		os.Exit(1)
	}
}
