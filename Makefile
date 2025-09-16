# ==============================================================================
# Makefile for goprojectstarter - With Independent Module Versioning
# ==============================================================================

# --- Application Information ---
APP_NAME := goprojectstarter
MAIN_PATH := ./cmd/$(APP_NAME)/main.go
OUTPUT_DIR := ./bin

# --- Versioning ---
# 版本号现在是针对具体命令提供的
# CLI 版本: make release-cli VERSION=v1.2.3
# PKG 版本: make release-pkg PKG_VERSION=v0.5.0
VERSION ?= ""
PKG_VERSION ?= ""
BRANCH ?= $(shell git rev-parse --abbrev-ref HEAD)

# --- Go Build Configuration ---
GOCMD := go
GO_BUILD := $(GOCMD) build
# LDFLAGS 现在会根据 VERSION 变量动态变化
LDFLAGS := -ldflags="-s -w -X main.version=$(VERSION)"


# ==============================================================================
# ✨ Primary Release Commands (主要发布命令)
# ==============================================================================
.PHONY: help release-cli release-pkg release-all

help: ## 帮助: 显示所有可用的 make 命令
	@echo "Usage: make <target> [ARGS...]"
	@echo ""
	@echo "Primary Release Targets:"
	@echo "  release-cli       🚀 仅发布 CLI 工具的新版本。"
	@echo "                      (例: make release-cli VERSION=v1.1.0)"
	@echo "  release-pkg       📦 仅发布 pkg 库的新版本。"
	@echo "                      (例: make release-pkg PKG_VERSION=v0.2.1)"
	@echo "  release-all       🚀📦 同步发布 CLI 和 pkg 的新版本。"
	@echo "                      (例: make release-all VERSION=v1.2.0)"
	@echo ""
	@# ... (其他 help 内容)

release-cli: ## 🚀 仅发布 CLI: 创建并推送一个根模块的 git tag (例: make release-cli VERSION=v1.1.0)
	@# 1. 检查 VERSION
	@if [ -z "$(VERSION)" ]; then echo "❌ 错误: 请提供 VERSION 参数"; exit 1; fi
	@# 2. 检查工作区
	@if [ -n "$(shell git status --porcelain)" ]; then echo "❌ 错误: Git 工作区有未提交的更改。"; exit 1; fi
	@# 3. 推送代码
	@echo ">> 正在推送 '$(BRANCH)' 分支的提交..."
	@git push origin $(BRANCH)
	@# 4. 创建 CLI 的 Tag
	@echo ">> 正在为 CLI 创建 Tag: $(VERSION)..."
	@git tag -a "$(VERSION)" -m "Release $(VERSION) for main module (CLI)"
	@# 5. 推送 CLI 的 Tag
	@echo ">> 正在推送 Tag: $(VERSION)..."
	@git push origin "$(VERSION)"
	@echo "\n✅ CLI 工具版本 $(VERSION) 已成功发布！"
	@echo "   - 安装命令: go install github.com/Skyenought/goprojectstarter/cmd/goprojectstarter@$(VERSION)"

release-pkg: ## 📦 仅发布 PKG: 创建并推送一个 pkg 子模块的 git tag (例: make release-pkg PKG_VERSION=v0.2.1)
	@# 1. 检查 PKG_VERSION
	@if [ -z "$(PKG_VERSION)" ]; then echo "❌ 错误: 请提供 PKG_VERSION 参数"; exit 1; fi
	@# 2. 检查工作区
	@if [ -n "$(shell git status --porcelain)" ]; then echo "❌ 错误: Git 工作区有未提交的更改。"; exit 1; fi
	@# 3. 推送代码
	@echo ">> 正在推送 '$(BRANCH)' 分支的提交..."
	@git push origin $(BRANCH)
	@# 4. 创建 pkg 的 Tag
	@echo ">> 正在为 pkg 模块创建 Tag: pkg/$(PKG_VERSION)..."
	@git tag -a "pkg/$(PKG_VERSION)" -m "Release $(PKG_VERSION) for pkg module"
	@# 5. 推送 pkg 的 Tag
	@echo ">> 正在推送 Tag: pkg/$(PKG_VERSION)..."
	@git push origin "pkg/$(PKG_VERSION)"
	@echo "\n✅ pkg 库版本 $(PKG_VERSION) 已成功发布！"
	@echo "   - 使用命令: go get github.com/Skyenought/goprojectstarter/pkg@$(PKG_VERSION)"

release-all: ## 🚀📦 同步发布: 同时发布 CLI 和 pkg (例: make release-all VERSION=v1.2.0)
	@# 1. 检查 VERSION
	@if [ -z "$(VERSION)" ]; then echo "❌ 错误: 请提供 VERSION 参数"; exit 1; fi
	@# 2. 检查工作区
	@if [ -n "$(shell git status --porcelain)" ]; then echo "❌ 错误: Git 工作区有未提交的更改。"; exit 1; fi
	@# 3. 推送代码
	@echo ">> 正在推送 '$(BRANCH)' 分支的提交..."
	@git push origin $(BRANCH)
	@# 4. 创建两个 Tags
	@echo ">> 正在创建 Tag: $(VERSION)..."
	@git tag -a "$(VERSION)" -m "Release $(VERSION) for main module (CLI)"
	@echo ">> 正在创建 Tag: pkg/$(VERSION)..."
	@git tag -a "pkg/$(VERSION)" -m "Release $(VERSION) for pkg module"
	@# 5. 推送两个 Tags
	@echo ">> 正在推送所有 Tags..."
	@git push origin "$(VERSION)" "pkg/$(VERSION)"
	@echo "\n✅ 同步版本 $(VERSION) 已成功发布！"
	@echo "   - CLI 工具: go install github.com/Skyenought/goprojectstarter/cmd/goprojectstarter@$(VERSION)"
	@echo "   - 库: go get github.com/Skyenought/goprojectstarter/pkg@$(VERSION)"


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
	@$(GOCMD) test -v ./...

clean: ## 清理: 删除所有构建产物
	@rm -rf $(OUTPUT_DIR)


# ==============================================================================
# 📦 Release Artifact Commands (用于 GitHub Release 的产物构建)
# ==============================================================================
.PHONY: release-files cross-build package

release-files: package ## 📦 构建发布产物: 为所有平台交叉编译并打包 (例如: make release-files VERSION=v1.2.3)
	@if [ -z "$(VERSION)" ]; then echo "❌ 错误: 请提供 VERSION 参数"; exit 1; fi
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