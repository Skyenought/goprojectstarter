package cmd

import (
	"embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"text/template"

	"github.com/joho/godotenv"

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
	err := godotenv.Overload()
	if err != nil {
		fmt.Printf("警告：加载 .env 文件时出错: %v\n", err)
	}
	rootCmd.Flags().BoolVar(&dddMode, "ddd", false, "使用领域驱动设计 (DDD) 结构初始化项目")
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func finishProjectCreation(project Project) {
	// 进入项目目录
	if err := os.Chdir(project.ProjectModule); err != nil {
		fmt.Printf("无法进入项目目录 %s: %v\n", project.ProjectModule, err)
		return
	}
	defer os.Chdir("..") // 操作完成后返回上一级目录

	fmt.Println("📦 正在整理依赖 (go mod tidy)...")
	cmd := exec.Command("go", "mod", "tidy")
	if output, err := cmd.CombinedOutput(); err != nil {
		fmt.Printf("   go mod tidy 失败: %v\n", err)
		fmt.Println("   输出:", string(output))
	} else {
		fmt.Println(" ✓ 依赖整理完成。")
	}

	fmt.Printf("\n 项目 '%s' 初始化成功！\n", project.ProjectModule)
	fmt.Println("👉 下一步:")
	fmt.Printf("   1. cd %s\n", project.ProjectModule)
	// go mod tidy 已经自动执行，所以从提示中移除
	fmt.Printf("   2. go run ./cmd/%s\n", project.AppName)
}

func createProject(projectName string) {
	project := Project{
		ProjectModule: projectName,
		AppName:       filepath.Base(projectName),
	}
	// 将项目创建的日志消息更新，以反映新架构
	fmt.Printf("🚀 开始初始化实用的整洁架构项目: %s\n", project.ProjectModule)

	// 注意：文件模板的输出路径已更新为新结构
	templates := []fileTemplate{
		{SourcePath: "tmpl/go.mod.tmpl", OutputPath: "go.mod"},
		{SourcePath: "tmpl/main.go.tmpl", OutputPath: "cmd/" + project.AppName + "/main.go"},
		{SourcePath: "tmpl/config.yaml.tmpl", OutputPath: "config.yaml"},
		{SourcePath: "tmpl/gitignore.tmpl", OutputPath: ".gitignore"},
		{SourcePath: "tmpl/configuration/config.go.tmpl", OutputPath: "internal/configuration/config.go"},
		{SourcePath: "tmpl/db/db.go.tmpl", OutputPath: "internal/adapter/repository/db.go"},
		{SourcePath: "tmpl/router/router.go.tmpl", OutputPath: "internal/adapter/router/router.go"},
		{SourcePath: "tmpl/di/container.go.tmpl", OutputPath: "internal/di/container.go"},
		// middleware 相关模板
		{SourcePath: "tmpl/middleware/jwt/config.go.tmpl", OutputPath: "internal/adapter/middleware/jwt/config.go"},
		{SourcePath: "tmpl/middleware/jwt/jwt.go.tmpl", OutputPath: "internal/adapter/middleware/jwt/jwt.go"},
		{SourcePath: "tmpl/Makefile.tmpl", OutputPath: "Makefile"},
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
		"internal/domain/entity",
		"internal/domain/ports",
		"internal/usecase/service",
		"internal/adapter/handler",
		"internal/adapter/dto",
		"internal/adapter/middleware",
	}
	for _, dir := range emptyDirs {
		fullPath := filepath.Join(projectName, dir)
		if err := os.MkdirAll(fullPath, 0o755); err != nil {
			fmt.Printf("创建空目录 '%s' 失败: %s\n", dir, err)
		} else {
			fmt.Printf(" ✓ 创建目录: %s\n", fullPath)
		}
	}

	finishProjectCreation(project)
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
		{SourcePath: "tmpl/middleware/jwt/config.go.tmpl", OutputPath: "internal/infrastructure/middleware/jwt/config.go"},
		{SourcePath: "tmpl/middleware/jwt/jwt.go.tmpl", OutputPath: "internal/infrastructure/middleware/jwt/jwt.go"},
		{SourcePath: "tmpl/Makefile.tmpl", OutputPath: "Makefile"},
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

	finishProjectCreation(project)
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
