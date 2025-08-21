package cmd

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"strings"
	"text/template"

	"golang.org/x/tools/go/ast/astutil"
)

// modifySourceFile 是一个高阶函数，用于读取、修改和写回 Go 源文件
func modifySourceFile(filePath string, modifier func(fset *token.FileSet, node *ast.File) error) error {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
	if err != nil {
		return fmt.Errorf("无法解析文件 %s: %w", filePath, err)
	}

	if err := modifier(fset, node); err != nil {
		return err
	}

	var buf bytes.Buffer
	if err := format.Node(&buf, fset, node); err != nil {
		return fmt.Errorf("格式化 AST 失败: %w", err)
	}

	return os.WriteFile(filePath, buf.Bytes(), 0644)
}

// addProviderToDI 自动将新的 providers 添加到 di/container.go，并且是幂等的
func addProviderToDI(info *EntityInfo) error {
	filePath := "internal/di/container.go"

	content, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	providerCheck := fmt.Sprintf("// %s Providers", info.EntityName)
	if strings.Contains(string(content), providerCheck) {
		fmt.Printf("  -> Providers for %s already exist in %s, skipping provider addition.\n", info.EntityName, filePath)
		return ensureImportsForDI(info)
	}

	fmt.Printf("  -> Modifying %s (adding providers)...\n", filePath)

	anchor := "// [GENERATOR ANCHOR] - Don't remove this comment!"
	providerTemplate := `
		// {{.EntityName}} Providers
		repository.New{{.EntityName}}Repository,
		service.New{{.EntityName}}Service,
		handler.New{{.EntityName}}Handler,
		` + anchor

	var tpl bytes.Buffer
	tmpl, err := template.New("providers").Parse(providerTemplate)
	if err != nil {
		return err
	}
	if err := tmpl.Execute(&tpl, info); err != nil {
		return err
	}

	newContent := strings.Replace(string(content), anchor, tpl.String(), 1)

	if err := os.WriteFile(filePath, []byte(newContent), 0644); err != nil {
		return err
	}

	return ensureImportsForDI(info)
}

// ensureImportsForDI 确保 DI 文件有正确的 imports
func ensureImportsForDI(info *EntityInfo) error {
	filePath := "internal/di/container.go"
	return modifySourceFile(filePath, func(fset *token.FileSet, node *ast.File) error {
		astutil.AddImport(fset, node, info.ProjectModule+"/internal/repository")
		astutil.AddImport(fset, node, info.ProjectModule+"/internal/service")
		astutil.AddImport(fset, node, info.ProjectModule+"/internal/handler")
		return nil
	})
}

