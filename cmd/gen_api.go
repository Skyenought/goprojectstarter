package cmd

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"github.com/Skyenought/goprojectstarter/pkg/llm/deepseek"
	"github.com/Skyenought/goprojectstarter/pkg/llm/gemini"
	"github.com/Skyenought/goprojectstarter/pkg/llm/volc"
	"go/ast"
	"go/parser"
	"go/token"
	"gopkg.in/yaml.v3"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/Skyenought/goprojectstarter/pkg/common"
	"github.com/Skyenought/goprojectstarter/pkg/llm"
	"github.com/spf13/cobra"
)

var (
	//go:embed prompt.tmpl
	promptTemplate  string
	interactiveMode bool
	historyMode     bool
	httpMethod      string
	apiPath         string
	userPrompt      string
	saveToMarkdown  bool
)

// LLMCodeSnippets 定义了从 LLM 期望获得的 JSON 结构
type LLMCodeSnippets struct {
	RepoInterfaceMethod string `json:"repo_interface_method"`
	RepoImplMethod      string `json:"repo_impl_method"`
	ServiceInterface    string `json:"service_interface_method"`
	ServiceImplMethod   string `json:"service_impl_method"`
	HandlerMethod       string `json:"handler_method"`
	RouterLine          string `json:"router_line"`
	MapperFullContent   string `json:"mapper_full_content"` // AI可以修改并返回完整的Mapper文件
}

var genApiCmd = &cobra.Command{
	Use:   "gen-api [EntityName] [MethodName]",
	Short: "为已存在的实体创建新的 API 接口",
	Long: `此命令可以为实体自动在 Handler, Service, Repository 层追加新的方法框架, 并注册路由。
它利用 LLM 根据你的自然语言描述来生成更智能、更贴合需求的代码。

支持三种模式:
1. 直接模式: goprojectstarter gen-api User PromoteUser --method POST --path /:id/promote -p "提升用户等级，需要一个 'level' 字符串在请求体中"
2. 交互模式: goprojectstarter gen-api -i (或不带参数)
3. 历史模式: goprojectstarter gen-api --history`,
	Run: runGenApi,
}

var genApiRevertCmd = &cobra.Command{
	Use:   "gen-api:revert",
	Short: "撤销一次由 `gen-api` 生成的操作",
	Long:  `此命令会列出最近由 'gen-api' 自动创建的 Git 提交，并允许你选择一个进行撤销(revert)。`,
	Run:   runGenApiRevert,
}

func init() {
	rootCmd.AddCommand(genApiCmd)
	rootCmd.AddCommand(genApiRevertCmd)

	genApiCmd.Flags().BoolVarP(&interactiveMode, "interactive", "i", false, "启用交互式向导来创建新接口")
	genApiCmd.Flags().BoolVar(&historyMode, "history", false, "从历史记录中选择并重新执行一次 `gen-api` 操作")
	genApiCmd.Flags().StringVar(&httpMethod, "method", "POST", "指定 HTTP 方法 (e.g., GET, POST)")
	genApiCmd.Flags().StringVar(&apiPath, "path", "", "指定 API 路径 (e.g., /:id/promote)")
	genApiCmd.Flags().StringVarP(&userPrompt, "prompt", "p", "", "用自然语言描述新 API 的功能、参数和业务流程")
	genApiCmd.Flags().BoolVar(&saveToMarkdown, "markdown", false, "保存生成的 AI prompt 到本地 markdown 文件用于调试, 不实际调用 LLM")
}

