package main

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func presetByID(t *testing.T, id string) Preset {
	t.Helper()
	for _, p := range presets {
		if p.ID == id {
			return p
		}
	}
	t.Fatalf("preset %s not found", id)
	return Preset{}
}

func presetMatches(t *testing.T, id string, h ClientHeaders) bool {
	t.Helper()
	for _, raw := range presetByID(t, id).Patterns {
		cp, err := compilePattern(raw)
		if err != nil {
			t.Fatalf("%s: %v", raw, err)
		}
		if cp.matchHeaders(h) {
			return true
		}
	}
	return false
}

func ua(s string) ClientHeaders { return ClientHeaders{UA: s} }

func TestHeaderTargetedPattern(t *testing.T) {
	h := ClientHeaders{UA: "OpenAI/JS 4.104.0", XTitle: "Cherry Studio", HTTPReferer: "https://cherry-ai.com", Custom: map[string]string{"X-Client": "claude-desktop"}}
	cases := []struct {
		p    string
		want bool
	}{
		{"header:X-Title=Cherry Studio", true},
		{"header:x-title=cherry studio", true}, // 头名、值都不区分大小写
		{"header:X-Title=re:^cherry studio$", true},
		{"header:User-Agent=Cherry", false}, // 只看指定头
		{"header:X-Client=re:^claude-desktop$", true},
		{"header:X-Client=re:^claude$", false},
		{"header:HTTP-Referer=cherry-ai.com", true},
		{"header:Referer=cherry-ai.com", false}, // Referer 与 HTTP-Referer 是两个不同的头
		{"header:X-Other=x", false},
		{"Cherry Studio", true}, // 普通子串匹配任意识别头
		{"re:^Cherry", true},
	}
	for _, c := range cases {
		cp, err := compilePattern(c.p)
		if err != nil {
			t.Fatalf("%s: %v", c.p, err)
		}
		if got := cp.matchHeaders(h); got != c.want {
			t.Errorf("%s = %v want %v", c.p, got, c.want)
		}
	}
	for _, bad := range []string{"header:=x", "header:X-Title", "header:X-Title=", "header:Authorization=sk", "header:x-api-key=1", "header:bad name=x", "header:X-Title=re:("} {
		if _, err := compilePattern(bad); err == nil {
			t.Errorf("%q should be rejected", bad)
		}
	}
}

func TestRegexCaseInsensitive(t *testing.T) {
	for _, p := range []string{"re:^CODEX[_-]", "re:^codex[_-]", "re:(?i)^Codex_", "RE:^codex_"} {
		cp, err := compilePattern(p)
		if err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		if !cp.matchHeaders(ua("Codex_CLI_RS/0.1")) {
			t.Errorf("%s should match case-insensitively", p)
		}
	}
	cp, _ := compilePattern("CoDeX")
	if !cp.matchHeaders(ua("xx codex yy")) {
		t.Error("substring should be case-insensitive")
	}
}

