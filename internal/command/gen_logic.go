package command

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"
	"time"

	"github.com/AlecAivazis/survey/v2"

	"golang.org/x/tools/go/ast/astutil"

	"github.com/Skyenought/goprojectstarter/internal/common"
	"github.com/spf13/cobra"
)

//go:embed prompt-add-logic.tmpl
var addLogicPromptTemplate string

// AdditionalContext 存储附加实体的信息
type AdditionalContext struct {
	EntityName               string
	EntityPath               string
	EntityFileContent        string
	MapperPath               string
	MapperFileContent        string
	RepoInterfacePath        string
	RepoInterfaceFileContent string
}

// LogicAdditionInfo 存储所有上下文
type LogicAdditionInfo struct {
	ProjectModule            string
	EntityName               string
	MethodName               string
	UserPrompt               string
	HandlerFile              string
	ServiceFile              string
	RepoImplFile             string
	HandlerStructName        string
	ServiceImplStructName    string
	RepoImplStructName       string
	ExistingHandlerCode      string
	ExistingServiceCode      string
	ExistingRepoCode         string
	EntityPath               string
	EntityFileContent        string
	MapperPath               string
	MapperFileContent        string
	RepoInterfacePath        string
	RepoInterfaceFileContent string
	ExampleMethodName        string
	ExampleHandlerCode       string
	ExampleServiceCode       string
	ExampleRepoCode          string
	AdditionalContexts       []AdditionalContext
}

// ModifiedCodeSnippets 解析LLM的JSON响应
type ModifiedCodeSnippets struct {
	ModifiedHandlerMethod     string `json:"modified_handler_method"`
	ModifiedServiceImplMethod string `json:"modified_service_impl_method"`
	ModifiedRepoImplMethod    string `json:"modified_repo_impl_method"`
	NewRepoInterfaceMethod    string `json:"new_repo_interface_method"`
}

var genLogicCmd = &cobra.Command{
	Use:   "gen-logic",
	Short: "为已存在的接口交互式地添加业务逻辑",
	Long:  `此命令通过 LLM 辅助，为已选定的 Handler 方法添加新的业务逻辑。`,
	Run:   runGenLogic,
}

func init() {
	rootCmd.AddCommand(genLogicCmd)
	genLogicCmd.Flags().BoolVar(&historyMode, "history", false, "从历史记录中选择并重新执行一次 `gen-logic` 操作")
	genLogicCmd.Flags().StringVar(&fromMarkdownFile, "from-markdown", "", "从一个 markdown prompt 文件生成逻辑")
	genLogicCmd.Flags().BoolVar(&saveToMarkdown, "markdown", false, "将 AI prompt 保存到本地 markdown 文件用于调试或后续使用")
}

func runGenLogic(cmd *cobra.Command, args []string) {
	if !isGitClean() {
		fmt.Println("❌ 错误：你的 Git 工作区有未提交的更改。请先提交或储藏。")
		return
	}
	var info *LogicAdditionInfo
	var err error

	if fromMarkdownFile != "" {
		info, err = runLogicFromMarkdownMode(fromMarkdownFile)
	} else if historyMode {
		info, err = runLogicHistoryMode()
	} else {
		info, err = runInteractiveAddLogic()
	}
	if err != nil {
		fmt.Printf("❌ 操作已取消或失败: %v\n", err)
		return
	}
	if info == nil {
		if saveToMarkdown {
			return
		}
		fmt.Println("❌ 内部错误：未能获取操作信息。")
		return
	}

	finalPrompt, err := buildPromptFromInfo(info)
	if err != nil {
		fmt.Printf("❌ 构建 Prompt 失败: %v\n", err)
		return
	}

	if saveToMarkdown {
		filename := fmt.Sprintf("gen-logic-prompt-%s-%s.md", info.EntityName, info.MethodName)
		if err := os.WriteFile(filename, []byte(finalPrompt), 0o644); err != nil {
			fmt.Printf("⚠️ 警告: 保存 prompt 到 markdown 文件失败: %v\n", err)
		} else {
			fmt.Printf("✅ Prompt 已保存至 %s。程序将在此终止。\n", filename)
		}
		return
	}

	fmt.Println("\n🤖 正在请求 LLM 生成增强逻辑后的代码...")
	snippets, rawLLMResponse, err := generateModifiedCodeWithLLM(finalPrompt)
	if err != nil {
		fmt.Printf("❌ LLM 代码生成失败: %v\n", err)
		if rawLLMResponse != "" {
			saveDebugFile(rawLLMResponse)
		}
		return
	}
	fmt.Println("   ✓ LLM 代码生成成功！")

	if err := applyGeneratedCode(info, snippets); err != nil {
		fmt.Printf("❌ 代码注入失败: %v\n", err)
		saveDebugFile(rawLLMResponse)
		return
	}

	fmt.Println("\n✅ 正在格式化代码...")
	common.FormatImport()
	common.FormatFile()
	commitMessage := fmt.Sprintf("feat(gen-logic): enhance %s in %s handler", info.MethodName, info.EntityName)
	if err := gitCommit(commitMessage); err != nil {
		fmt.Printf("⚠️ 警告：代码已生成，但自动 Git 提交失败: %v\n", err)
	} else {
		fmt.Printf("✅ 已自动创建 Git 提交: \"%s\"\n", commitMessage)
	}
	fmt.Println("\n👉 请检查更新后的代码, 确保逻辑符合预期。")
}