func runGenApi(cmd *cobra.Command, args []string) {
	if !isGitClean() {
		fmt.Println("❌ 错误：你的 Git 工作区有未提交的更改。")
		fmt.Println("请先提交或储藏你的更改。")
		return
	}

	var info common.ApiInfo
	var err error

	if historyMode {
		info, err = runHistoryMode()
	} else if interactiveMode || len(args) == 0 {
		info, err = runInteractiveMode()
	} else {
		info, err = runDirectMode(args)
		if userPrompt == "" && !saveToMarkdown {
			fmt.Println("❌ 错误：在直接模式下必须使用 `-p` 或 `--prompt` 标志提供功能描述。")
			return
		}
	}

	if err != nil {
		fmt.Printf("❌ 操作已取消: %v\n", err)
		return
	}

	fmt.Println("\n🤖 正在请求 LLM 生成代码骨架...")
	snippets, err := generateCodeWithLLM(info, userPrompt)
	if err != nil {
		fmt.Printf("❌ LLM 代码生成失败: %v\n", err)
		return
	}

	if saveToMarkdown {
		return
	}
	fmt.Println("   ✓ LLM 代码生成成功！")

	if err := injectGeneratedCode(info, snippets); err != nil {
		fmt.Printf("❌ 代码注入失败: %v\n", err)
		return
	}
	fmt.Println("\n✅ 基础代码骨架已注入！")

	fmt.Println("\n✅ 操作成功！正在格式化代码...")
	common.FormatImport()
	common.FormatFile()

	commitMessage := fmt.Sprintf("feat(gen-api): add %s to %s", info.MethodName, info.EntityName)
	if err := gitCommit(commitMessage); err != nil {
		fmt.Printf("⚠️ 警告：代码已生成，但自动 Git 提交失败: %v\n", err)
	} else {
		fmt.Printf("✅ 已自动创建 Git 提交: \"%s\"\n", commitMessage)
	}

	fmt.Println("\n👉 请检查新生成的代码, 并根据需要微调业务逻辑。")
}

func runInteractiveMode() (common.ApiInfo, error) {
	fmt.Println("🚀 欢迎使用 API 接口生成向导！")
	answers := struct {
		EntityName string
		MethodName string
		HttpVerb   string
		ApiPath    string
		UserPrompt string
	}{}

	entities, err := findEntities("internal/domain/entity")
	if err != nil || len(entities) == 0 {
		return common.ApiInfo{}, fmt.Errorf("在 'internal/domain/entity' 目录下找不到任何实体")
	}

	questions := []*survey.Question{
		{Name: "EntityName", Prompt: &survey.Select{Message: "请选择您要操作的实体:", Options: entities}, Validate: survey.Required},
		{Name: "MethodName", Prompt: &survey.Input{Message: "请输入新的方法名 (例如: PromoteUser):"}, Validate: survey.Required},
		{Name: "HttpVerb", Prompt: &survey.Select{Message: "请选择 HTTP 方法:", Options: []string{"POST", "GET", "PUT", "DELETE", "PATCH"}, Default: "POST"}},
		{Name: "ApiPath", Prompt: &survey.Input{Message: "请输入 API 路径 (例如: /:id/promote):"}, Validate: survey.Required},
		{Name: "UserPrompt", Prompt: &survey.Editor{
			Message:  "请详细描述新 API 的功能、参数 (来源、名称、类型) 和业务流程:",
			FileName: "api_prompt*.txt",
			Help:     "描述越清晰，LLM 生成的代码就越准确。请说明参数来源(路径path,查询query,请求体body), 名称和类型。",
			Default:  "例如: 这是一个提升用户等级的接口。需要从请求体(JSON body)中获取一个名为 'newLevel' 的字符串参数。成功后返回更新后的用户信息。",
		}, Validate: survey.Required},
	}

	if err := survey.Ask(questions, &answers); err != nil {
		return common.ApiInfo{}, err
	}
	userPrompt = answers.UserPrompt
	return buildApiInfo(answers.EntityName, answers.MethodName, answers.HttpVerb, answers.ApiPath)
}

