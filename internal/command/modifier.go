package command

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"text/template"

	"golang.org/x/tools/go/ast/astutil"
)

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

	return os.WriteFile(filePath, buf.Bytes(), 0o644)
}

func addProviderToDI(info *EntityInfo, paths PathConfig) error {
	filePath := paths.DIFile

	content, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	providerCheck := fmt.Sprintf("// %s Providers", info.EntityName)
	if strings.Contains(string(content), providerCheck) {
		fmt.Printf("  -> Providers for %s already exist in %s, skipping provider addition.\n", info.EntityName, filePath)
		return ensureImportsForDI(info, paths)
	}

	fmt.Printf("  -> Modifying %s (adding providers)...\n", filePath)

	anchor := "// [GENERATOR ANCHOR] - Don't remove this comment!"
	// 根据模式选择不同的 Provider 模板
	providerTemplateStr := `
		// {{.EntityName}} Providers
		repository.New{{.EntityName}}Repository,
		service.New{{.EntityName}}Service,
		handler.New{{.EntityName}}Handler,
		` + anchor

	if paths.IsDDD {
		providerTemplateStr = `
		// {{.EntityName}} Providers
		persistence.New{{.EntityName}}Repository,
		service.New{{.EntityName}}Service,
		handler.New{{.EntityName}}Handler,
		` + anchor
	}

	var tpl bytes.Buffer
	tmpl, err := template.New("providers").Parse(providerTemplateStr)
	if err != nil {
		return err
	}
	if err := tmpl.Execute(&tpl, info); err != nil {
		return err
	}

	newContent := strings.Replace(string(content), anchor, tpl.String(), 1)

	if err := os.WriteFile(filePath, []byte(newContent), 0o644); err != nil {
		return err
	}

	return ensureImportsForDI(info, paths)
}

func ensureImportsForDI(info *EntityInfo, paths PathConfig) error {
	return modifySourceFile(paths.DIFile, func(fset *token.FileSet, node *ast.File) error {
		for _, pkgPath := range paths.DIImports {
			astutil.AddImport(fset, node, info.ProjectModule+pkgPath)
		}
		return nil
	})
}

func addHandlerToRouter(info *EntityInfo, paths PathConfig) error {
	filePath := paths.RouterFile
	fmt.Printf("  -> Modifying %s (injecting Handler)...\n", filePath)

	return modifySourceFile(filePath, func(fset *token.FileSet, node *ast.File) error {
		handlerName := info.EntityName + "Handler"
		handlerType := "*handler." + info.EntityName + "Handler"
		paramName := toLowerCamel(handlerName)

		astutil.Apply(node, func(cursor *astutil.Cursor) bool {
			if ts, ok := cursor.Node().(*ast.TypeSpec); ok && ts.Name.Name == "Router" {
				if st, ok := ts.Type.(*ast.StructType); ok {
					fieldExists := false
					for _, field := range st.Fields.List {
						if len(field.Names) > 0 && field.Names[0].Name == handlerName {
							fieldExists = true
							break
						}
					}
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

			if fd, ok := cursor.Node().(*ast.FuncDecl); ok && fd.Name.Name == "NewRouter" {
				paramExists := false
				for _, param := range fd.Type.Params.List {
					if len(param.Names) > 0 && param.Names[0].Name == paramName {
						paramExists = true
						break
					}
				}
				if !paramExists {
					fmt.Printf("     + Adding parameter '%s' to NewRouter function\n", paramName)
					paramExpr, _ := parser.ParseExpr(handlerType)
					fd.Type.Params.List = append(fd.Type.Params.List, &ast.Field{
						Names: []*ast.Ident{ast.NewIdent(paramName)},
						Type:  paramExpr,
					})
				}

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

		astutil.AddImport(fset, node, info.ProjectModule+paths.HandlerPackagePath)
		return nil
	})
}

func addRoutesToRouter(info *EntityInfo, paths PathConfig) error {
	filePath := paths.RouterFile

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
		fmt.Printf("    ⚠️ Code formatting failed: %v. Writing unformatted code.\n", err)
		return os.WriteFile(filePath, []byte(newContent), 0o644)
	}

	return os.WriteFile(filePath, formatted, 0o644)
}
