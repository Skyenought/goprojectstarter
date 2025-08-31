package cmd

import (
	"bufio"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"github.com/Skyenought/goprojectstarter/pkg/common"

	"github.com/spf13/cobra"
)

// RouteInfo 存储从 router.go 中解析出的路由信息
type RouteInfo struct {
	HTTPMethod string
	Path       string
	Handler    string // e.g., "UserHandler"
	Function   string // e.g., "Create"
}

var syncRoutesCmd = &cobra.Command{
	Use:   "sync-routes",
	Short: "根据 router.go 自动更新或创建 handler 中的 @Router 注释",
	Long: `此命令会扫描项目中的路由定义文件, 提取路由信息。
它会自动更新或为缺少注释的处理器方法创建完整的 Swagger 注释块, 确保文档与代码同步。`,
	Run: runSyncRoutes,
}

func init() {
	rootCmd.AddCommand(syncRoutesCmd)
}

func runSyncRoutes(cmd *cobra.Command, args []string) {
	fmt.Println("🔍 开始同步路由注释...")

	routerPath := findRouterPath()
	if routerPath == "" {
		fmt.Println("❌ 未能找到 router.go 文件。")
		return
	}
	fmt.Printf("   - 正在解析路由文件: %s\n", routerPath)

	routes, err := parseRoutes(routerPath)
	if err != nil {
		fmt.Printf("❌ 解析路由文件失败: %v\n", err)
		return
	}
	if len(routes) == 0 {
		fmt.Println("⚠️ 在路由文件中没有找到可识别的路由定义。")
		return
	}
	fmt.Printf("   - 成功解析到 %d 个路由定义。\n", len(routes))

	handlerDirs := findHandlerDirs()
	if len(handlerDirs) == 0 {
		fmt.Println("❌ 未能找到任何 handler 目录。")
		return
	}

	for _, dir := range handlerDirs {
		err := updateHandlersInDir(dir, routes)
		if err != nil {
			fmt.Printf("❌ 更新目录 %s 中的 handler 失败: %v\n", dir, err)
		}
	}

	common.FormatImport()
	common.FormatFile()

	fmt.Println("✅ 路由注释同步完成！")
}

func parseRoutes(path string) (map[string]RouteInfo, error) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return nil, err
	}

	routes := make(map[string]RouteInfo)
	var currentGroupPrefix string

	ast.Inspect(node, func(n ast.Node) bool {
		if as, ok := n.(*ast.AssignStmt); ok {
			if call, ok := as.Rhs[0].(*ast.CallExpr); ok {
				if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Group" {
					if len(call.Args) > 0 {
						if pathLit, ok := call.Args[0].(*ast.BasicLit); ok {
							currentGroupPrefix = "/api/v1" + strings.Trim(pathLit.Value, `"`)
						}
					}
				}
			}
		}

		if call, ok := n.(*ast.CallExpr); ok {
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
				httpMethod := strings.ToUpper(sel.Sel.Name)
				if !isHTTPMethod(httpMethod) {
					return true
				}

				if len(call.Args) < 2 {
					return true
				}

				pathLit, okPath := call.Args[0].(*ast.BasicLit)
				handlerSel, okHandler := call.Args[1].(*ast.SelectorExpr)

				if okPath && okHandler {
					routePath := currentGroupPrefix + strings.Trim(pathLit.Value, `"`)
					routePath = strings.Replace(routePath, "//", "/", -1)

					innerSel, okInner := handlerSel.X.(*ast.SelectorExpr)
					if !okInner {
						return true
					}

					handlerName := innerSel.Sel.Name
					functionName := handlerSel.Sel.Name

					key := fmt.Sprintf("%s.%s", handlerName, functionName)
					routes[key] = RouteInfo{
						HTTPMethod: httpMethod,
						Path:       routePath,
						Handler:    handlerName,
						Function:   functionName,
					}
				}
			}
		}
		return true
	})

	return routes, nil
}

func updateHandlersInDir(dir string, routes map[string]RouteInfo) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !strings.HasSuffix(info.Name(), "_handler.go") {
			return nil
		}
		fmt.Printf("   - 正在扫描: %s\n", path)
		return updateHandlerFile(path, routes)
	})
}