func runDirectMode(args []string) (common.ApiInfo, error) {
	if len(args) < 2 {
		return common.ApiInfo{}, fmt.Errorf("缺少必要的参数。请提供 EntityName 和 MethodName")
	}
	if apiPath == "" {
		return common.ApiInfo{}, fmt.Errorf("必须使用 --path 标志提供 API 路径")
	}
	return buildApiInfo(args[0], args[1], httpMethod, apiPath)
}

func runHistoryMode() (common.ApiInfo, error) {
	fmt.Println("🔍 正在查找 `gen-api` 的历史记录...")
	commits, err := findGenApiCommits(15)
	if err != nil || len(commits) == 0 {
		return common.ApiInfo{}, fmt.Errorf("未找到 `gen-api` 的历史记录")
	}

	var selection string
	prompt := &survey.Select{
		Message: "请从历史记录中选择一个要重新执行的操作:",
		Options: commits,
	}
	if err := survey.AskOne(prompt, &selection); err != nil {
		return common.ApiInfo{}, err
	}
	if selection == "" {
		return common.ApiInfo{}, fmt.Errorf("未选择任何操作")
	}

	commitMessage := strings.SplitN(selection, " - ", 2)[1]
	info, err := parseCommitMessage(commitMessage)
	if err != nil {
		return common.ApiInfo{}, fmt.Errorf("解析历史提交信息失败: %w", err)
	}

	fmt.Printf("✅ 已恢复基本信息: %s on %s (%s %s)\n", info.MethodName, info.EntityName, info.HttpVerb, info.FullApiPath)
	fmt.Println("📝 由于无法从 Git 历史中恢复原始的功能描述，请为这次操作重新提供：")

	promptEditor := &survey.Editor{
		Message:  "请为这个历史操作提供详细的功能描述:",
		FileName: "api_prompt*.txt",
		Help:     "即使是历史操作，也需要提供清晰的描述，以便 LLM 生成正确的代码。",
	}
	if err := survey.AskOne(promptEditor, &userPrompt, survey.WithValidator(survey.Required)); err != nil {
		return common.ApiInfo{}, err
	}
	return info, nil
}

func generateCodeWithLLM(info common.ApiInfo, userPrompt string) (*LLMCodeSnippets, error) {
	entityContent, entityPath, err := findEntityContent(info.EntityName)
	if err != nil {
		return nil, fmt.Errorf("无法找到并读取实体 '%s' 的文件: %w", info.EntityName, err)
	}

	mapperContent, mapperPath, err := findMapperContent(info.EntityName)
	if err != nil {
		fmt.Printf("   - 提示: 未找到 Mapper 文件 at %s, 将视为空文件处理，AI 会尝试创建它。\n", mapperPath)
		mapperContent = ""
	}

	tmpl, err := template.New("llm_prompt").Parse(promptTemplate)
	if err != nil {
		return nil, fmt.Errorf("解析 LLM prompt 模板失败: %w", err)
	}

	templateData := map[string]interface{}{
		"EntityName": info.EntityName, "LowerEntityName": info.LowerEntityName,
		"MethodName": info.MethodName, "HttpVerb": info.HttpVerb,
		"FullApiPath": info.FullApiPath, "UserPrompt": userPrompt,
		"EntityContent": entityContent, "EntityPath": entityPath,
		"MapperContent": mapperContent, "MapperPath": mapperPath,
	}

	var promptBuf bytes.Buffer
	if err := tmpl.Execute(&promptBuf, templateData); err != nil {
		return nil, fmt.Errorf("渲染 LLM prompt 模板失败: %w", err)
	}
	finalPrompt := promptBuf.String()

	if saveToMarkdown {
		filename := fmt.Sprintf("gen-api-prompt-%s-%s.md", info.EntityName, info.MethodName)
		if err := os.WriteFile(filename, []byte(finalPrompt), 0644); err != nil {
			fmt.Printf("⚠️ 警告：保存 prompt 到 markdown 文件失败: %v\n", err)
		} else {
			fmt.Printf("✅ Prompt 已保存至 %s。程序将在此终止。\n", filename)
		}
		return nil, nil
	}

	llmResponse, err := GenerateWithDefaultLLM(finalPrompt)
	if err != nil {
		return nil, fmt.Errorf("LLM API 调用失败: %w", err)
	}

	var snippets LLMCodeSnippets
	cleanedResponse := strings.TrimSpace(llmResponse)
	cleanedResponse = strings.TrimPrefix(cleanedResponse, "```json")
	cleanedResponse = strings.TrimSuffix(cleanedResponse, "```")
	if err := json.Unmarshal([]byte(cleanedResponse), &snippets); err != nil {
		return nil, fmt.Errorf("无法将 LLM 的响应解析为 JSON。原始响应:\n%s\n错误详情: %w", llmResponse, err)
	}

	return &snippets, nil
}