// buildPromptFromInfo 负责构建最终的 LLM prompt 字符串
func buildPromptFromInfo(info *LogicAdditionInfo) (string, error) {
	tmpl, err := template.New("llm_prompt").Parse(addLogicPromptTemplate)
	if err != nil {
		return "", fmt.Errorf("无法解析LLM prompt模板: %w", err)
	}
	var promptBuf bytes.Buffer
	if err := tmpl.Execute(&promptBuf, info); err != nil {
		return "", fmt.Errorf("无法渲染LLM prompt模板: %w", err)
	}
	return promptBuf.String(), nil
}

// generateModifiedCodeWithLLM 现在返回原始响应字符串
func generateModifiedCodeWithLLM(prompt string) (*ModifiedCodeSnippets, string, error) {
	llmResponse, err := common.GenWithDefaultLLM(prompt)
	if err != nil {
		return nil, "", fmt.Errorf("LLM API调用失败: %w", err)
	}
	var snippets ModifiedCodeSnippets
	cleanedResponse := strings.TrimSpace(llmResponse)
	cleanedResponse = strings.TrimPrefix(cleanedResponse, "```json")
	cleanedResponse = strings.TrimSuffix(cleanedResponse, "```")
	if err := json.Unmarshal([]byte(cleanedResponse), &snippets); err != nil {
		return nil, llmResponse, fmt.Errorf("无法将LLM响应解析为JSON: %w", err)
	}
	return &snippets, llmResponse, nil
}

// saveDebugFile 将内容保存到带时间戳的文件中
func saveDebugFile(content string) {
	filename := fmt.Sprintf("llm_error_response_%s.txt", time.Now().Format("20060102_150405"))
	err := os.WriteFile(filename, []byte(content), 0o644)
	if err != nil {
		fmt.Printf("   ⚠️ 无法保存调试文件 %s: %v\n", filename, err)
	} else {
		fmt.Printf("   ℹ️ 原始 LLM 响应已保存至: %s\n", filename)
	}
}

func runInteractiveAddLogic() (*LogicAdditionInfo, error) {
	fmt.Println("🚀 欢迎使用业务逻辑增强向导！")
	paths, err := common.GetProjectPaths()
	if err != nil {
		return nil, err
	}
	allEntities, err := findEntities("internal/domain/entity")
	if err != nil || len(allEntities) == 0 {
		return nil, fmt.Errorf("在 'internal/domain/entity' 目录下找不到任何实体")
	}

	answers := struct {
		EntityName            string
		MethodName            string
		AdditionalEntityNames []string
		ExampleMethodName     string
		UserPrompt            string
	}{}

	entityPrompt := &survey.Select{Message: "请选择要操作的主要实体:", Options: allEntities}
	if err := survey.AskOne(entityPrompt, &answers.EntityName, survey.WithValidator(survey.Required)); err != nil {
		return nil, err
	}

	otherEntities := make([]string, 0, len(allEntities)-1)
	for _, e := range allEntities {
		if e != answers.EntityName {
			otherEntities = append(otherEntities, e)
		}
	}

	if len(otherEntities) > 0 {
		additionalPrompt := &survey.MultiSelect{
			Message: "是否需要其他实体的上下文信息？(按空格键选中)",
			Options: otherEntities,
		}
		if err := survey.AskOne(additionalPrompt, &answers.AdditionalEntityNames); err != nil {
			return nil, err
		}
	}

	handlerPath := filepath.Join(paths.HandlerDir, common.ToSnakeCase(answers.EntityName)+"_handler.go")
	methods, err := findPublicMethods(handlerPath)
	if err != nil || len(methods) == 0 {
		return nil, fmt.Errorf("在 %s 中找不到任何可用的方法", handlerPath)
	}

	methodPrompt := &survey.Select{Message: "请选择要增强逻辑的方法:", Options: methods}
	if err := survey.AskOne(methodPrompt, &answers.MethodName, survey.WithValidator(survey.Required)); err != nil {
		return nil, err
	}
	examplePrompt := &survey.Input{Message: "请输入一个参考方法名 (可选, 用于模仿其逻辑):"}
	if err := survey.AskOne(examplePrompt, &answers.ExampleMethodName); err != nil {
		return nil, err
	}
	promptEditor := &survey.Editor{
		Message:  "请详细描述要添加或修改的业务逻辑:",
		FileName: "logic_prompt*.txt",
		Help:     "例如：'在创建歌曲前，需要验证其关联的专辑和艺术家都存在。'",
	}
	if err := survey.AskOne(promptEditor, &answers.UserPrompt, survey.WithValidator(survey.Required)); err != nil {
		return nil, err
	}

	return buildLogicAdditionInfo(answers.EntityName, answers.MethodName, answers.UserPrompt, answers.ExampleMethodName, answers.AdditionalEntityNames)
}

