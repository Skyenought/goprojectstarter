package cmd

import (
	"bytes"
	"embed"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"
	"unicode"

	"github.com/Skyenought/goprojectstarter/pkg/common"

	"github.com/spf13/cobra"
	"golang.org/x/mod/modfile"
)

var (
	//go:embed tmpl/generate
	generateTemplates embed.FS
	forceGenerate     bool
	noCrudMethods     bool
)

// PathConfig 根据项目结构存储不同的路径和包名
type PathConfig struct {
	IsDDD              bool
	DIFile             string
	RouterFile         string
	DIImports          []string
	HandlerPackagePath string
}

// FileGenerationTask 定义了单个文件的生成任务
type FileGenerationTask struct {
	TemplatePath string
	OutputDir    string
	FileName     string
	Suffix       string // e.g., "_repository", "_service"
	IsSingular   bool   // for files like dto.go that don't have a suffix
}

type FieldInfo struct {
	Name          string
	Type          string
	GormName      string
	LowerName     string
	DTOType       string
	IsAssociation bool
	IsSlice       bool
	BaseType      string
}

type EntityInfo struct {
	ProjectModule   string
	EntityName      string
	LowerEntityName string
	TableName       string
	PrimaryKey      FieldInfo
	Fields          []FieldInfo
	NoCrudMethods   bool
}

var generateCmd = &cobra.Command{
	Use:     "generate [entity-file-path]",
	Short:   "根据实体文件自动生成 Repository, Service, 和 Handler",
	Long:    `根据检测到的项目结构 (标准或DDD), 读取指定的Go实体文件, 解析其结构, 并自动生成对应的CRUD代码层。`,
	Aliases: []string{"gen"},
	Args:    cobra.ExactArgs(1),
	Run:     runGenerate,
}

func init() {
	rootCmd.AddCommand(generateCmd)
	generateCmd.Flags().BoolVarP(&forceGenerate, "force", "F", false, "强制覆盖已存在的文件")
	generateCmd.Flags().BoolVar(&noCrudMethods, "no-crud", false, "不要生成 CRUD 模板方法")
}

// isDDDProject 通过检查关键目录是否存在来判断项目结构
func isDDDProject() bool {
	_, err := os.Stat("internal/application")
	return err == nil
}

func runGenerate(cmd *cobra.Command, args []string) {
	entityFilePath := args[0]
	fmt.Printf("🔍 开始解析实体文件: %s\n", entityFilePath)

	var paths PathConfig
	if isDDDProject() {
		fmt.Println("   检测到 DDD 项目结构")
		paths = PathConfig{
			IsDDD:              true,
			DIFile:             "internal/di/container.go",
			RouterFile:         "internal/infrastructure/router/router.go",
			HandlerPackagePath: "/internal/interfaces/handler",
			DIImports: []string{
				"/internal/infrastructure/persistence",
				"/internal/application/service",
				"/internal/interfaces/handler",
			},
		}
	} else {
		fmt.Println("   检测到整洁架构")
		paths = PathConfig{
			IsDDD:              false,
			DIFile:             "internal/di/container.go",
			RouterFile:         "internal/adapter/router/router.go",
			HandlerPackagePath: "/internal/adapter/handler",
			DIImports: []string{
				"/internal/adapter/repository",
				"/internal/usecase/service",
				"/internal/adapter/handler",
			},
		}
	}

	module, err := getProjectModule()
	if err != nil {
		fmt.Printf("   获取项目 module 失败: %v\n", err)
		return
	}

	info, err := parseEntityFile(entityFilePath, module)
	if err != nil {
		fmt.Printf("   解析实体文件失败: %v\n", err)
		return
	}
	info.NoCrudMethods = noCrudMethods
	fmt.Printf(" ✓ 解析成功! 实体: %s, 表名: %s\n", info.EntityName, info.TableName)

	generateCode(info, paths)

	if err := addProviderToDI(info, paths); err != nil {
		fmt.Printf("   自动修改 %s 失败: %v\n", paths.DIFile, err)
		return
	}
	if err := addHandlerToRouter(info, paths); err != nil {
		fmt.Printf("   自动修改 %s 失败: %v\n", paths.RouterFile, err)
		return
	}
	if !info.NoCrudMethods {
		if err := addRoutesToRouter(info, paths); err != nil {
			fmt.Printf("   自动添加路由到 %s 失败: %v\n", paths.RouterFile, err)
			return
		}
	}

	_ = common.FormatImport()
	_ = common.FormatFile()

	printNextSteps(info)
}