func injectGeneratedCode(info common.ApiInfo, snippets *LLMCodeSnippets) error {
	paths, err := common.GetProjectPaths()
	if err != nil {
		return err
	}

	// 步骤1: 处理 Mapper 文件的覆盖
	if snippets.MapperFullContent != "" {
		mapperDir := "internal/interfaces/dto" // 假设 DDD 结构
		mapperPath := filepath.Join(mapperDir, common.ToSnakeCase(info.EntityName)+"_mapper.go")
		fmt.Printf("  -> 正在覆盖/创建 Mapper 文件 %s...\n", mapperPath)
		if err := os.MkdirAll(filepath.Dir(mapperPath), 0755); err != nil {
			return fmt.Errorf("创建 Mapper 目录 %s 失败: %w", filepath.Dir(mapperPath), err)
		}
		if err := os.WriteFile(mapperPath, []byte(snippets.MapperFullContent), 0644); err != nil {
			return fmt.Errorf("写入 Mapper 文件 %s 失败: %w", mapperPath, err)
		}
	}

	// 步骤2: 处理其他文件的代码追加
	if err := ensureRouteGroupExists(paths.RouterFile, info); err != nil {
		return fmt.Errorf("确保路由组存在失败: %w", err)
	}

	tasks := []struct {
		filePathTmpl string
		codeSnippet  string
		anchor       string
		mode         common.InsertionMode
	}{
		{filePathTmpl: paths.RepoInterfaceDir + "/%s_repository.go", codeSnippet: "\n\t" + snippets.RepoInterfaceMethod, anchor: "type {{.EntityName}}Repository interface", mode: common.InsertAfterBrace},
		{filePathTmpl: paths.RepoImplDir + "/%s_repository_impl.go", codeSnippet: "\n" + snippets.RepoImplMethod, anchor: "", mode: common.AppendToEnd},
		{filePathTmpl: paths.ServiceDir + "/%s_service.go", codeSnippet: "\n\t" + snippets.ServiceInterface, anchor: "type {{.EntityName}}Service interface", mode: common.InsertAfterBrace},
		{filePathTmpl: paths.ServiceDir + "/%s_service.go", codeSnippet: "\n" + snippets.ServiceImplMethod, anchor: "", mode: common.AppendToEnd},
		{filePathTmpl: paths.HandlerDir + "/%s_handler.go", codeSnippet: "\n" + snippets.HandlerMethod, anchor: "", mode: common.AppendToEnd},
		{filePathTmpl: paths.RouterFile, codeSnippet: "\n\t" + snippets.RouterLine, anchor: fmt.Sprintf(`%sRoutes := apiV1.Group("/%s")`, info.LowerEntityName, info.TableName), mode: common.InsertAfterLine},
	}

	for _, task := range tasks {
		var filePath string
		if strings.Contains(task.filePathTmpl, "%s") {
			filePath = fmt.Sprintf(task.filePathTmpl, common.ToSnakeCase(info.EntityName))
		} else {
			filePath = task.filePathTmpl
		}
		fmt.Printf("  -> 正在修改 %s...\n", filePath)
		if err := appendToFile(filePath, task.codeSnippet, info, task.anchor, task.mode); err != nil {
			return fmt.Errorf("修改文件 %s 失败: %w", filePath, err)
		}
	}
	return nil
}

