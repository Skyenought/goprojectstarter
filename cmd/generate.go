package cmd

import (
	"bytes"
	"embed"
	"fmt"
	"github.com/spf13/cobra"
	"go/ast"
	"go/parser"
	"go/token"
	"golang.org/x/mod/modfile"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"unicode"
)

var (
	//go:embed tmpl/generate
	generateTemplates embed.FS
	forceGenerate     bool
)

type FieldInfo struct {
	Name      string
	Type      string
	GormName  string
	LowerName string
}

type EntityInfo struct {
	ProjectModule   string
	EntityName      string
	LowerEntityName string
	TableName       string
	PrimaryKey      FieldInfo
	Fields          []FieldInfo
}

var generateCmd = &cobra.Command{
	Use:     "generate [entity-file-path]",
	Short:   "根据实体文件自动生成 Repository, Service, 和 Controller",
	Long:    `读取指定的 Go 实体文件，解析其结构和 GORM 标签，然后自动生成对应的 CRUD 代码层。`,
	Aliases: []string{"gen"},
	Args:    cobra.ExactArgs(1),
	Run:     runGenerate,
}

func init() {
	rootCmd.AddCommand(generateCmd)
	generateCmd.Flags().BoolVarP(&forceGenerate, "force", "F", false, "强制覆盖已存在的文件")
}

func runGenerate(cmd *cobra.Command, args []string) {
	entityFilePath := args[0]
	fmt.Printf("🔍 开始解析实体文件: %s\n", entityFilePath)

	module, err := getProjectModule()
	if err != nil {
		fmt.Printf("   获取项目 module 失败: %v\n", err)
		return // 正确：错误检查
	}

	info, err := parseEntityFile(entityFilePath, module)

	if err != nil {
		fmt.Printf("   解析实体文件失败: %v\n", err)
		fmt.Println("   请检查以下几点：")
		fmt.Println("   1. 确保文件路径正确。")
		fmt.Println("   2. 确保实体 struct 中有且仅有一个字段标记了 `gorm:\"primaryKey\"`。")
		fmt.Println("   3. 确保文件没有语法错误。")
		return
	}

	fmt.Printf(" ✓ 解析成功! 实体: %s, 表名: %s\n", info.EntityName, info.TableName)

	generateCode(info)

	if err := addProviderToDI(info); err != nil {
		fmt.Printf("   自动修改 di/container.go 失败: %v\n", err)
		return
	}
	if err := addHandlerToRouter(info); err != nil {
		fmt.Printf("   自动修改 router/router.go (注入 Controller) 失败: %v\n", err)
		return
	}
	if err := addRoutesToRouter(info); err != nil {
		fmt.Printf("   自动添加路由到 router/router.go 失败: %v\n", err)
		return
	}

	formatFile("internal/di/container.go")
	formatFile("internal/router/router.go")

	printNextSteps(info)
}

func generateCode(info *EntityInfo) {
	filesToGenerate := map[string]string{
		"model":      "tmpl/generate/model.go.tmpl",
		"repository": "tmpl/generate/repository.go.tmpl",
		"service":    "tmpl/generate/service.go.tmpl",
		"handler":    "tmpl/generate/handler.go.tmpl",
	}

	for pkg, tmplPath := range filesToGenerate {

		fileName := fmt.Sprintf("internal/%s/%s_%s.go", pkg, toSnakeCase(info.EntityName), pkg)
		if pkg == "model" {
			fileName = fmt.Sprintf("internal/%s/%s.go", pkg, toSnakeCase(info.EntityName))
		} else {
			fileName = fmt.Sprintf("internal/%s/%s_%s.go", pkg, toSnakeCase(info.EntityName), pkg)
		}
		fmt.Printf("  -> 正在处理 %s...\n", fileName)

		if _, err := os.Stat(fileName); err == nil {
			if !forceGenerate {
				fmt.Printf("  文件已存在，跳过生成。请使用 -F 或 --force 选项来覆盖。\n")
				continue
			}
			fmt.Printf("  文件已存在，正在强制覆盖...\n")
		} else if !os.IsNotExist(err) {
			fmt.Printf("  检查文件 %s 状态时出错: %v\n", fileName, err)
			continue
		}

		// 如果文件不存在，或者用户强制覆盖，则继续执行以下生成代码
		dir := filepath.Dir(fileName)
		if err := os.MkdirAll(dir, 0755); err != nil {
			fmt.Printf("  创建目录 %s 失败: %v\n", dir, err)
			continue
		}

		tmpl, err := template.ParseFS(generateTemplates, tmplPath)
		if err != nil {
			fmt.Printf("  读取嵌入的模板 %s 失败: %v\n", tmplPath, err)
			continue
		}

		var tpl bytes.Buffer
		if err := tmpl.Execute(&tpl, info); err != nil {
			fmt.Printf("  渲染模板 %s 失败: %v\n", tmplPath, err)
			continue
		}

		if err := os.WriteFile(fileName, tpl.Bytes(), 0644); err != nil {
			fmt.Printf("  写入文件 %s 失败: %v\n", fileName, err)
		} else {
			// 只有成功写入才打印成功信息
			fmt.Printf("  成功生成文件: %s\n", fileName)
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
						gormName = toSnakeCase(fieldName)
					}

					fieldInfo := FieldInfo{
						Name:      fieldName,
						Type:      fieldType,
						GormName:  gormName,
						LowerName: toLowerCamel(fieldName),
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
		info.TableName = toSnakeCase(info.EntityName) + "s"
		fmt.Printf("未找到 TableName() 方法, 将使用默认表名: %s\n", info.TableName)
	}

	return info, nil
}

// printNextSteps 现在变得非常简单
func printNextSteps(info *EntityInfo) {
	appName := filepath.Base(info.ProjectModule) // 从 module 路径推断 appName
	fmt.Println("\n✅ 代码已自动集成!")
	fmt.Println("👉 下一步:")
	fmt.Println("   1. go mod tidy")
	fmt.Println("   2. (可选) 查看 service 层 DTOs 并根据需要实现转换逻辑")
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

func toSnakeCase(s string) string {
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

func mustReadFile(filePath string, start, end token.Pos) []byte {
	content, err := os.ReadFile(filePath)
	if err != nil {
		panic(err)
	}
	return content[start:end]
}