func generateCode(info *EntityInfo, paths PathConfig) {
	var tasks []FileGenerationTask
	if paths.IsDDD {
		tasks = []FileGenerationTask{
			{TemplatePath: "tmpl/generate/dto.go.ddd.tmpl", OutputDir: "internal/interfaces/dto", FileName: common.ToSnakeCase(info.EntityName), IsSingular: true},
			{TemplatePath: "tmpl/generate/mapper.go.ddd.tmpl", OutputDir: "internal/interfaces/dto", Suffix: "_mapper"},
			{TemplatePath: "tmpl/generate/repository_interface.go.ddd.tmpl", OutputDir: "internal/domain/repository", Suffix: "_repository"},
			{TemplatePath: "tmpl/generate/repository_impl.go.ddd.tmpl", OutputDir: "internal/infrastructure/persistence", Suffix: "_repository_impl"},
			{TemplatePath: "tmpl/generate/service.go.ddd.tmpl", OutputDir: "internal/application/service", Suffix: "_service"},
			{TemplatePath: "tmpl/generate/handler.go.ddd.tmpl", OutputDir: "internal/interfaces/handler", Suffix: "_handler"},
		}
	} else {
		tasks = []FileGenerationTask{
			{TemplatePath: "tmpl/generate/dto.go.tmpl", OutputDir: "internal/adapter/dto", FileName: common.ToSnakeCase(info.EntityName), IsSingular: true},
			{TemplatePath: "tmpl/generate/repository_interface.go.tmpl", OutputDir: "internal/domain/ports", Suffix: "_repository"},
			{TemplatePath: "tmpl/generate/repository_impl.go.tmpl", OutputDir: "internal/adapter/repository", Suffix: "_repository_impl"},
			{TemplatePath: "tmpl/generate/service.go.tmpl", OutputDir: "internal/usecase/service", Suffix: "_service"},
			{TemplatePath: "tmpl/generate/handler.go.tmpl", OutputDir: "internal/adapter/handler", Suffix: "_handler"},
		}
	}

	for _, task := range tasks {
		fileName := task.FileName
		if !task.IsSingular {
			fileName = common.ToSnakeCase(info.EntityName) + task.Suffix
		}
		fullPath := filepath.Join(task.OutputDir, fileName+".go")

		fmt.Printf("  -> 正在处理 %s...\n", fullPath)

		if _, err := os.Stat(fullPath); err == nil {
			if !forceGenerate {
				fmt.Printf("     文件已存在, 跳过生成。请使用 -F 或 --force 选项来覆盖。\n")
				continue
			}
			fmt.Printf("     文件已存在, 正在强制覆盖...\n")
		} else if !os.IsNotExist(err) {
			fmt.Printf("     检查文件 %s 状态时出错: %v\n", fullPath, err)
			continue
		}

		if err := os.MkdirAll(task.OutputDir, 0o755); err != nil {
			fmt.Printf("     创建目录 %s 失败: %v\n", task.OutputDir, err)
			continue
		}

		// 为模板添加自定义函数
		funcMap := template.FuncMap{
			"toLowerCamel": toLowerCamel,
		}

		tmpl, err := template.New(filepath.Base(task.TemplatePath)).Funcs(funcMap).ParseFS(generateTemplates, task.TemplatePath)
		if err != nil {
			fmt.Printf("     读取嵌入的模板 %s 失败: %v\n", task.TemplatePath, err)
			continue
		}

		var tpl bytes.Buffer
		if err := tmpl.Execute(&tpl, info); err != nil {
			fmt.Printf("     渲染模板 %s 失败: %v\n", task.TemplatePath, err)
			continue
		}

		if err := os.WriteFile(fullPath, tpl.Bytes(), 0o644); err != nil {
			fmt.Printf("     写入文件 %s 失败: %v\n", fullPath, err)
		} else {
			fmt.Printf("     成功生成文件: %s\n", fullPath)
		}
	}
}

func getProjectModule() (string, error) {
	modBytes, err := os.ReadFile("go.mod")
	if err != nil {
		return "", fmt.Errorf("无法读取 go.mod 文件, 请确保在项目根目录运行此命令")
	}
	return modfile.ModulePath(modBytes), nil
}

// isKnownType 检查是否是 Go 内置类型或常用库类型
func isKnownType(typeName string) bool {
	knownTypes := map[string]bool{
		"string": true,
		"int":    true, "int8": true, "int16": true, "int32": true, "int64": true,
		"uint": true, "uint8": true, "uint16": true, "uint32": true, "uint64": true,
		"float32": true, "float64": true,
		"bool":      true,
		"byte":      true,
		"rune":      true,
		"time.Time": true,
		"uuid.UUID": true,
	}
	return knownTypes[typeName]
}

