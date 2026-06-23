package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
)

// loadWebConfig 在 Web 模式下加载 cfdata-config.json 并应用到全局变量
func loadWebConfig() {
	configPath := defaultCLIConfigPath()
	fileCfg, created, err := loadOrCreateCLIConfig(configPath)
	if err != nil {
		fmt.Printf("[config] 配置文件加载失败: %v\n", err)
		return
	}
	if created {
		fmt.Printf("[config] 已生成配置文件模板: %s\n", configPath)
		fmt.Println("[config] 编辑配置文件后可持久化 Web 设置，也可在网页中保存")
		return
	}

	// 应用配置到全局变量
	if fileCfg.URL != "" {
		speedTestURL = fileCfg.URL
	}
	if fileCfg.DNS != "" {
		customDNSServer = fileCfg.DNS
		customDNSForced = true
	}
	if fileCfg.SpeedTest > 0 {
		speedTestWorkers = fileCfg.SpeedTest
	}

	fmt.Printf("[config] 已加载配置文件: %s\n", configPath)
}

// handleGetConfig GET /api/config — 返回 cliFileConfig 的 JSON（不含模板包装）
func handleGetConfig(w http.ResponseWriter, r *http.Request) {
	configPath := defaultCLIConfigPath()
	data, err := os.ReadFile(configPath)
	if err != nil {
		// 文件不存在时返回默认配置
		defaultCfg := defaultCLIFileConfig()
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(w).Encode(defaultCfg)
		return
	}
	// 先尝试解析为模板格式（带 config 包装）
	var template struct {
		Config json.RawMessage `json:"config"`
	}
	if err := json.Unmarshal(data, &template); err == nil && len(template.Config) > 0 {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Write(template.Config)
		return
	}
	// 失败则直接返回原始内容
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Write(data)
}

// handleSaveConfig POST /api/config — 接收部分 cliFileConfig JSON，合并后写入文件
func handleSaveConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "仅支持 POST", http.StatusMethodNotAllowed)
		return
	}

	configPath := defaultCLIConfigPath()

	// 读取当前配置
	var current cliFileConfig
	if data, err := os.ReadFile(configPath); err == nil {
		// 尝试模板格式
		var template struct {
			Config cliFileConfig `json:"config"`
		}
		if err := json.Unmarshal(data, &template); err == nil && template.Config.Mode != "" {
			current = template.Config
		} else if err := json.Unmarshal(data, &current); err != nil {
			// 无法解析则用默认值
			current = defaultCLIFileConfig()
		}
	} else {
		current = defaultCLIFileConfig()
	}

	// 先读取原始 body，同时解析为 map（判断哪些字段被发送了）和 struct
	bodyBytes, err := io.ReadAll(r.Body)
	r.Body.Close()
	if err != nil {
		http.Error(w, fmt.Sprintf("读取请求体失败: %v", err), http.StatusBadRequest)
		return
	}

	// 判断哪些字段是前端显式发送的
	var raw map[string]interface{}
	provided := map[string]bool{}
	if err := json.Unmarshal(bodyBytes, &raw); err == nil {
		for k := range raw {
			provided[k] = true
		}
	}

	// 解析为结构化配置
	var incoming cliFileConfig
	if err := json.Unmarshal(bodyBytes, &incoming); err != nil {
		http.Error(w, fmt.Sprintf("解析请求体失败: %v", err), http.StatusBadRequest)
		return
	}

	// 合并：仅覆盖传入的字段
	mergeCLIFileConfig(&current, incoming, provided)

	// 写回文件
	if err := writeCLIConfigTemplate(configPath, current); err != nil {
		http.Error(w, fmt.Sprintf("写入配置文件失败: %v", err), http.StatusInternalServerError)
		return
	}

	// 应用到当前运行的全局变量
	if incoming.URL != "" {
		speedTestURL = incoming.URL
	}
	if incoming.DNS != "" {
		customDNSServer = incoming.DNS
		customDNSForced = true
	}
	if incoming.SpeedTest > 0 {
		speedTestWorkers = incoming.SpeedTest
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "配置已保存",
		"path":    configPath,
	})
}