func runLogicFromMarkdownMode(filePath string) (*LogicAdditionInfo, error) {
	fmt.Printf("🔍 正在从Markdown文件解析任务: %s\n", filePath)
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("读取markdown文件失败: %w", err)
	}
	entityName, methodName, prompt, exampleMethod, addEntities, err := parseLogicMarkdownPrompt(string(content))
	if err != nil {
		return nil, fmt.Errorf("解析markdown prompt失败: %w", err)
	}
	fmt.Println("   ✓ 解析成功！")
	return buildLogicAdditionInfo(entityName, methodName, prompt, exampleMethod, addEntities)
}

func runLogicHistoryMode() (*LogicAdditionInfo, error) {
	fmt.Println("🔍 正在查找 `gen-logic` 的历史记录...")
	commits, err := findGenLogicCommits(15)
	if err != nil || len(commits) == 0 {
		return nil, fmt.Errorf("未找到 `gen-logic` 的历史记录")
	}
	var selection string
	prompt := &survey.Select{Message: "请从历史记录中选择一个要重新执行的操作:", Options: commits}
	if err := survey.AskOne(prompt, &selection); err != nil {
		return nil, err
	}
	if selection == "" {
		return nil, fmt.Errorf("未选择任何操作")
	}
	commitMessage := strings.SplitN(selection, " - ", 2)[1]
	entityName, methodName, err := parseLogicCommitMessage(commitMessage)
	if err != nil {
		return nil, fmt.Errorf("解析历史提交信息失败: %w", err)
	}
	fmt.Printf("✅ 已恢复基本信息: 增强 %s 中的 %s\n", methodName, entityName)
	fmt.Println("📝 请重新提供本次操作的逻辑描述和上下文信息：")

	allEntities, _ := findEntities("internal/domain/entity")
	otherEntities := make([]string, 0)
	for _, e := range allEntities {
		if e != entityName {
			otherEntities = append(otherEntities, e)
		}
	}

	answers := struct {
		AdditionalEntityNames []string
		ExampleMethodName     string
		UserPrompt            string
	}{}

	if len(otherEntities) > 0 {
		additionalPrompt := &survey.MultiSelect{Message: "是否需要其他实体的上下文信息？", Options: otherEntities}
		if err := survey.AskOne(additionalPrompt, &answers.AdditionalEntityNames); err != nil {
			return nil, err
		}
	}
	examplePrompt := &survey.Input{Message: "请输入一个参考方法名 (可选):"}
	if err := survey.AskOne(examplePrompt, &answers.ExampleMethodName); err != nil {
		return nil, err
	}

	promptEditor := &survey.Editor{Message: "请详细描述要添加或修改的业务逻辑:"}
	if err := survey.AskOne(promptEditor, &answers.UserPrompt, survey.WithValidator(survey.Required)); err != nil {
		return nil, err
	}

	return buildLogicAdditionInfo(entityName, methodName, answers.UserPrompt, answers.ExampleMethodName, answers.AdditionalEntityNames)
}

