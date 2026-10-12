package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

const (
	minHostSchema   uint32 = 2
	maxPluginSchema uint32 = 6

	mgmtBase         = "/v0/management/plugins/client-guard"
	statePath        = mgmtBase + "/state"
	rulesPath        = mgmtBase + "/rules"
	clearPath        = mgmtBase + "/clear-stats"
	settingsResource = "/v0/resource/plugins/client-guard/settings"
)

func handleMethod(method string, request []byte) ([]byte, error) {
	switch method {
	case methodPluginRegister, methodPluginReconfigure:
		return register(request)
	case methodRequestInterceptBefore:
		return interceptRequest(request)
	case methodRequestInterceptAfter:
		return okEnvelope(requestInterceptResponse{}) // 只在 before 阶段判定，避免重复记录
	case methodManagementRegister:
		return okEnvelope(managementRegistration())
	case methodManagementHandle:
		return handleManagement(request)
	default:
		return errorEnvelope("unknown_method", "unknown method: "+method, 0), nil
	}
}

func register(raw []byte) ([]byte, error) {
	var req lifecycleRequest
	if err := decodeJSON(raw, &req); err != nil {
		return nil, err
	}
	cfg, err := parseConfig(req.ConfigYAML)
	if err != nil {
		// 配置写错时不阻断 CPA：回退默认值（log-only）并在页面显示错误
		warnf("%v；已使用默认配置", err)
		cfg, _ = parseConfig(nil)
		guard.configure(cfg, req.SchemaVersion)
		guard.mu.Lock()
		guard.lastError = err.Error() + "（已使用默认配置）"
		guard.mu.Unlock()
	} else {
		guard.configure(cfg, req.SchemaVersion)
	}
	schema := req.SchemaVersion
	if schema == 0 || schema > maxPluginSchema {
		schema = maxPluginSchema
	}
	return okEnvelope(registration{
		SchemaVersion: schema,
		Metadata: metadata{
			Name:             "Client Guard（客户端限制）",
			Version:          pluginVersion,
			Author:           "王远杰",
			GitHubRepository: pluginRepository,
			ConfigFields: []configField{
				{Name: "default_mode", Type: "string", EnumValues: []string{ModeOff, ModeLog, ModeEnforce}, Description: "全局默认模式，默认 log-only（只记录不拦截）。页面保存的 rules.json 优先。"},
				{Name: "default_allow", Type: "array", Description: "全局默认允许的客户端特征，默认 [re:^codex[_-]]（0.1.1 及以前默认 [codex]）。普通文本=不区分大小写子串；re: 开头=不区分大小写正则；header:名称=值 只匹配指定请求头。"},
				{Name: "fail_open", Type: "boolean", Description: "插件内部出错时是否放行，默认 true。"},
				{Name: "data_dir", Type: "string", Description: "规则与统计保存目录（相对 CPA 工作目录），默认 plugins/client-guard。"},
				{Name: "recent_limit", Type: "number", Description: "最近请求记录条数上限，默认 300，最大 2000。"},
				{Name: "extra_headers", Type: "array", Description: "额外读取并记录的自定义请求头，默认 [X-Client]。规则可用 header:X-Client=值 精确匹配。敏感头（Authorization 等）不会读取。"},
				{Name: "block_message", Type: "string", Description: "拦截时返回给客户端的提示文字。"},
			},
		},
		Capabilities: capabilities{
			RequestInterceptor: req.SchemaVersion >= minHostSchema,
			ManagementAPI:      true,
		},
	})
}

func headerFirst(h http.Header, name string) string {
	if h == nil {
		return ""
	}
	v := strings.TrimSpace(h.Get(name))
	if len(v) > 300 {
		v = v[:300]
	}
	return v
}

func interceptRequest(raw []byte) (out []byte, err error) {
	defer func() {
		if r := recover(); r != nil {
			out, err = internalFailure(fmt.Errorf("panic: %v", r))
		}
	}()
	var req requestInterceptRequest
	if err := decodeJSON(raw, &req); err != nil {
		return internalFailure(err)
	}
	scope := anonymousScope
	if s, ok := req.Metadata["caller_scope"].(string); ok {
		s = strings.ToLower(strings.TrimSpace(s))
		if validScope(s) {
			scope = s
		}
	}
	ch := extractHeaders(req.Headers, guard.headerNames())
	ua, originator := ch.UA, ch.Originator
	d, cfg := guard.decideHeaders(scope, ch)
	if d.Mode == ModeOff {
		return okEnvelope(requestInterceptResponse{})
	}
	model := strings.TrimSpace(req.RequestedModel)
	if model == "" {
		model = strings.TrimSpace(req.Model)
	}
	path, _ := req.Metadata["request_path"].(string)
	result := "allowed"
	switch {
	case d.Block:
		result = "blocked"
	case d.EmptyAllow:
		result = "unchecked"
	case !d.Allowed:
		result = "not-in-list"
	}
	guard.record(LogEntry{
		Time: guard.now(), Scope: scope, UA: ua, Originator: originator,
		XTitle: ch.XTitle, HTTPReferer: ch.HTTPReferer, Referer: ch.Referer, XClientName: ch.XClientName, Custom: ch.Custom,
		Preset: presetID(ch), Suggest: suggestPattern(ch),
		Client: clientLabelHeaders(ch), Path: path, Model: model, Format: req.SourceFormat,
		Mode: d.Mode, Matched: d.Matched, Result: result,
	}, d.Block)
	if d.Block {
		return blockedResponse(cfg.BlockMessage, ua, originator)
	}
	return okEnvelope(requestInterceptResponse{})
}