func updateHandlerFile(path string, routes map[string]RouteInfo) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	re := regexp.MustCompile(`func \(.*? \*(\w+)\) (\w+)\(.*?\)`)

	var newLines []string
	changed := false
	i := 0
	for i < len(lines) {
		line := lines[i]
		matches := re.FindStringSubmatch(line)

		if len(matches) == 3 {
			handlerName := matches[1]
			functionName := matches[2]
			key := fmt.Sprintf("%s.%s", handlerName, functionName)

			if routeInfo, ok := routes[key]; ok {
				// 找到一个有路由定义的方法，检查它上面是否有注释
				commentBlockEnd := i - 1
				commentBlockStart := -1

				// 向上查找注释块的起始位置
				for j := commentBlockEnd; j >= 0; j-- {
					if !strings.HasPrefix(strings.TrimSpace(lines[j]), "//") {
						commentBlockStart = j + 1
						break
					}
					if j == 0 {
						commentBlockStart = 0
					}
				}

				if commentBlockStart == -1 || commentBlockStart > commentBlockEnd {
					fmt.Printf("     - 为方法 %s 生成新的 Swagger 注释\n", functionName)
					commentBlock := generateDefaultSwaggerComments(routeInfo)
					newLines = append(newLines, commentBlock...)
					changed = true
				} else {
					hasRouterTag := false
					for j := commentBlockStart; j <= commentBlockEnd; j++ {
						if strings.HasPrefix(strings.TrimSpace(lines[j]), "// @Router") {
							newRouterLine := fmt.Sprintf("// @Router       %s [%s]", routeInfo.Path, strings.ToLower(routeInfo.HTTPMethod))
							if lines[j] != newRouterLine {
								fmt.Printf("     - 更新方法 %s: %s -> %s\n", functionName, strings.TrimSpace(lines[j]), strings.TrimSpace(newRouterLine))
								lines[j] = newRouterLine
								changed = true
							}
							hasRouterTag = true
							break
						}
					}
					// 如果有注释块但没有 @Router 标签, 则在末尾添加
					if !hasRouterTag {
						fmt.Printf("     - 为方法 %s 添加缺失的 @Router 注释\n", functionName)
						newRouterLine := fmt.Sprintf("// @Router       %s [%s]", routeInfo.Path, strings.ToLower(routeInfo.HTTPMethod))
						// 插入到注释块的最后一行
						lines[commentBlockEnd] += "\n" + newRouterLine
						changed = true
					}
				}
			}
		}
		newLines = append(newLines, line)
		i++
	}

	if changed {
		fmt.Printf("   - 正在写回文件: %s\n", path)
		// 如果是 Case 2 的情况, lines 已经被修改, 所以直接用 lines
		if len(newLines) == len(lines) {
			return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644)
		}
		// 如果是 Case 1 的情况, newLines 是全新的, 用 newLines
		return os.WriteFile(path, []byte(strings.Join(newLines, "\n")), 0o644)
	}

	return nil
}

func generateDefaultSwaggerComments(info RouteInfo) []string {
	handlerTag := strings.TrimSuffix(info.Handler, "Handler")
	summary := camelCaseToWords(info.Function)

	return []string{
		fmt.Sprintf("// %s", info.Function),
		fmt.Sprintf("// @Summary      %s", summary),
		fmt.Sprintf("// @Description  %s", summary),
		fmt.Sprintf("// @Tags         %s", handlerTag),
		"// @Accept       json",
		"// @Produce      json",
		"// @Param        id   path      int  true  \"Some ID\"",
		"// @Success      200  {object}  map[string]interface{}",
		"// @Failure      400  {object}  map[string]interface{}",
		"// @Failure      500  {object}  map[string]interface{}",
		fmt.Sprintf("// @Router       %s [%s]", info.Path, strings.ToLower(info.HTTPMethod)),
	}
}

func camelCaseToWords(s string) string {
	var result []rune
	if len(s) == 0 {
		return ""
	}

	for i, r := range s {
		if unicode.IsUpper(r) {
			if i > 0 {
				result = append(result, ' ')
			}
			result = append(result, r)
		} else {
			result = append(result, r)
		}
	}
	return strings.Title(string(result))
}

func isHTTPMethod(method string) bool {
	switch method {
	case "GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS":
		return true
	default:
		return false
	}
}

func findRouterPath() string {
	pathsToTry := []string{
		"internal/adapter/router/router.go",
		"internal/infrastructure/router/router.go",
		"internal/router/router.go",
	}
	for _, p := range pathsToTry {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func findHandlerDirs() []string {
	var dirs []string
	pathsToTry := []string{
		"internal/adapter/handler",
		"internal/interfaces/handler",
		"internal/handler",
	}
	for _, p := range pathsToTry {
		if _, err := os.Stat(p); err == nil {
			dirs = append(dirs, p)
		}
	}
	return dirs
}