func ensureRouteGroupExists(routerPath string, info common.ApiInfo) error {
	content, err := os.ReadFile(routerPath)
	if err != nil {
		return err
	}
	groupDefinition := fmt.Sprintf(`%sRoutes := apiV1.Group("/%s")`, info.LowerEntityName, info.TableName)
	if bytes.Contains(content, []byte(groupDefinition)) {
		return nil
	}
	fmt.Printf("  -> 在 %s 中未找到路由组，正在创建...\n", routerPath)
	creationCode := fmt.Sprintf("\n\t// %s routes\n\t%s", info.EntityName, groupDefinition)
	anchor := `apiV1 := r.App.Group("/api/v1")`
	return appendToFile(routerPath, creationCode, info, anchor, common.InsertAfterLine)
}

func appendToFile(filePath, codeSnippet string, info common.ApiInfo, anchorTmplStr string, mode common.InsertionMode) error {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}
	var newContent []byte
	switch mode {
	case common.AppendToEnd:
		newContent = append(content, append([]byte("\n"), []byte(codeSnippet)...)...)
	case common.InsertAfterLine, common.InsertAfterBrace:
		if anchorTmplStr == "" {
			return fmt.Errorf("模式 %v 需要一个非空的锚点", mode)
		}
		anchorTmpl, err := template.New("anchor").Parse(anchorTmplStr)
		if err != nil {
			return err
		}
		var anchorBuf bytes.Buffer
		if err := anchorTmpl.Execute(&anchorBuf, info); err != nil {
			return err
		}
		renderedAnchor := anchorBuf.Bytes()
		anchorPos := bytes.Index(content, renderedAnchor)
		if anchorPos == -1 {
			return fmt.Errorf("在文件 %s 中未找到锚点: `%s`", filePath, string(renderedAnchor))
		}
		var insertionPoint int
		if mode == common.InsertAfterBrace {
			sliceAfterAnchor := content[anchorPos:]
			bracePos := bytes.Index(sliceAfterAnchor, []byte("{"))
			if bracePos == -1 {
				return fmt.Errorf("在锚点 `%s` 之后未找到 '{'", string(renderedAnchor))
			}
			insertionPoint = anchorPos + bracePos + 1
		} else {
			insertionPoint = anchorPos + len(renderedAnchor)
		}
		var finalContent bytes.Buffer
		finalContent.Write(content[:insertionPoint])
		if mode == common.InsertAfterLine {
			finalContent.WriteString("\n")
		}
		finalContent.WriteString(codeSnippet)
		finalContent.Write(content[insertionPoint:])
		newContent = finalContent.Bytes()
	}
	return os.WriteFile(filePath, newContent, 0o644)
}

func runGenApiRevert(cmd *cobra.Command, args []string) {
	commits, err := findGenApiCommits(10)
	if err != nil {
		fmt.Printf("❌ 查找 gen-api 提交记录失败: %v\n", err)
		return
	}
	if len(commits) == 0 {
		fmt.Println("ℹ️ 未找到最近由 `gen-api` 创建的提交记录。")
		return
	}
	var selection string
	prompt := &survey.Select{Message: "请选择一个要撤销的操作:", Options: commits}
	survey.AskOne(prompt, &selection)
	if selection == "" {
		fmt.Println("操作已取消。")
		return
	}
	commitHash := strings.Split(selection, " ")[0]
	fmt.Printf("正在撤销提交 %s...\n", commitHash)
	revertCmd := exec.Command("git", "revert", "--no-edit", commitHash)
	output, err := revertCmd.CombinedOutput()
	if err != nil {
		fmt.Printf("❌ Git revert 失败:\n%s\n", string(output))
		return
	}
	fmt.Printf("✅ 成功撤销！\n%s\n", string(output))
}