// mergeCLIFileConfig 将 src 的非零字段合并到 dst
// provided 标记前端实际发送的字段，用于保护未发送的 bool 不被零值覆盖
func mergeCLIFileConfig(dst *cliFileConfig, src cliFileConfig, provided map[string]bool) {
	if src.Mode != "" {
		dst.Mode = src.Mode
	}
	if src.IPType != 0 {
		dst.IPType = src.IPType
	}
	if src.Threads != 0 {
		dst.Threads = src.Threads
	}
	if src.Out != "" {
		dst.Out = src.Out
	}
	if src.SpeedTest != 0 {
		dst.SpeedTest = src.SpeedTest
	}
	if src.URL != "" {
		dst.URL = src.URL
	}
	if src.DNS != "" {
		dst.DNS = src.DNS
	}
	if src.TestPort != 0 {
		dst.TestPort = src.TestPort
	}
	if src.Delay != 0 {
		dst.Delay = src.Delay
	}
	if src.NSBFallbackPort != 0 {
		dst.NSBFallbackPort = src.NSBFallbackPort
	}
	if src.SpeedLimit != 0 {
		dst.SpeedLimit = src.SpeedLimit
	}
	if src.SpeedMin != 0 {
		dst.SpeedMin = src.SpeedMin
	}
	if src.NSBSpeedMin != 0 {
		dst.NSBSpeedMin = src.NSBSpeedMin
	}
	if src.NSBSpeedLimit != 0 {
		dst.NSBSpeedLimit = src.NSBSpeedLimit
	}
	if src.ResultLimit != 0 {
		dst.ResultLimit = src.ResultLimit
	}
	if src.DC != "" {
		dst.DC = src.DC
	}
	if src.NSBDC != "" {
		dst.NSBDC = src.NSBDC
	}
	if src.NSBIPType != "" {
		dst.NSBIPType = src.NSBIPType
	}
	if src.File != "" {
		dst.File = src.File
	}
	if src.SourceURL != "" {
		dst.SourceURL = src.SourceURL
	}
	if src.Format != "" {
		dst.Format = src.Format
	}
	if src.Fields != "" {
		dst.Fields = src.Fields
	}
	// Bool 字段——只在显式发送时覆盖
	if provided["tls"] {
		dst.TLS = src.TLS
	}
	if provided["compact"] {
		dst.Compact = src.Compact
	}
	if provided["compactipv4"] {
		dst.CompactIPv4 = src.CompactIPv4
	}
	if provided["nsbqualified"] {
		dst.NSBQualified = src.NSBQualified
	}
	if provided["github"] {
		dst.GitHub = src.GitHub
	}
	if provided["progress"] {
		dst.Progress = src.Progress
	}
	if provided["cli"] {
		dst.CLI = src.CLI
	}
	if provided["nocolor"] {
		dst.NoColor = src.NoColor
	}
	// 调试模式
	if src.Debug != nil {
		dst.Debug = src.Debug
	}
	// 导出/GitHub 字段
	if src.GHRepo != "" {
		dst.GHRepo = src.GHRepo
	}
	if src.GHBranch != "" {
		dst.GHBranch = src.GHBranch
	}
	if src.GHPath != "" {
		dst.GHPath = src.GHPath
	}
	if src.GHMessage != "" {
		dst.GHMessage = src.GHMessage
	}
	if src.GHToken != "" {
		dst.GHToken = src.GHToken
	}
	if src.GHTokenFile != "" {
		dst.GHTokenFile = src.GHTokenFile
	}
	if src.GHUpload != "" {
		dst.GHUpload = src.GHUpload
	}
	if src.Custom != "" {
		dst.Custom = src.Custom
	}
}