// addHandlerToRouter 自动在 router/router.go 中注入 Handler，并且是幂等的
func addHandlerToRouter(info *EntityInfo) error {
	filePath := "internal/router/router.go"
	fmt.Printf("  -> Modifying %s (injecting Handler)...\n", filePath)

	return modifySourceFile(filePath, func(fset *token.FileSet, node *ast.File) error {
		handlerName := info.EntityName + "Handler"
		handlerType := "*handler." + info.EntityName + "Handler"
		paramName := toLowerCamel(handlerName)

		astutil.Apply(node, func(cursor *astutil.Cursor) bool {
			// --- 修改 Router 结构体 ---
			if ts, ok := cursor.Node().(*ast.TypeSpec); ok && ts.Name.Name == "Router" {
				if st, ok := ts.Type.(*ast.StructType); ok {
					// 检查字段是否已存在
					fieldExists := false
					for _, field := range st.Fields.List {
						if len(field.Names) > 0 && field.Names[0].Name == handlerName {
							fieldExists = true
							break
						}
					}
					// 如果不存在，则添加
					if !fieldExists {
						fmt.Printf("     + Adding field '%s' to Router struct\n", handlerName)
						fieldExpr, _ := parser.ParseExpr(handlerType)
						st.Fields.List = append(st.Fields.List, &ast.Field{
							Names: []*ast.Ident{ast.NewIdent(handlerName)},
							Type:  fieldExpr,
						})
					}
				}
			}

			// --- 修改 NewRouter 函数 ---
			if fd, ok := cursor.Node().(*ast.FuncDecl); ok && fd.Name.Name == "NewRouter" {
				// 检查参数是否已存在
				paramExists := false
				for _, param := range fd.Type.Params.List {
					if len(param.Names) > 0 && param.Names[0].Name == paramName {
						paramExists = true
						break
					}
				}
				// 如果不存在，则添加
				if !paramExists {
					fmt.Printf("     + Adding parameter '%s' to NewRouter function\n", paramName)
					paramExpr, _ := parser.ParseExpr(handlerType)
					fd.Type.Params.List = append(fd.Type.Params.List, &ast.Field{
						Names: []*ast.Ident{ast.NewIdent(paramName)},
						Type:  paramExpr,
					})
				}

				// 检查返回语句中的字段是否已存在
				for _, stmt := range fd.Body.List {
					if rs, ok := stmt.(*ast.ReturnStmt); ok && len(rs.Results) > 0 {
						if ue, ok := rs.Results[0].(*ast.UnaryExpr); ok {
							if cl, ok := ue.X.(*ast.CompositeLit); ok {
								returnFieldExists := false
								for _, elt := range cl.Elts {
									if kve, ok := elt.(*ast.KeyValueExpr); ok {
										if keyIdent, ok := kve.Key.(*ast.Ident); ok && keyIdent.Name == handlerName {
											returnFieldExists = true
											break
										}
									}
								}
								// 如果不存在，则添加
								if !returnFieldExists {
									fmt.Printf("     + Adding field '%s' to NewRouter return statement\n", handlerName)
									cl.Elts = append(cl.Elts, &ast.KeyValueExpr{
										Key:   ast.NewIdent(handlerName),
										Value: ast.NewIdent(paramName),
									})
								}
							}
						}
					}
				}
			}
			return true
		}, nil)

		// 确保 import 存在且不重复
		astutil.AddImport(fset, node, info.ProjectModule+"/internal/handler")
		return nil
	})
}

// addRoutesToRouter 使用字符串替换的方式添加路由，并且是幂等的
func addRoutesToRouter(info *EntityInfo) error {
	filePath := "internal/router/router.go"

	content, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	routeCheck := fmt.Sprintf("// %s routes", info.EntityName)
	if strings.Contains(string(content), routeCheck) {
		fmt.Printf("  -> Routes for %s already exist in %s, skipping.\n", info.EntityName, filePath)
		return nil
	}

	fmt.Printf("  -> Adding routes to %s...\n", filePath)

	anchor := "// [GENERATOR ANCHOR] - Don't remove this comment!"
	routeTemplate := `
	// {{.EntityName}} routes
	{{.LowerEntityName}}Routes := apiV1.Group("/{{.TableName}}")
	{{.LowerEntityName}}Routes.Post("/", r.{{.EntityName}}Handler.Create)
	{{.LowerEntityName}}Routes.Get("/", r.{{.EntityName}}Handler.GetAll)
	{{.LowerEntityName}}Routes.Get("/:id", r.{{.EntityName}}Handler.GetByID)
	{{.LowerEntityName}}Routes.Put("/:id", r.{{.EntityName}}Handler.Update)
	{{.LowerEntityName}}Routes.Delete("/:id", r.{{.EntityName}}Handler.Delete)

	` + anchor

	var tpl bytes.Buffer
	tmpl, err := template.New("routes").Parse(routeTemplate)
	if err != nil {
		return err
	}
	if err := tmpl.Execute(&tpl, info); err != nil {
		return err
	}

	newContent := strings.Replace(string(content), anchor, tpl.String(), 1)

	formatted, err := format.Source([]byte(newContent))
	if err != nil {
		fmt.Printf("     ⚠️ Code formatting failed: %v. Writing unformatted code.\n", err)
		return os.WriteFile(filePath, []byte(newContent), 0644)
	}

	return os.WriteFile(filePath, formatted, 0644)
}

// formatFile 运行 gofmt 来格式化文件
func formatFile(filePath string) error {
	cmd := exec.Command("gofmt", "-w", filePath)
	return cmd.Run()
}
