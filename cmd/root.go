package cmd

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"text/template"

	"github.com/spf13/cobra"
)

var (
	//go:embed tmpl
	projectTemplates embed.FS
	dddMode          bool
)

type Project struct {
	ProjectModule string
	AppName       string
}

type fileTemplate struct {
	SourcePath string
	OutputPath string
}

var rootCmd = &cobra.Command{
	Use:   "go-fiber-starter <project-name>",
	Short: "一个用于快速创建 Go Fiber v3 Clean Architecture 项目的工具",
	Long: `此工具可以帮助你快速初始化一个基于 Fiber v3 和依赖注入 (Wire) 的 Go 项目。
它生成了一个干净的项目骨架，为后续的自动化代码生成做好了准备。`,
	Args: cobra.ExactArgs(1),
	// 修改 Run 函数以处理 dddMode 标志
	Run: func(cmd *cobra.Command, args []string) {
		projectName := args[0]
		if dddMode {
			createDDDProject(projectName)
		} else {
			createProject(projectName)
		}
	},
}

func init() {
	// 添加 --ddd 标志
	rootCmd.Flags().BoolVar(&dddMode, "ddd", false, "使用领域驱动设计 (DDD) 结构初始化项目")
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

	templates := []fileTemplate{
		{SourcePath: "tmpl/go.mod.tmpl", OutputPath: "go.mod"},
		{SourcePath: "tmpl/main.go.tmpl", OutputPath: "cmd/" + project.AppName + "/main.go"},
		{SourcePath: "tmpl/config.yaml.tmpl", OutputPath: "config.yaml"},
		{SourcePath: "tmpl/gitignore.tmpl", OutputPath: ".gitignore"},
		{SourcePath: "tmpl/configuration/config.go.tmpl", OutputPath: "internal/configuration/config.go"},
		{SourcePath: "tmpl/db/db.go.tmpl", OutputPath: "internal/db/db.go"},
		{SourcePath: "tmpl/router/router.go.tmpl", OutputPath: "internal/router/router.go"},
		{SourcePath: "tmpl/di/container.go.tmpl", OutputPath: "internal/di/container.go"},
	}

	if err := os.Mkdir(project.ProjectModule, 0o755); err != nil {
		fmt.Printf("创建项目目录失败: %s\n", err)
		return
	}

	for _, t := range templates {
		outputDir := filepath.Dir(filepath.Join(project.ProjectModule, t.OutputPath))
		if err := os.MkdirAll(outputDir, 0o755); err != nil {
			fmt.Printf("创建子目录 '%s' 失败: %s\n", outputDir, err)
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
		if err := os.MkdirAll(fullPath, 0o755); err != nil {
			fmt.Printf("创建空目录 '%s' 失败: %s\n", dir, err)
		} else {
			fmt.Printf(" ✓ 创建目录: %s\n", fullPath)
		}
	}

	fmt.Printf("\n 项目 '%s' 初始化成功！\n", project.ProjectModule)
	fmt.Println("👉 下一步:")
	fmt.Printf("   1. cd %s\n", project.ProjectModule)
	fmt.Println("   2. go mod tidy")
	fmt.Printf("   3. go run ./cmd/%s\n", project.AppName)
}

// 新增 createDDDProject 函数
func createDDDProject(projectName string) {
	project := Project{
		ProjectModule: projectName,
		AppName:       filepath.Base(projectName),
	}
	fmt.Printf("🚀 开始初始化 DDD 项目: %s\n", project.ProjectModule)

	// DDD 模式使用不同的模板和输出路径
	templates := []fileTemplate{
		{SourcePath: "tmpl/go.mod.tmpl", OutputPath: "go.mod"},
		{SourcePath: "tmpl/main.go.ddd.tmpl", OutputPath: "cmd/" + project.AppName + "/main.go"},
		{SourcePath: "tmpl/config.yaml.tmpl", OutputPath: "config.yaml"},
		{SourcePath: "tmpl/gitignore.tmpl", OutputPath: ".gitignore"},
		{SourcePath: "tmpl/configuration/config.go.tmpl", OutputPath: "internal/configuration/config.go"},
		{SourcePath: "tmpl/db/db.go.ddd.tmpl", OutputPath: "internal/infrastructure/persistence/db.go"},
		{SourcePath: "tmpl/router/router.go.tmpl", OutputPath: "internal/infrastructure/router/router.go"},
		{SourcePath: "tmpl/di/container.go.ddd.tmpl", OutputPath: "internal/di/container.go"},
	}

	if err := os.Mkdir(project.ProjectModule, 0o755); err != nil {
		fmt.Printf("创建项目目录失败: %s\n", err)
		return
	}

	for _, t := range templates {
		outputDir := filepath.Dir(filepath.Join(project.ProjectModule, t.OutputPath))
		if err := os.MkdirAll(outputDir, 0o755); err != nil {
			fmt.Printf("创建子目录 '%s' 失败: %s\n", outputDir, err)
			return
		}
		createFileFromTemplate(project, t.SourcePath, t.OutputPath)
	}

	// DDD 模式的目录结构
	emptyDirs := []string{
		"internal/application/service",
		"internal/domain/entity",
		"internal/domain/repository",
		"internal/domain/model",
		"internal/infrastructure/middleware",
		"internal/interfaces/handler",
		"internal/interfaces/dto",
	}
	for _, dir := range emptyDirs {
		fullPath := filepath.Join(projectName, dir)
		if err := os.MkdirAll(fullPath, 0o755); err != nil {
			fmt.Printf("创建空目录 '%s' 失败: %s\n", dir, err)
		} else {
			fmt.Printf(" ✓ 创建目录: %s\n", fullPath)
		}
	}

	fmt.Printf("\n DDD 项目 '%s' 初始化成功！\n", project.ProjectModule)
	fmt.Println("👉 下一步:")
	fmt.Printf("   1. cd %s\n", project.ProjectModule)
	fmt.Println("   2. go mod tidy")
	fmt.Printf("   3. go run ./cmd/%s\n", project.AppName)
}

func createFileFromTemplate(p Project, tmplPath, outputName string) {
	tmpl, err := template.ParseFS(projectTemplates, tmplPath)
	if err != nil {
		fmt.Printf("读取嵌入的模板 '%s' 失败: %s\n", tmplPath, err)
		return
	}

	outputPath := filepath.Join(p.ProjectModule, outputName)
	file, err := os.Create(outputPath)
	if err != nil {
		fmt.Printf("创建文件 '%s' 失败: %s\n", outputPath, err)
		return
	}
	defer file.Close()

	if err := tmpl.Execute(file, p); err != nil {
		fmt.Printf("渲染模板 '%s' 失败: %s\n", tmplPath, err)
		return
	}
	fmt.Printf(" ✓ 创建文件: %s\n", outputPath)
}
