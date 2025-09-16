# ==============================================================================
# Makefile for goprojectstarter - Focused on Go Module Versioning
# ==============================================================================

# --- Application Information ---
APP_NAME := goprojectstarter
MAIN_PATH := ./cmd/$(APP_NAME)/main.go
OUTPUT_DIR := ./bin

# --- Versioning ---
# 版本号必须在运行时提供, 例如: make release VERSION=v1.2.3
# 这可以防止意外发布
VERSION ?= ""
# 默认的 Git 分支
BRANCH ?= $(shell git rev-parse --abbrev-ref HEAD)

# --- Go Build Configuration ---
GOCMD := go
GO_BUILD := $(GOCMD) build
LDFLAGS := -ldflags="-s -w -X main.version=$(VERSION)"


# ==============================================================================
# ✨ Primary Commands (主要命令)
# ==============================================================================
.PHONY: all help release

all: help ## ✨ (默认) 显示帮助信息

help: ## 帮助: 显示所有可用的 make 命令
	@echo "Usage: make <target> [ARGS...]"
	@echo ""
	@echo "Primary Targets:"
	@echo "  release           🚀 发布新版本。需要 VERSION 参数。"
	@echo "                      (例如: make release VERSION=v1.0.1)"
	@echo ""
	@echo "Development Targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | grep -v '✨' | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-20s %s\n", $$1, $$2}'
	@echo ""
	@echo "Release Artifact Targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | grep '📦' | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-20s %s\n", $$1, $$2}'


release: ## 🚀 发布新版本: 推送代码, 创建并推送一个新的 git tag (例如: make release VERSION=v1.2.3)
	@# 1. 检查是否提供了 VERSION 参数
	@if [ -z "$(VERSION)" ]; then \
		echo "❌ 错误: 请提供 VERSION 参数 (例如: make release VERSION=v1.2.3)"; \
		exit 1; \
	fi
	@# 2. 检查 Git 工作区是否干净
	@if [ -n "$(shell git status --porcelain)" ]; then \
		echo "❌ 错误: Git 工作区有未提交的更改。请先提交或储藏。"; \
		exit 1; \
	fi
	@# 3. 推送当前分支的提交，确保 tag 打在最新的代码上
	@echo ">> 正在推送 '$(BRANCH)' 分支的提交..."
	@git push origin $(BRANCH)
	@# 4. 创建 Git Tag
	@echo ">> 正在创建 Tag: $(VERSION)..."
	@git tag -a "$(VERSION)" -m "Release $(VERSION)"
	@# 5. 推送 Git Tag
	@echo ">> 正在推送 Tag: $(VERSION)..."
	@git push origin "$(VERSION)"
	@echo "\n✅ 版本 $(VERSION) 已成功发布！"
	@echo "   现在其他项目可以通过 'go get github.com/Skyenought/goprojectstarter/pkg@$(VERSION)' 来使用这个版本。"


# ==============================================================================
# 🛠️ Development Commands (开发常用命令)
# ==============================================================================
.PHONY: build run test clean

build: ## 构建: 编译适用于当前系统的二进制文件
	@echo ">> Building for current OS..."
	@mkdir -p $(OUTPUT_DIR)
	$(GO_BUILD) -o $(OUTPUT_DIR)/$(APP_NAME) $(MAIN_PATH)
	@echo ">> Build complete: $(OUTPUT_DIR)/$(APP_NAME)"

run: build ## 运行: 构建并运行应用
	@$(OUTPUT_DIR)/$(APP_NAME)

test: ## 测试: 运行所有的 Go 测试
	@$(GO_TEST) -v ./...

clean: ## 清理: 删除所有构建产物
	@rm -rf $(OUTPUT_DIR)


# ==============================================================================
# 📦 Release Artifact Commands (用于 GitHub Release 的产物构建)
# ==============================================================================
.PHONY: release-files cross-build package

release-files: package ## 📦 构建发布产物: 为所有平台交叉编译并打包 (例如: make release-files VERSION=v1.2.3)
	@if [ -z "$(VERSION)" ]; then \
		echo "❌ 错误: 请提供 VERSION 参数 (例如: make release-files VERSION=v1.2.3)"; \
		exit 1; \
	fi
	@echo "\n✅ 版本 $(VERSION) 的发布产物已生成在 '$(OUTPUT_DIR)' 目录下。"
	@echo "   您可以将这些 .tar.gz 文件上传到 GitHub Release 页面。"
	@ls -lh $(OUTPUT_DIR)

cross-build: ## (内部命令) 为所有在 PLATFORMS 中定义的平台进行构建
	@echo ">> Cross-compiling for version $(VERSION)..."
	@$(foreach p, linux/amd64 windows/amd64 darwin/amd64 darwin/arm64, \
		$(eval GOOS := $(word 1, $(subst /, ,$p))) \
		$(eval GOARCH := $(word 2, $(subst /, ,$p))) \
		$(eval EXT :=) \
		$(if $(findstring windows,$(GOOS)),$(eval EXT := .exe)) \
		echo "   -> Building for $(GOOS)/$(GOARCH)..."; \
		GOOS=$(GOOS) GOARCH=$(GOARCH) $(GO_BUILD) $(LDFLAGS) -o $(OUTPUT_DIR)/$(APP_NAME)-$(GOOS)-$(GOARCH)$(EXT) $(MAIN_PATH); \
	)

package: cross-build ## (内部命令) 将交叉编译后的文件打包成 .tar.gz
	@echo ">> Packaging release artifacts..."
	@$(foreach p, linux/amd64 windows/amd64 darwin/amd64 darwin/arm64, \
		$(eval GOOS := $(word 1, $(subst /, ,$p))) \
		$(eval GOARCH := $(word 2, $(subst /, ,$p))) \
		$(eval EXT :=) \
		$(if $(findstring windows,$(GOOS)),$(eval EXT := .exe)) \
		$(eval BINARY_NAME := $(APP_NAME)-$(GOOS)-$(GOARCH)$(EXT)) \
		$(eval ARCHIVE_NAME := $(APP_NAME)-$(VERSION)-$(GOOS)-$(GOARCH).tar.gz) \
		echo "   -> Creating archive $(ARCHIVE_NAME)..."; \
		tar -czf $(OUTPUT_DIR)/$(ARCHIVE_NAME) -C $(OUTPUT_DIR) $(BINARY_NAME); \
	)
	@# 清理掉未打包的二进制文件，只保留压缩包
	@find $(OUTPUT_DIR) -type f -not -name '*.tar.gz' -delete