func buildLogicAdditionInfo(entityName, methodName, userPrompt, exampleMethodName string, additionalEntityNames []string) (*LogicAdditionInfo, error) {
	paths, err := common.GetProjectPaths()
	if err != nil {
		return nil, err
	}
	module, err := getProjectModule()
	if err != nil {
		return nil, err
	}
	snakeName := common.ToSnakeCase(entityName)
	info := &LogicAdditionInfo{
		ProjectModule:         module,
		EntityName:            entityName,
		MethodName:            methodName,
		UserPrompt:            userPrompt,
		ExampleMethodName:     exampleMethodName,
		HandlerFile:           filepath.Join(paths.HandlerDir, snakeName+"_handler.go"),
		ServiceFile:           filepath.Join(paths.ServiceDir, snakeName+"_service.go"),
		RepoImplFile:          filepath.Join(paths.RepoImplDir, snakeName+"_repository_impl.go"),
		RepoInterfacePath:     filepath.Join(paths.RepoInterfaceDir, snakeName+"_repository.go"),
		HandlerStructName:     entityName + "Handler",
		ServiceImplStructName: common.ToLowerCamel(entityName) + "ServiceImpl",
		RepoImplStructName:    common.ToLowerCamel(entityName) + "RepositoryImpl",
	}

	fmt.Println("   - 正在提取主要实体上下文...")
	info.ExistingHandlerCode, _ = findMethodContent(info.HandlerFile, info.HandlerStructName, info.MethodName)
	info.ExistingServiceCode, _ = findMethodContent(info.ServiceFile, info.ServiceImplStructName, info.MethodName)
	info.ExistingRepoCode, _ = findMethodContent(info.RepoImplFile, info.RepoImplStructName, info.MethodName)
	info.EntityFileContent, info.EntityPath, _ = findEntityContent(entityName)
	info.MapperFileContent, info.MapperPath, _ = findMapperContent(entityName)
	repoInterfaceContent, _ := os.ReadFile(info.RepoInterfacePath)
	info.RepoInterfaceFileContent = string(repoInterfaceContent)

	if info.ExampleMethodName != "" {
		fmt.Printf("   - 正在提取参考方法 '%s' 的代码...\n", info.ExampleMethodName)
		info.ExampleHandlerCode, _ = findMethodContent(info.HandlerFile, info.HandlerStructName, info.ExampleMethodName)
		info.ExampleServiceCode, _ = findMethodContent(info.ServiceFile, info.ServiceImplStructName, info.ExampleMethodName)
		info.ExampleRepoCode, _ = findMethodContent(info.RepoImplFile, info.RepoImplStructName, info.ExampleMethodName)
	}

	if len(additionalEntityNames) > 0 {
		fmt.Println("   - 正在提取附加实体上下文...")
		for _, addEntityName := range additionalEntityNames {
			fmt.Printf("     - %s...\n", addEntityName)
			addSnakeName := common.ToSnakeCase(addEntityName)
			addCtx := AdditionalContext{EntityName: addEntityName}
			addCtx.EntityFileContent, addCtx.EntityPath, _ = findEntityContent(addEntityName)
			addCtx.MapperFileContent, addCtx.MapperPath, _ = findMapperContent(addEntityName)
			addCtx.RepoInterfacePath = filepath.Join(paths.RepoInterfaceDir, addSnakeName+"_repository.go")
			addRepoInterfaceContent, _ := os.ReadFile(addCtx.RepoInterfacePath)
			addCtx.RepoInterfaceFileContent = string(addRepoInterfaceContent)
			info.AdditionalContexts = append(info.AdditionalContexts, addCtx)
		}
	}
	fmt.Println("   ✓ 上下文提取完成。")
	return info, nil
}

func applyGeneratedCode(info *LogicAdditionInfo, snippets *ModifiedCodeSnippets) error {
	tasks := []struct {
		filePath   string
		newCode    string
		structName string
		methodName string // The primary method being targeted
	}{
		{info.HandlerFile, snippets.ModifiedHandlerMethod, info.HandlerStructName, info.MethodName},
		{info.ServiceFile, snippets.ModifiedServiceImplMethod, info.ServiceImplStructName, info.MethodName},
		{info.RepoImplFile, snippets.ModifiedRepoImplMethod, info.RepoImplStructName, info.MethodName},
	}

	for _, task := range tasks {
		if task.newCode != "" {
			fmt.Printf("  -> 正在智能更新 %s...\n", task.filePath)
			// Pass the target method name to the smart replacement function
			if err := smartReplaceOrAddMethods(task.filePath, task.newCode, task.structName); err != nil {
				return err
			}
		}
	}

	if snippets.NewRepoInterfaceMethod != "" {
		fmt.Printf("  -> 正在向接口 %s 添加新方法...\n", info.RepoInterfacePath)
		anchor := fmt.Sprintf("type %sRepository interface", info.EntityName)
		err := appendToFile(info.RepoInterfacePath, "\n\t"+snippets.NewRepoInterfaceMethod, common.ApiInfo{EntityName: info.EntityName}, anchor, common.InsertAfterBrace)
		if err != nil {
			return fmt.Errorf("向仓库接口添加方法失败: %w", err)
		}
	}
	return nil
}

