package cmd

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"text/template"

	"github.com/spf13/cobra"
)

//go:embed tmpl
var projectTemplates embed.FS

type Project struct {
	ProjectModule string // 之前叫 Name，改成 ProjectModule 更清晰
	AppName       string // 应用名称，用于 cmd/app-name
}

// fileTemplate 定义了模板源文件和其在目标项目中的输出路径
type fileTemplate struct {
	SourcePath string
	OutputPath string
}

// rootCmd 是 cobra 应用的根命令
var rootCmd = &cobra.Command{
	Use:   "go-fiber-starter <project-name>",
	Short: "一个用于快速创建 Go Fiber v3 Clean Architecture 项目的工具",
	Long: `此工具可以帮助你快速初始化一个基于 Fiber v3 和依赖注入 (Wire) 的 Go 项目。
它生成了一个干净的项目骨架，为后续的自动化代码生成做好了准备。`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		projectName := args[0]
		createProject(projectName)
	},
}

func init() {
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func createProject(projectName string) {
	project := Project{
		ProjectModule: projectName,
		AppName:       filepath.Base(projectName),
	}
	fmt.Printf("🚀 开始初始化项目: %s\n", project.ProjectModule)
	// 路径依然需要 "tmpl/" 前缀，因为 embed.FS 保留了目录结构
	templates := []fileTemplate{
		{SourcePath: "tmpl/go.mod.tmpl", OutputPath: "go.mod"},
		// main.go 的新位置
		{SourcePath: "tmpl/main.go.tmpl", OutputPath: "cmd/" + project.AppName + "/main.go"},
		{SourcePath: "tmpl/config.yaml.tmpl", OutputPath: "config.yaml"},
		{SourcePath: "tmpl/Dockerfile.tmpl", OutputPath: "Dockerfile"},
		{SourcePath: "tmpl/.gitignore.tmpl", OutputPath: ".gitignore"},

		{SourcePath: "tmpl/configuration/config.go.tmpl", OutputPath: "internal/configuration/config.go"},
		{SourcePath: "tmpl/db/db.go.tmpl", OutputPath: "internal/db/db.go"},
		{SourcePath: "tmpl/router/router.go.tmpl", OutputPath: "internal/router/router.go"},
		{SourcePath: "tmpl/di/container.go.tmpl", OutputPath: "internal/di/container.go"},
	}

	if err := os.Mkdir(project.ProjectModule, 0755); err != nil {
		fmt.Printf("❌ 创建项目目录失败: %s\n", err)
		return
	}

	for _, t := range templates {
		outputDir := filepath.Dir(filepath.Join(project.ProjectModule, t.OutputPath))
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			fmt.Printf("❌ 创建子目录 '%s' 失败: %s\n", outputDir, err)
			return
		}
		createFileFromTemplate(project, t.SourcePath, t.OutputPath)
	}

	emptyDirs := []string{
		"internal/entity",
		"internal/model",
		"internal/repository",
		"internal/service",
		"internal/handler",
		"internal/middleware",
	}
	for _, dir := range emptyDirs {
		fullPath := filepath.Join(projectName, dir)
		if err := os.MkdirAll(fullPath, 0755); err != nil {
			fmt.Printf("❌ 创建空目录 '%s' 失败: %s\n", dir, err)
		} else {
			fmt.Printf(" ✓ 创建目录: %s\n", fullPath)
		}
	}

	fmt.Printf("\n🎉 项目 '%s' 初始化成功！\n", project.ProjectModule)
	fmt.Println("👉 下一步:")
	fmt.Printf("   1. cd %s\n", project.ProjectModule)
	fmt.Println("   2. go mod tidy")
	// go run 的目标也变了
	fmt.Printf("   3. go run ./cmd/%s\n", project.AppName)
}

func createFileFromTemplate(p Project, tmplPath, outputName string) {
	tmpl, err := template.ParseFS(projectTemplates, tmplPath)
	if err != nil {
		fmt.Printf("❌ 读取嵌入的模板 '%s' 失败: %s\n", tmplPath, err)
		return
	}

	outputPath := filepath.Join(p.ProjectModule, outputName)
	file, err := os.Create(outputPath)
	if err != nil {
		fmt.Printf("❌ 创建文件 '%s' 失败: %s\n", outputPath, err)
		return
	}
	defer file.Close()

	if err := tmpl.Execute(file, p); err != nil {
		fmt.Printf("❌ 渲染模板 '%s' 失败: %s\n", tmplPath, err)
		return
	}
	fmt.Printf(" ✓ 创建文件: %s\n", outputPath)
}
