package util

import (
	"os"
	"os/exec"
)

func FormatImport() error {
	dir, _ := os.Getwd()
	os.Chdir(dir)
	cmd := exec.Command("goimports", "-l", "-w", ".")
	return cmd.Run()
}

func FormatFile() error {
	dir, _ := os.Getwd()
	os.Chdir(dir)
	cmd := exec.Command("gofumpt", "-l", "-w", "-extra", ".")
	return cmd.Run()
}