func TestPresetsRealStrings(t *testing.T) {
	pos := []struct {
		id string
		h  ClientHeaders
	}{
		{"codex", ua("codex_cli_rs/0.158.0 (Mac OS 15.8.0; arm64) iTerm.app")},
		{"codex-cli", ua("codex_cli_rs/0.158.0 (Mac OS 15.8.0; arm64) iTerm.app")},
		{"codex-cli", ClientHeaders{UA: "x", Originator: "codex_exec"}},
		{"codex-cli", ClientHeaders{Originator: "codex-tui"}},
		{"codex", ua("codex_desktop/26.1.0 (Mac OS 15.8.0; arm64)")},
		{"codex-desktop", ua("codex_desktop/26.1.0 (Mac OS 15.8.0; arm64)")},
		{"codex-desktop", ClientHeaders{Originator: "codex_desktop"}},
		{"codex-vscode", ua("codex_vscode/0.4.20 (Windows 10.0.26100; x86_64) vscode/1.105")},
		{"claude-code", ua("claude-cli/1.0.80 (external, cli)")},
		{"gemini-cli", ua("GeminiCLI/0.9.0/gemini-2.5-pro (darwin; arm64; terminal)")},
		{"qwen-code", ua("QwenCode/0.0.14 (darwin; arm64)")},
		{"cline", ua("Cline/3.36.0")},
		{"opencode", ua("opencode/latest/0.15.0/cli")},
		{"kilo-code", ua("kilo/latest/1.0.3/vscode")},
		{"roo-code", ua("roo-code/3.30.0 (darwin 24.6.0; arm64) node/22.17.0")},
		{"continue-cli", ua("Continue-CLI/1.5.0")},
		{"deepseek-harness", ua("deepseek-harness/0.2.0-rc.2 (+https://github.com/deepseek-ai/deepseek-harness)")},
		{"cherry-studio", ClientHeaders{UA: "Mozilla/5.0 Electron/37", XTitle: "Cherry Studio"}},
		{"cherry-studio", ClientHeaders{UA: "OpenAI/JS 5.1", HTTPReferer: "https://cherry-ai.com"}},
		{"cherry-studio", ClientHeaders{Originator: "cherry-studio"}},
		{"claude-desktop", ClientHeaders{Custom: map[string]string{"X-Client": "claude-desktop"}}},
		{"debug", ua("curl/8.4.0")},
		{"debug", ua("PostmanRuntime/7.39.0")},
		{"debug", ua("HTTPie/3.2.2")},
		{"debug", ua("Apifox/1.0.0 (https://apifox.com)")},
		{"openai-sdk", ua("OpenAI/Python 1.51.0")},
	}
	for _, c := range pos {
		if !presetMatches(t, c.id, c.h) {
			t.Errorf("preset %s should match %+v", c.id, c.h)
		}
	}
	neg := []struct {
		id string
		h  ClientHeaders
	}{
		{"codex", ua("Mozilla/5.0 (Macintosh) AppleWebKit/537.36 Chrome/130 Safari/537.36 https://x.com/codex_cli")},
		{"codex", ClientHeaders{UA: "Mozilla/5.0", Referer: "https://chatgpt.com/codex"}},
		{"codex", ua("OpenAI/JS 4.104.0")},
		{"codex", ua("my-codex-proxy/1.0")},
		{"codex-cli", ua("codex_desktop/1.0")},
		{"codex-desktop", ua("codex_cli_rs/0.158.0")},
		{"claude-code", ua("my claude-cli/1.0 wrapper")},
		{"cline", ClientHeaders{UA: "OpenAI/JS 4", XTitle: "Cline"}}, // 只认 UA 前缀 Cline/
		{"cherry-studio", ClientHeaders{UA: "CherryStudio/1.5", XTitle: "Cherry Studio Fork"}},
		{"claude-desktop", ClientHeaders{UA: "claude-desktop/1.0"}}, // 只看 X-Client 头
		{"openai-sdk", ua("codex_cli_rs/0.158.0")},
	}
	for _, c := range neg {
		if presetMatches(t, c.id, c.h) {
			t.Errorf("preset %s should NOT match %+v", c.id, c.h)
		}
	}
	if len(presetByID(t, "workbuddy").Patterns) != 0 {
		t.Error("workbuddy should have no pattern")
	}
	if presetByID(t, "openai-sdk").Warning == "" {
		t.Error("openai-sdk needs a warning")
	}
}

func TestFriendlyLabelAndSuggest(t *testing.T) {
	cases := []struct {
		h              ClientHeaders
		label, suggest string
	}{
		{ClientHeaders{UA: "codex_desktop/26.1 (Mac OS 15.8.0; arm64)", Originator: "codex_desktop"}, "Codex 桌面版", "re:^codex_desktop([/ ]|$)"},
		{ua("codex_cli_rs/0.158.0 (Mac OS 15.8.0; arm64) iTerm.app"), "Codex CLI", "re:^(codex_cli_rs|codex-tui|codex_exec|codex-cli)([/ ]|$)"},
		{ClientHeaders{Originator: "codex_sdk_ts"}, "Codex（codex_sdk_ts）", "header:Originator=re:^codex_sdk_ts$"},
		{ClientHeaders{UA: "Mozilla/5.0 Electron/37", XTitle: "Cherry Studio"}, "Cherry Studio", "header:X-Title=re:^Cherry Studio$"},
		{ua("OpenAI/JS 4.104.0"), "OpenAI SDK（通用）", "header:User-Agent=re:^OpenAI/"},
		{ua("curl/8.4.0"), "curl", "re:^curl/"},
		{ClientHeaders{UA: "WorkBuddy/2.1.0 (Windows)"}, "WorkBuddy", "header:User-Agent=re:^WorkBuddy/"},
		{ClientHeaders{UA: "node", Custom: map[string]string{"X-Client": "zhang-mac"}}, "node", "header:X-Client=re:^zhang-mac$"},
	}
	for _, c := range cases {
		if got := clientLabelHeaders(c.h); got != c.label {
			t.Errorf("label %+v = %q want %q", c.h, got, c.label)
		}
		got := suggestPattern(c.h)
		if got != c.suggest {
			t.Errorf("suggest %+v = %q want %q", c.h, got, c.suggest)
		}
		// 建议的特征必须能放行它自己
		if cp, err := compilePattern(got); err != nil || !cp.matchHeaders(c.h) {
			t.Errorf("suggest %q does not match its own client (%v)", got, err)
		}
	}
}

func TestExtractHeaders(t *testing.T) {
	h := http.Header{}
	h.Set("User-Agent", "u")
	h.Set("X-Title", "Cherry Studio")
	h.Set("HTTP-Referer", "https://cherry-ai.com")
	h.Set("X-Client", "claude-desktop")
	h.Set("X-Team", "a")
	h.Set("Authorization", "Bearer sk-secret")
	c := extractHeaders(h, []string{"x-client", "Authorization", "X-Team"})
	if c.XTitle != "Cherry Studio" || c.HTTPReferer != "https://cherry-ai.com" || c.Custom["X-Client"] != "claude-desktop" || c.Custom["X-Team"] != "a" {
		t.Fatalf("%+v", c)
	}
	if _, ok := c.Custom["Authorization"]; ok {
		t.Fatal("sensitive header leaked")
	}
}