func findEntityContent(entityName string) (string, string, error) {
	entityDir := "internal/domain/entity"
	snakeCaseName := common.ToSnakeCase(entityName)
	var filePath string
	possiblePaths := []string{
		filepath.Join(entityDir, snakeCaseName+".go"),
		filepath.Join(entityDir, entityName+".go"),
	}
	var err error
	for _, p := range possiblePaths {
		if _, e := os.Stat(p); e == nil {
			filePath = p
			break
		} else {
			err = e
		}
	}
	if filePath == "" {
		return "", "", fmt.Errorf("在 %s 目录下未找到实体文件 (尝试了 %s.go 和 %s.go): %w", entityDir, snakeCaseName, entityName, err)
	}
	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", "", err
	}
	return string(content), filePath, nil
}

func findMapperContent(entityName string) (string, string, error) {
	mapperDir := "internal/interfaces/dto" // 假设 DDD 结构
	mapperFileName := common.ToSnakeCase(entityName) + "_mapper.go"
	mapperPath := filepath.Join(mapperDir, mapperFileName)
	content, err := os.ReadFile(mapperPath)
	if err != nil {
		return "", mapperPath, err
	}
	return string(content), mapperPath, nil
}

func findEntities(dir string) ([]string, error) {
	var entities []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(info.Name(), ".go") {
			return err
		}
		fset := token.NewFileSet()
		node, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return fmt.Errorf("解析文件 %s 失败: %w", path, err)
		}
		ast.Inspect(node, func(n ast.Node) bool {
			if ts, ok := n.(*ast.TypeSpec); ok {
				if _, ok := ts.Type.(*ast.StructType); ok {
					entities = append(entities, ts.Name.Name)
				}
			}
			return true
		})
		return nil
	})
	return entities, err
}

func findGenApiCommits(limit int) ([]string, error) {
	cmd := exec.Command("git", "log", fmt.Sprintf("-%d", limit), "--grep=^feat(gen-api):", "--pretty=format:%h - %s")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	if len(output) == 0 {
		return []string{}, nil
	}
	return strings.Split(strings.TrimSpace(string(output)), "\n"), nil
}

func isGitClean() bool {
	cmd := exec.Command("git", "status", "--porcelain")
	output, err := cmd.Output()
	if err != nil {
		fmt.Println("⚠️ 警告: 无法检查 git 状态。")
		return true
	}
	return len(output) == 0
}

func gitCommit(message string) error {
	if _, err := exec.Command("git", "add", ".").Output(); err != nil {
		return fmt.Errorf("执行 'git add .' 失败: %w", err)
	}
	if _, err := exec.Command("git", "commit", "-m", message).Output(); err != nil {
		return fmt.Errorf("执行 'git commit' 失败: %w", err)
	}
	return nil
}

func buildApiInfo(entity, method, verb, path string) (common.ApiInfo, error) {
	info := common.ApiInfo{
		EntityName:          entity,
		LowerEntityName:     common.ToLowerCamel(entity),
		TableName:           common.ToPluralSnakeCase(entity),
		MethodName:          method,
		HttpVerb:            strings.ToUpper(verb),
		ApiPath:             path,
		FiberApiPath:        path,
		CapitalizedHttpVerb: strings.Title(strings.ToLower(verb)),
	}
	info.FullApiPath = fmt.Sprintf("/api/v1/%s%s", info.TableName, info.FiberApiPath)
	return info, nil
}