func internalFailure(err error) ([]byte, error) {
	guard.mu.RLock()
	failOpen := guard.cfg.FailOpen || !guard.configured
	guard.mu.RUnlock()
	warnf("内部错误: %v（fail_open=%v）", err, failOpen)
	if failOpen {
		return okEnvelope(requestInterceptResponse{})
	}
	body, _ := json.Marshal(map[string]any{"error": map[string]any{
		"type": "client_guard_unavailable", "message": "client-guard 插件内部错误，按 fail_open=false 拒绝请求",
	}})
	return okEnvelope(requestInterceptResponse{
		Terminate: true, StatusCode: http.StatusForbidden,
		ResponseHeaders: http.Header{"Content-Type": []string{"application/json; charset=utf-8"}},
		ResponseBody:    body,
	})
}

func blockedResponse(message, ua, originator string) ([]byte, error) {
	detail := map[string]any{
		"type":       "client_not_allowed",
		"code":       "client_not_allowed",
		"message":    message,
		"user_agent": ua,
	}
	if originator != "" {
		detail["originator"] = originator
	}
	body, err := json.Marshal(map[string]any{"error": detail})
	if err != nil {
		return nil, err
	}
	return okEnvelope(requestInterceptResponse{
		Terminate:       true,
		StatusCode:      http.StatusForbidden,
		ResponseHeaders: http.Header{"Content-Type": []string{"application/json; charset=utf-8"}},
		ResponseBody:    body,
	})
}

func managementRegistration() managementRegistrationResponse {
	return managementRegistrationResponse{
		Routes: []managementRoute{
			{Method: http.MethodGet, Path: statePath, Description: "client-guard 规则、客户端统计与最近请求"},
			{Method: http.MethodPut, Path: rulesPath, Description: "保存 client-guard 规则"},
			{Method: http.MethodPost, Path: clearPath, Description: "清空 client-guard 统计与日志"},
		},
		Resources: []resourceRoute{{Path: "/settings", Menu: "客户端限制", Description: "按 API Key 查看和限制客户端"}},
	}
}

func handleManagement(raw []byte) ([]byte, error) {
	var req managementRequest
	if err := decodeJSON(raw, &req); err != nil {
		return nil, err
	}
	method := strings.ToUpper(strings.TrimSpace(req.Method))
	path := strings.TrimRight(strings.TrimSpace(req.Path), "/")
	switch {
	case method == http.MethodGet && path == settingsResource:
		return settingsPage()
	case method == http.MethodGet && path == statePath:
		return managementJSON(http.StatusOK, guard.snapshot())
	case method == http.MethodPut && path == rulesPath:
		var rs RuleSet
		if err := json.Unmarshal(req.Body, &rs); err != nil {
			return managementJSON(http.StatusBadRequest, map[string]any{"error": "请求体不是有效 JSON: " + err.Error()})
		}
		if rs.Keys == nil {
			rs.Keys = map[string]Rule{}
		}
		saved, err := guard.saveRules(rs)
		if err != nil {
			return managementJSON(http.StatusBadRequest, map[string]any{"error": err.Error()})
		}
		return managementJSON(http.StatusOK, map[string]any{"ok": true, "rules": saved, "rules_from": guard.snapshot().RulesFrom, "warning": guard.snapshot().LastError})
	case method == http.MethodPost && path == clearPath:
		guard.clearStats()
		guard.flush()
		return managementJSON(http.StatusOK, map[string]any{"ok": true})
	default:
		return managementJSON(http.StatusNotFound, map[string]any{"error": "management route not found"})
	}
}

func managementJSON(status int, v any) ([]byte, error) {
	body, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return okEnvelope(managementResponse{
		StatusCode: status,
		Headers:    http.Header{"Content-Type": []string{"application/json; charset=utf-8"}, "Cache-Control": []string{"no-store"}},
		Body:       body,
	})
}

func okEnvelope(value any) ([]byte, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return json.Marshal(envelope{OK: true, Result: raw})
}

func errorEnvelope(code, message string, status int) []byte {
	raw, err := json.Marshal(envelope{OK: false, Error: &envelopeError{Code: code, Message: message, HTTPStatus: status}})
	if err != nil {
		return []byte(`{"ok":false,"error":{"code":"plugin_error","message":"encode error"}}`)
	}
	return raw
}

func presetID(h ClientHeaders) string {
	if p := presetMatch(h); p != nil {
		return p.ID
	}
	return ""
}