func TestConfigExtraHeaders(t *testing.T) {
	c, err := parseConfig(nil)
	if err != nil || len(c.ExtraHeaders) != 1 || c.ExtraHeaders[0] != "X-Client" || c.DefaultAllow[0] != "re:^codex[_-]" {
		t.Fatalf("%+v %v", c, err)
	}
	c, err = parseConfig([]byte("extra_headers: [x-client, X-Device, x-client]\n"))
	if err != nil || len(c.ExtraHeaders) != 2 || c.ExtraHeaders[1] != "X-Device" {
		t.Fatalf("%+v %v", c, err)
	}
	if _, err := parseConfig([]byte("extra_headers: [Authorization]\n")); err == nil {
		t.Fatal("sensitive header accepted")
	}
}

// 规则里 header: 引用了不在 extra_headers 里的头时，请求时也会去读它。
func TestReferencedHeaderFlow(t *testing.T) {
	dir := t.TempDir()
	cfg := "data_dir: " + dir + "\n"
	raw, _ := json.Marshal(lifecycleRequest{ConfigYAML: []byte(cfg), SchemaVersion: 6})
	if _, err := handleMethod(methodPluginRegister, raw); err != nil {
		t.Fatal(err)
	}
	if _, err := guard.saveRules(RuleSet{Default: Rule{Mode: ModeEnforce, Allow: []string{"header:X-Device=re:^team-mac$", "codex"}}, Keys: map[string]Rule{}}); err != nil {
		t.Fatal(err)
	}
	call := func(hdr map[string]string) requestInterceptResponse {
		h := http.Header{}
		for k, v := range hdr {
			h.Set(k, v)
		}
		req, _ := json.Marshal(requestInterceptRequest{Headers: h, Metadata: map[string]any{"caller_scope": scopeA}})
		out, err := handleMethod(methodRequestInterceptBefore, req)
		if err != nil {
			t.Fatal(err)
		}
		var env envelope
		_ = json.Unmarshal(out, &env)
		var r requestInterceptResponse
		_ = json.Unmarshal(env.Result, &r)
		return r
	}
	if r := call(map[string]string{"User-Agent": "OpenAI/JS 4", "X-Device": "team-mac"}); r.Terminate {
		t.Fatal("X-Device rule should allow")
	}
	if r := call(map[string]string{"User-Agent": "OpenAI/JS 4", "X-Device": "other"}); !r.Terminate {
		t.Fatal("should block")
	}
	// 旧的 codex 子串规则保持可用
	if r := call(map[string]string{"User-Agent": "codex_cli_rs/0.158.0"}); r.Terminate {
		t.Fatal("old codex rule should allow")
	}
	snap := guard.snapshot()
	if len(snap.Recent) == 0 || snap.Recent[1].Custom["X-Device"] != "other" || snap.Recent[0].Client != "Codex CLI" {
		t.Fatalf("recent: %+v", snap.Recent)
	}
	guard.flush()
	if _, err := os.Stat(filepath.Join(dir, "stats.json")); err != nil {
		t.Fatal(err)
	}
}

// 0.1.1 保存的 rules.json / stats.json 原样可读，旧的 codex 规则不被改写。
func TestLegacyFilesLoad(t *testing.T) {
	dir := t.TempDir()
	rulesOld := `{"default":{"mode":"enforce","allow":["codex","CherryStudio"]},"keys":{"` + scopeA + `":{"mode":"log-only","allow":["claude-cli"],"label":"张三"}}}`
	statsOld := `{"recent":[{"time":"2026-10-12 10:00:00","scope":"` + scopeA + `","ua":"codex_cli_rs/0.46","originator":"codex_cli_rs","client":"codex_cli_rs","mode":"enforce","matched":"codex","result":"allowed"}],"seen":{"` + scopeA + `":{"codex_cli_rs":{"client":"codex_cli_rs","sample_ua":"codex_cli_rs/0.46","count":3,"blocked":0,"first_seen":"a","last_seen":"b"}}},"daily":{"2026-10-12":{"requests":3}}}`
	_ = os.WriteFile(filepath.Join(dir, "rules.json"), []byte(rulesOld), 0o600)
	_ = os.WriteFile(filepath.Join(dir, "stats.json"), []byte(statsOld), 0o600)
	g := newGuardState()
	cfg, _ := parseConfig([]byte("data_dir: " + dir + "\n"))
	g.configure(cfg, 6)
	defer g.shutdown()
	if g.lastError != "" {
		t.Fatal(g.lastError)
	}
	if g.rules.Default.Allow[0] != "codex" || g.rules.Keys[scopeA].Label != "张三" {
		t.Fatalf("rules changed: %+v", g.rules)
	}
	if g.stats.Seen[scopeA]["codex_cli_rs"].Count != 3 || len(g.stats.Recent) != 1 {
		t.Fatalf("stats: %+v", g.stats)
	}
}