func parseCommitMessage(message string) (common.ApiInfo, error) {
	re := regexp.MustCompile(`^feat\(gen-api\): add (\w+) to (\w+) \((\w+) (.*)\)$`)
	matches := re.FindStringSubmatch(message)
	if len(matches) != 5 {
		reOld := regexp.MustCompile(`^feat\(gen-api\): add (\w+) to (\w+)$`)
		matchesOld := reOld.FindStringSubmatch(message)
		if len(matchesOld) != 3 {
			return common.ApiInfo{}, fmt.Errorf("无法解析提交信息格式: %s", message)
		}
		fmt.Println("⚠️ 警告：从旧格式的历史记录中恢复，HTTP 方法和路径可能不准确。")
		return buildApiInfo(matchesOld[2], matchesOld[1], "POST", "/<unknown>")
	}
	methodName, entityName, httpVerb, fullApiPath := matches[1], matches[2], matches[3], matches[4]
	pathParts := strings.SplitN(strings.TrimPrefix(fullApiPath, "/api/v1/"), "/", 2)
	if len(pathParts) < 2 {
		return common.ApiInfo{}, fmt.Errorf("无法从路径中解析表名: %s", fullApiPath)
	}
	apiPath := "/" + pathParts[1]
	return buildApiInfo(entityName, methodName, httpVerb, apiPath)
}

type LLMConfig struct {
	Default   string `yaml:"default"`
	Providers map[string]struct {
		Models []string `yaml:"models"`
	} `yaml:"providers"`
}

// GenerateWithDefaultLLM 是一个高级辅助函数，它负责：
// 1. 读取 `.goprojectstarter.yaml` 配置文件。
// 2. 根据配置中的 `default` 字段确定要使用的 LLM 提供商和模型。
// 3. 从环境变量中获取对应的 API Key。
// 4. 初始化选择的 LLM 客户端。
// 5. 发送 prompt 并返回结果。
func GenerateWithDefaultLLM(prompt string) (string, error) {
	// 加载 LLM 配置
	config, err := loadLLMConfig()
	if err != nil {
		return "", fmt.Errorf("无法加载 LLM 配置: %w", err)
	}

	// 解析默认的提供商和模型
	parts := strings.Split(config.Default, ":")
	if len(parts) != 2 {
		return "", fmt.Errorf("配置文件中 'default' LLM 格式无效 (应为 'provider:model'): %s", config.Default)
	}
	provider, model := parts[0], parts[1]

	var client llm.Assistant // 使用顶层 Assistant 接口
	var apiKey string

	fmt.Printf("   - 使用默认 LLM: %s (%s)\n", provider, model)

	// 根据提供商选择并初始化客户端
	switch provider {
	case "gemini":
		// Gemini 客户端通过环境变量自动读取 API key
		client, err = gemini.NewClient(gemini.WithModel(model))
	case "deepseek":
		apiKey = os.Getenv("DEEPSEEK_API_KEY")
		if apiKey == "" {
			return "", fmt.Errorf("环境变量 DEEPSEEK_API_KEY 未设置")
		}
		client, err = deepseek.NewClient(apiKey, deepseek.WithModel(model))
	case "volc":
		apiKey = os.Getenv("ARK_API_KEY")
		if apiKey == "" {
			return "", fmt.Errorf("环境变量 ARK_API_KEY 未设置")
		}
		client, err = volc.NewClient(volc.WithModel(model))
	default:
		return "", fmt.Errorf("不支持的 LLM 提供商: %s", provider)
	}

	if err != nil {
		return "", fmt.Errorf("为 %s 创建 LLM 客户端失败: %w", provider, err)
	}

	// 为 API 调用设置一个超时上下文
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// 使用通用的 Send 方法发送请求
	return client.Send(ctx, prompt)
}

// loadLLMConfig 读取并解析 .goprojectstarter.yaml 文件
func loadLLMConfig() (*LLMConfig, error) {
	file, err := os.ReadFile(".goprojectstarter.yaml")
	if err != nil {
		return nil, err
	}
	// 我们只关心文件中的 'llm' 顶级键
	var config struct {
		LLM LLMConfig `yaml:"llm"`
	}
	if err := yaml.Unmarshal(file, &config); err != nil {
		return nil, err
	}
	return &config.LLM, nil
}
