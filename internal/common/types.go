package common

// ApiInfo 存储了生成新 API 所需的所有信息
type ApiInfo struct {
	EntityName          string
	LowerEntityName     string
	TableName           string
	MethodName          string
	HttpVerb            string
	ApiPath             string
	FiberApiPath        string
	FullApiPath         string
	CapitalizedHttpVerb string
}

// ProjectPathConfig 存储项目结构路径
type ProjectPathConfig struct {
	RepoInterfaceDir string
	RepoImplDir      string
	ServiceDir       string
	HandlerDir       string
	RouterFile       string
}

// InsertionMode 定义了代码的插入策略
type InsertionMode int

const (
	AppendToEnd InsertionMode = iota
	InsertAfterBrace
	InsertAfterLine
)
