package common

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"unicode"
)

func GetProjectPaths() (ProjectPathConfig, error) {
	if _, err := os.Stat("internal/application"); err == nil {
		return ProjectPathConfig{
			RepoInterfaceDir: "internal/domain/repository",
			RepoImplDir:      "internal/infrastructure/persistence",
			ServiceDir:       "internal/application/service",
			HandlerDir:       "internal/interfaces/handler",
			RouterFile:       "internal/infrastructure/router/router.go",
		}, nil
	}
	if _, err := os.Stat("internal/usecase"); err == nil {
		return ProjectPathConfig{
			RepoInterfaceDir: "internal/domain/ports",
			RepoImplDir:      "internal/adapter/repository",
			ServiceDir:       "internal/usecase/service",
			HandlerDir:       "internal/adapter/handler",
			RouterFile:       "internal/adapter/router/router.go",
		}, nil
	}
	return ProjectPathConfig{}, fmt.Errorf("无法识别的项目结构，请确保在项目根目录运行")
}

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

func ToSnakeCase(s string) string {
	var result []rune
	for i, r := range s {
		if unicode.IsUpper(r) {
			if i > 0 && (unicode.IsLower(rune(s[i-1])) || (i+1 < len(s) && unicode.IsLower(rune(s[i+1])))) {
				result = append(result, '_')
			}
			result = append(result, unicode.ToLower(r))
		} else {
			result = append(result, r)
		}
	}
	return string(result)
}

func ToPluralSnakeCase(s string) string {
	snake := ToSnakeCase(s)
	if strings.HasSuffix(snake, "y") {
		return strings.TrimSuffix(snake, "y") + "ies"
	}
	if strings.HasSuffix(snake, "s") {
		return snake + "es"
	}
	return snake + "s"
}

func ToLowerCamel(s string) string {
	if s == "" {
		return ""
	}
	r := []rune(s)
	r[0] = unicode.ToLower(r[0])
	return string(r)
}
