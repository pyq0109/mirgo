package main

import (
	"os"
	"testing"
)

func TestParseJSONC(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    map[string]string
		wantErr bool
	}{
		{
			name:  "整行注释",
			input: "// comment\n{\"name\": \"test\"}",
			want:  map[string]string{"name": "test"},
		},
		{
			name:  "行尾注释",
			input: "{\n\"name\": \"test\" // 行尾注释\n}",
			want:  map[string]string{"name": "test"},
		},
		{
			name:  "块注释",
			input: "{\n/* 块注释 */\n\"name\": \"test\"\n}",
			want:  map[string]string{"name": "test"},
		},
		{
			name:  "尾逗号",
			input: "{\"name\": \"test\",}",
			want:  map[string]string{"name": "test"},
		},
		{
			name:  "字符串内含双斜杠",
			input: "{\"url\": \"http://example.com\"} // 注释",
			want:  map[string]string{"url": "http://example.com"},
		},
		{
			name:    "非法JSON",
			input:   "{\"name\": }",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got map[string]string
			err := parseJSONC([]byte(tt.input), &got)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseJSONC(%q) expected error, got nil", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseJSONC(%q) error: %v", tt.input, err)
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("parseJSONC(%q)[%q] = %q, want %q", tt.input, k, got[k], v)
				}
			}
		})
	}
}

func TestLoadConfigWithComments(t *testing.T) {
	dir := t.TempDir()
	content := `{
	// 服务器配置
	"server": {
		"name": "测试", // 行尾注释
		"listen": {
			"addr": ":7100",
		},
	},
	/* 数据库配置 */
	"database": {
		"path": "test.db"
	}
}`
	if err := os.WriteFile(dir+"/server.jsonc", []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(dir)
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}
	if cfg.Server.Name != "测试" {
		t.Errorf("Server.Name = %q, want %q", cfg.Server.Name, "测试")
	}
	if cfg.GetListenAddr() != ":7100" {
		t.Errorf("GetListenAddr() = %q, want %q", cfg.GetListenAddr(), ":7100")
	}
	if cfg.GetDatabasePath() != "test.db" {
		t.Errorf("GetDatabasePath() = %q, want %q", cfg.GetDatabasePath(), "test.db")
	}
}