// hasSwaggerAnnotations 检查注释组是否包含 Swagger/Swag 的注解。
func hasSwaggerAnnotations(doc *ast.CommentGroup) bool {
	if doc == nil {
		return false
	}
	for _, comment := range doc.List {
		// 检查注释行是否包含 '@' 符号，这是 Swagger 注解的典型特征
		if strings.Contains(comment.Text, "@") {
			return true
		}
	}
	return false
}

// smartReplaceOrAddMethods 使用基于 AST 的智能合并策略来更新或添加方法。
func smartReplaceOrAddMethods(filePath, codeSnippet, targetStructName string) error {
	if strings.TrimSpace(codeSnippet) == "" {
		return nil
	}

	fsetSnippet := token.NewFileSet()
	snippetFile, err := parser.ParseFile(fsetSnippet, "", "package temp\n"+codeSnippet, parser.ParseComments)
	if err != nil || len(snippetFile.Decls) == 0 {
		return fmt.Errorf("无法解析LLM生成的代码片段: %w。代码:\n%s", err, codeSnippet)
	}
	newMethod, ok := snippetFile.Decls[0].(*ast.FuncDecl)
	if !ok {
		return fmt.Errorf("LLM响应中未找到有效的函数声明")
	}
	methodName := newMethod.Name.Name

	fsetTarget := token.NewFileSet()
	var originalContent []byte
	var fileExists bool
	if _, statErr := os.Stat(filePath); statErr == nil {
		originalContent, _ = os.ReadFile(filePath)
		fileExists = true
	}

	var targetNode *ast.File
	if fileExists {
		targetNode, err = parser.ParseFile(fsetTarget, filePath, originalContent, parser.ParseComments)
		if err != nil {
			return fmt.Errorf("无法解析目标文件 %s: %w", filePath, err)
		}
	} else {
		pkgName := filepath.Base(filepath.Dir(filePath))
		targetNode = &ast.File{Name: ast.NewIdent(pkgName)}
	}

	var oldMethod *ast.FuncDecl
	astutil.Apply(targetNode, func(cursor *astutil.Cursor) bool {
		if fd, ok := cursor.Node().(*ast.FuncDecl); ok && fd.Name.Name == methodName {
			if strings.EqualFold(getReceiverTypeName(fd.Recv), targetStructName) {
				oldMethod = fd
				return false
			}
		}
		return true
	}, nil)

	if oldMethod != nil {
		fmt.Printf("     - 找到现有方法 '%s', 正在智能合并...\n", methodName)
		finalDoc := newMethod.Doc
		if hasSwaggerAnnotations(oldMethod.Doc) && !hasSwaggerAnnotations(newMethod.Doc) {
			fmt.Println("       -> 检测到并保留了现有的 Swagger 注释。")
			finalDoc = oldMethod.Doc
		}
		oldMethod.Doc = finalDoc
		oldMethod.Body = newMethod.Body // 只替换函数体
	} else {
		fmt.Printf("     - 未找到方法 '%s', 将其作为新方法添加。\n", methodName)
		targetNode.Decls = append(targetNode.Decls, newMethod)
	}

	var buf bytes.Buffer
	if err := format.Node(&buf, fsetTarget, targetNode); err != nil {
		return fmt.Errorf("格式化 AST 到 buffer 失败: %w", err)
	}

	formattedContent, err := format.Source(buf.Bytes())
	if err != nil {
		fmt.Printf("   ⚠️ 警告: format.Source 最终格式化失败: %v。将写入未经 import 整理的代码。\n", err)
		return os.WriteFile(filePath, buf.Bytes(), 0o644)
	}

	// 6. 将最终的、完全格式化好的代码写回文件
	return os.WriteFile(filePath, formattedContent, 0o644)
}

func getReceiverTypeName(recv *ast.FieldList) string {
	if recv == nil || len(recv.List) == 0 {
		return ""
	}
	expr := recv.List[0].Type
	if starExpr, ok := expr.(*ast.StarExpr); ok {
		if ident, ok := starExpr.X.(*ast.Ident); ok {
			return ident.Name
		}
	} else if ident, ok := expr.(*ast.Ident); ok {
		return ident.Name
	}
	return ""
}