// convertToDTOType 将实体类型转换为 DTO 响应类型
func convertToDTOType(entityType string) string {
	// 正则表达式匹配可选的 `[]` 或 `*` 前缀和一个大写字母开头的单词
	re := regexp.MustCompile(`^(\[]|\*)?([A-Z]\w*)$`)
	matches := re.FindStringSubmatch(entityType)

	if len(matches) == 3 {
		prefix := matches[1]
		baseType := matches[2]
		// 检查基本类型是否是已知类型 (如 time.Time)，如果是，则不加 "Response" 后缀
		if isKnownType(baseType) {
			return entityType
		}
		return prefix + baseType + "Response"
	}

	// 如果不匹配（例如是基本类型），则返回原类型
	return entityType
}

func parseEntityFile(filePath, projectModule string) (*EntityInfo, error) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filePath, nil, 0)
	if err != nil {
		return nil, err
	}

	info := &EntityInfo{
		ProjectModule: projectModule,
	}

	ast.Inspect(node, func(n ast.Node) bool {
		if typeSpec, ok := n.(*ast.TypeSpec); ok {
			if structType, ok := typeSpec.Type.(*ast.StructType); ok {
				info.EntityName = typeSpec.Name.Name
				info.LowerEntityName = toLowerCamel(info.EntityName)

				for _, field := range structType.Fields.List {
					if len(field.Names) == 0 {
						continue
					}
					fieldName := field.Names[0].Name
					fieldType := string(mustReadFile(filePath, field.Type.Pos()-1, field.Type.End()-1))

					var gormName string
					isPrimaryKey := false

					if field.Tag != nil {
						tag := strings.Trim(field.Tag.Value, "`")
						if strings.Contains(tag, "gorm:\"") {
							gormTag := strings.Split(strings.Split(tag, "gorm:\"")[1], "\"")[0]
							parts := strings.Split(gormTag, ";")
							for _, part := range parts {
								if strings.HasPrefix(part, "column:") {
									gormName = strings.Split(part, ":")[1]
								}
								if part == "primaryKey" {
									isPrimaryKey = true
								}
							}
						}
					}

					if gormName == "" {
						gormName = common.ToSnakeCase(fieldName)
					}

					// --- 新增的元数据解析逻辑 ---
					isSlice := strings.HasPrefix(fieldType, "[]")
					baseType := strings.TrimPrefix(strings.TrimPrefix(fieldType, "[]"), "*")
					isAssociation := !isKnownType(baseType) && unicode.IsUpper([]rune(baseType)[0])

					fieldInfo := FieldInfo{
						Name:          fieldName,
						Type:          fieldType,
						GormName:      gormName,
						LowerName:     toLowerCamel(fieldName),
						DTOType:       convertToDTOType(fieldType),
						IsAssociation: isAssociation,
						IsSlice:       isSlice,
						BaseType:      baseType,
					}
					info.Fields = append(info.Fields, fieldInfo)

					if isPrimaryKey {
						info.PrimaryKey = fieldInfo
					}
				}
			}
		}

		if fn, ok := n.(*ast.FuncDecl); ok && fn.Name.Name == "TableName" {
			if len(fn.Body.List) > 0 {
				if retStmt, ok := fn.Body.List[0].(*ast.ReturnStmt); ok {
					if len(retStmt.Results) > 0 {
						if basicLit, ok := retStmt.Results[0].(*ast.BasicLit); ok {
							info.TableName = strings.Trim(basicLit.Value, `"`)
						}
					}
				}
			}
		}
		return true
	})

	if info.EntityName == "" {
		return nil, fmt.Errorf("在文件中未找到任何 struct 定义")
	}
	if info.PrimaryKey.Name == "" {
		return nil, fmt.Errorf("未找到 gorm:\"primaryKey\" 标签，请在主键字段上明确添加")
	}
	if info.TableName == "" {
		info.TableName = common.ToSnakeCase(info.EntityName) + "s"
		fmt.Printf("   未找到 TableName() 方法, 将使用默认表名: %s\n", info.TableName)
	}

	return info, nil
}

func printNextSteps(info *EntityInfo) {
	cmd := exec.Command("goimports", "-l", "-w", ".")
	cmd.Run()

	appName := filepath.Base(info.ProjectModule)
	fmt.Println("\n✅ 代码已自动集成!")
	fmt.Println("👉 下一步:")
	fmt.Println("   1. 仔细检查新生成的 TODO 注释, 并实现业务逻辑。")
	fmt.Println("   2. go mod tidy")
	fmt.Printf("   3. go run ./cmd/%s\n", appName)
}

func toLowerCamel(s string) string {
	if s == "" {
		return ""
	}
	r := []rune(s)
	r[0] = unicode.ToLower(r[0])
	return string(r)
}

func mustReadFile(filePath string, start, end token.Pos) []byte {
	content, err := os.ReadFile(filePath)
	if err != nil {
		panic(err)
	}
	return content[start:end]
}