func findMethodContent(filePath, receiverTypeName, methodName string) (string, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filePath, string(content), 0)
	if err != nil {
		return "", err
	}
	var methodNode *ast.FuncDecl
	ast.Inspect(node, func(n ast.Node) bool {
		if fd, ok := n.(*ast.FuncDecl); ok && fd.Name.Name == methodName {
			currentReceiverTypeName := getReceiverTypeName(fd.Recv)
			if strings.EqualFold(strings.TrimPrefix(currentReceiverTypeName, "*"), strings.TrimPrefix(receiverTypeName, "*")) {
				methodNode = fd
				return false
			}
		}
		return true
	})
	if methodNode == nil {
		return "", nil
	}
	startOffset := fset.Position(methodNode.Pos()).Offset
	endOffset := fset.Position(methodNode.End()).Offset
	return string(content[startOffset:endOffset]), nil
}

func findPublicMethods(filePath string) ([]string, error) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filePath, nil, 0)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("无法解析文件 %s: %w", filePath, err)
	}
	var methods []string
	ast.Inspect(node, func(n ast.Node) bool {
		if fd, ok := n.(*ast.FuncDecl); ok {
			if fd.Recv != nil && len(fd.Recv.List) > 0 && fd.Name.IsExported() {
				methods = append(methods, fd.Name.Name)
			}
		}
		return true
	})
	return methods, nil
}

func parseLogicMarkdownPrompt(content string) (entityName, methodName, userPrompt, exampleMethodName string, additionalEntities []string, err error) {
	reEntity := regexp.MustCompile(`- \*\*主要实体\*\*: (\w+)`)
	reMethod := regexp.MustCompile(`- \*\*目标方法\*\*: (\w+)`)
	reExample := regexp.MustCompile(`\*\*参考示例代码 .* '(\w+)'`)
	reAdditional := regexp.MustCompile(`### 附加实体: (\w+)`)
	entityMatch := reEntity.FindStringSubmatch(content)
	if len(entityMatch) < 2 {
		err = fmt.Errorf("在markdown中未找到主要实体")
		return
	}
	entityName = entityMatch[1]
	methodMatch := reMethod.FindStringSubmatch(content)
	if len(methodMatch) < 2 {
		err = fmt.Errorf("在markdown中未找到目标方法")
		return
	}
	methodName = methodMatch[1]
	if exampleMatch := reExample.FindStringSubmatch(content); len(exampleMatch) > 1 {
		exampleMethodName = exampleMatch[1]
	}
	if additionalMatches := reAdditional.FindAllStringSubmatch(content, -1); len(additionalMatches) > 0 {
		for _, match := range additionalMatches {
			additionalEntities = append(additionalEntities, match[1])
		}
	}
	promptStartMarker := "## 用户的目标 (USER'S GOAL)"
	promptEndMarker := "## 核心定义文件 (DEFINITIONS FOR"
	startIndex := strings.Index(content, promptStartMarker)
	if startIndex == -1 {
		err = fmt.Errorf("在markdown中未找到 '%s'", promptStartMarker)
		return
	}
	contentAfterStart := content[startIndex+len(promptStartMarker):]
	endIndex := strings.Index(contentAfterStart, promptEndMarker)
	if endIndex == -1 {
		err = fmt.Errorf("在markdown中未找到 '%s'", promptEndMarker)
		return
	}
	userPrompt = strings.TrimSpace(contentAfterStart[:endIndex])
	if userPrompt == "" {
		err = fmt.Errorf("用户目标不能为空")
	}
	return
}

func findGenLogicCommits(limit int) ([]string, error) {
	cmd := exec.Command("git", "log", fmt.Sprintf("-%d", limit), "--grep=^feat(gen-logic):", "--pretty=format:%h - %s")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	if len(output) == 0 {
		return []string{}, nil
	}
	return strings.Split(strings.TrimSpace(string(output)), "\n"), nil
}

func parseLogicCommitMessage(message string) (entityName, methodName string, err error) {
	re := regexp.MustCompile(`^feat\(gen-logic\): enhance (\w+) in (\w+) handler$`)
	matches := re.FindStringSubmatch(message)
	if len(matches) != 3 {
		err = fmt.Errorf("无法解析提交信息格式: %s", message)
		return
	}
	methodName = matches[1]
	entityName = matches[2]
	return
}
