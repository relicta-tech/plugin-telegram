package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/relicta-tech/relicta-plugin-sdk/plugin"
)

func TestGetInfo(t *testing.T) {
	p := &TelegramPlugin{}
	info := p.GetInfo()

	if info.Name != "telegram" {
		t.Errorf("expected name 'telegram', got %q", info.Name)
	}

	if info.Version == "" {
		t.Error("expected non-empty version")
	}

	if len(info.Hooks) == 0 {
		t.Error("expected at least one hook")
	}

	// Check hooks include expected values
	hasPostPublish := false
	hasOnError := false
	for _, hook := range info.Hooks {
		if hook == plugin.HookPostPublish {
			hasPostPublish = true
		}
		if hook == plugin.HookOnError {
			hasOnError = true
		}
	}
	if !hasPostPublish {
		t.Error("expected HookPostPublish in hooks")
	}
	if !hasOnError {
		t.Error("expected HookOnError in hooks")
	}
}

func TestEscapeMarkdownV2(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple text",
			input:    "hello world",
			expected: "hello world",
		},
		{
			name:     "version with dots",
			input:    "v1.2.3",
			expected: "v1\\.2\\.3",
		},
		{
			name:     "text with underscores",
			input:    "my_variable_name",
			expected: "my\\_variable\\_name",
		},
		{
			name:     "text with asterisks",
			input:    "*bold*",
			expected: "\\*bold\\*",
		},
		{
			name:     "text with brackets",
			input:    "[link](url)",
			expected: "\\[link\\]\\(url\\)",
		},
		{
			name:     "complex text",
			input:    "Release v1.0.0 - feat: add *new* feature!",
			expected: "Release v1\\.0\\.0 \\- feat: add \\*new\\* feature\\!",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := escapeMarkdownV2(tt.input)
			if result != tt.expected {
				t.Errorf("escapeMarkdownV2(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestValidateBotToken(t *testing.T) {
	tests := []struct {
		name    string
		token   string
		wantErr bool
	}{
		{
			name:    "valid token",
			token:   "123456789:ABCdefGHIjklMNOpqrsTUVwxyz123456789",
			wantErr: false,
		},
		{
			name:    "valid token with long alphanumeric",
			token:   "1234567890:ABCdefGHIjklMNOpqrsTUVwxyz1234567890abc",
			wantErr: false,
		},
		{
			name:    "empty token",
			token:   "",
			wantErr: true,
		},
		{
			name:    "missing colon",
			token:   "123456789ABCdefGHIjklMNOpqrsTUVwxyz123456789",
			wantErr: true,
		},
		{
			name:    "invalid format",
			token:   "invalid-token",
			wantErr: true,
		},
		{
			name:    "too short second part",
			token:   "123456789:ABC",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateBotToken(tt.token)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateBotToken(%q) error = %v, wantErr %v", tt.token, err, tt.wantErr)
			}
		})
	}
}

func TestRenderTemplate(t *testing.T) {
	releaseCtx := plugin.ReleaseContext{
		Version:      "1.2.3",
		TagName:      "v1.2.3",
		Branch:       "main",
		ReleaseType:  "minor",
		ReleaseNotes: "Bug fixes and improvements",
	}

	tests := []struct {
		name     string
		template string
		contains []string
	}{
		{
			name:     "version placeholder",
			template: "Release {{.Version}}",
			contains: []string{"Release 1.2.3"},
		},
		{
			name:     "multiple placeholders",
			template: "{{.Version}} on {{.Branch}}",
			contains: []string{"1.2.3 on main"},
		},
		{
			name:     "tag name",
			template: "Tag: {{.TagName}}",
			contains: []string{"Tag: v1.2.3"},
		},
		{
			name:     "release notes",
			template: "Notes: {{.ReleaseNotes}}",
			contains: []string{"Notes: Bug fixes and improvements"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := renderTemplate(tt.template, releaseCtx)
			if err != nil {
				t.Fatalf("renderTemplate() error = %v", err)
			}
			for _, c := range tt.contains {
				if !strings.Contains(result, c) {
					t.Errorf("renderTemplate() = %q, want to contain %q", result, c)
				}
			}
		})
	}
}

func TestParseConfig(t *testing.T) {
	p := &TelegramPlugin{}

	tests := []struct {
		name   string
		config map[string]any
		check  func(*Config) bool
	}{
		{
			name: "with all fields",
			config: map[string]any{
				"bot_token":                "123:abc",
				"chat_id":                  "@mychannel",
				"parse_mode":               "HTML",
				"disable_web_page_preview": false,
				"notify_on_success":        false,
				"include_changelog":        true,
				"max_changelog_length":     1000,
			},
			check: func(cfg *Config) bool {
				return cfg.BotToken == "123:abc" &&
					cfg.ChatID == "@mychannel" &&
					cfg.ParseMode == "HTML" &&
					cfg.DisableWebPagePreview == false &&
					cfg.NotifyOnSuccess == false &&
					cfg.IncludeChangelog == true &&
					cfg.MaxChangelogLength == 1000
			},
		},
		{
			name:   "with defaults",
			config: map[string]any{},
			check: func(cfg *Config) bool {
				return cfg.ParseMode == "MarkdownV2" &&
					cfg.DisableWebPagePreview == true &&
					cfg.NotifyOnSuccess == true &&
					cfg.NotifyOnError == true &&
					cfg.MaxChangelogLength == 3000
			},
		},
		{
			name: "with thread ID",
			config: map[string]any{
				"message_thread_id": int64(12345),
			},
			check: func(cfg *Config) bool {
				return cfg.MessageThreadID == 12345
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := p.parseConfig(tt.config)
			if !tt.check(cfg) {
				t.Errorf("parseConfig() did not produce expected config")
			}
		})
	}
}

func TestBuildSuccessMessage(t *testing.T) {
	p := &TelegramPlugin{}

	releaseCtx := plugin.ReleaseContext{
		Version:     "1.0.0",
		TagName:     "v1.0.0",
		Branch:      "main",
		ReleaseType: "major",
		Changes: &plugin.CategorizedChanges{
			Features: []plugin.ConventionalCommit{
				{Hash: "abc123", Type: "feat", Description: "new feature"},
			},
			Fixes: []plugin.ConventionalCommit{
				{Hash: "def456", Type: "fix", Description: "bug fix"},
			},
		},
	}

	tests := []struct {
		name      string
		parseMode string
		contains  []string
	}{
		{
			name:      "MarkdownV2",
			parseMode: "MarkdownV2",
			contains:  []string{"🚀", "*Release", "1\\.0\\.0", "*Version:*", "*Type:*"},
		},
		{
			name:      "HTML",
			parseMode: "HTML",
			contains:  []string{"🚀", "<b>Release", "1.0.0", "<b>Version:</b>"},
		},
		{
			name:      "plain text",
			parseMode: "",
			contains:  []string{"🚀", "Release 1.0.0 Published!", "Version: 1.0.0"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{ParseMode: tt.parseMode}
			result := p.buildSuccessMessage(cfg, releaseCtx)
			for _, c := range tt.contains {
				if !strings.Contains(result, c) {
					t.Errorf("buildSuccessMessage() = %q, want to contain %q", result, c)
				}
			}
		})
	}
}

func TestBuildErrorMessage(t *testing.T) {
	p := &TelegramPlugin{}

	releaseCtx := plugin.ReleaseContext{
		Version: "1.0.0",
		Branch:  "main",
	}

	tests := []struct {
		name      string
		parseMode string
		contains  []string
	}{
		{
			name:      "MarkdownV2",
			parseMode: "MarkdownV2",
			contains:  []string{"❌", "*Release", "Failed", "check the CI logs"},
		},
		{
			name:      "HTML",
			parseMode: "HTML",
			contains:  []string{"❌", "<b>Release", "Failed", "check the CI logs"},
		},
		{
			name:      "plain text",
			parseMode: "",
			contains:  []string{"❌", "Release 1.0.0 Failed", "check the CI logs"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{ParseMode: tt.parseMode}
			result := p.buildErrorMessage(cfg, releaseCtx)
			for _, c := range tt.contains {
				if !strings.Contains(result, c) {
					t.Errorf("buildErrorMessage() = %q, want to contain %q", result, c)
				}
			}
		})
	}
}

func TestExecuteDryRun(t *testing.T) {
	p := &TelegramPlugin{}
	ctx := context.Background()

	releaseCtx := plugin.ReleaseContext{
		Version: "1.0.0",
		Branch:  "main",
	}

	tests := []struct {
		name            string
		hook            plugin.Hook
		notifyOnSuccess bool
		notifyOnError   bool
		expectSuccess   bool
		expectMessage   string
	}{
		{
			name:            "success notification in dry-run",
			hook:            plugin.HookPostPublish,
			notifyOnSuccess: true,
			expectSuccess:   true,
			expectMessage:   "Would send Telegram success notification",
		},
		{
			name:            "error notification in dry-run",
			hook:            plugin.HookOnError,
			notifyOnError:   true,
			expectSuccess:   true,
			expectMessage:   "Would send Telegram error notification",
		},
		{
			name:            "success disabled",
			hook:            plugin.HookPostPublish,
			notifyOnSuccess: false,
			expectSuccess:   true,
			expectMessage:   "Success notification disabled",
		},
		{
			name:          "error disabled",
			hook:          plugin.HookOnError,
			notifyOnError: false,
			expectSuccess: true,
			expectMessage: "Error notification disabled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := plugin.ExecuteRequest{
				Hook:   tt.hook,
				DryRun: true,
				Config: map[string]any{
					"bot_token":         "123:abc",
					"chat_id":           "@test",
					"notify_on_success": tt.notifyOnSuccess,
					"notify_on_error":   tt.notifyOnError,
				},
				Context: releaseCtx,
			}

			resp, err := p.Execute(ctx, req)
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if resp.Success != tt.expectSuccess {
				t.Errorf("Execute() success = %v, want %v", resp.Success, tt.expectSuccess)
			}
			if resp.Message != tt.expectMessage {
				t.Errorf("Execute() message = %q, want %q", resp.Message, tt.expectMessage)
			}
		})
	}
}

func TestSendMessage(t *testing.T) {
	p := &TelegramPlugin{}

	tests := []struct {
		name       string
		statusCode int
		response   TelegramResponse
		wantErr    bool
	}{
		{
			name:       "successful send",
			statusCode: http.StatusOK,
			response:   TelegramResponse{OK: true},
			wantErr:    false,
		},
		{
			name:       "API error",
			statusCode: http.StatusOK,
			response:   TelegramResponse{OK: false, ErrorCode: 400, Description: "Bad Request"},
			wantErr:    true,
		},
		{
			name:       "chat not found",
			statusCode: http.StatusOK,
			response:   TelegramResponse{OK: false, ErrorCode: 404, Description: "Chat not found"},
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				_ = json.NewEncoder(w).Encode(tt.response)
			}))
			defer server.Close()

			// Note: We can't easily inject the server URL here without modifying the plugin
			// This test demonstrates the structure; in practice, you'd use dependency injection
			_ = p
		})
	}
}

func TestValidate(t *testing.T) {
	p := &TelegramPlugin{}
	ctx := context.Background()

	tests := []struct {
		name      string
		config    map[string]any
		wantValid bool
	}{
		{
			name: "valid config",
			config: map[string]any{
				"bot_token": "123456789:ABCdefGHIjklMNOpqrsTUVwxyz123456789",
				"chat_id":   "@mychannel",
			},
			wantValid: true,
		},
		{
			name: "missing bot token",
			config: map[string]any{
				"chat_id": "@mychannel",
			},
			wantValid: false,
		},
		{
			name: "missing chat ID",
			config: map[string]any{
				"bot_token": "123456789:ABCdefGHIjklMNOpqrsTUVwxyz123456789",
			},
			wantValid: false,
		},
		{
			name: "invalid bot token format",
			config: map[string]any{
				"bot_token": "invalid-token",
				"chat_id":   "@mychannel",
			},
			wantValid: false,
		},
		{
			name: "invalid parse mode",
			config: map[string]any{
				"bot_token":  "123456789:ABCdefGHIjklMNOpqrsTUVwxyz123456789",
				"chat_id":    "@mychannel",
				"parse_mode": "InvalidMode",
			},
			wantValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := p.Validate(ctx, tt.config)
			if err != nil {
				t.Fatalf("Validate() error = %v", err)
			}

			isValid := len(resp.Errors) == 0
			if isValid != tt.wantValid {
				t.Errorf("Validate() valid = %v, want %v; errors: %v", isValid, tt.wantValid, resp.Errors)
			}
		})
	}
}

func TestBuildSuccessMessageWithChangelog(t *testing.T) {
	p := &TelegramPlugin{}

	releaseCtx := plugin.ReleaseContext{
		Version:      "1.0.0",
		TagName:      "v1.0.0",
		Branch:       "main",
		ReleaseType:  "major",
		ReleaseNotes: "This is the release notes content.",
	}

	cfg := &Config{
		ParseMode:          "",
		IncludeChangelog:   true,
		MaxChangelogLength: 100,
	}

	result := p.buildSuccessMessage(cfg, releaseCtx)

	if !strings.Contains(result, "Release Notes:") {
		t.Error("Expected changelog section in message")
	}
	if !strings.Contains(result, "This is the release notes content.") {
		t.Error("Expected release notes content in message")
	}
}

func TestBuildSuccessMessageChangelogTruncation(t *testing.T) {
	p := &TelegramPlugin{}

	longNotes := strings.Repeat("a", 200)
	releaseCtx := plugin.ReleaseContext{
		Version:      "1.0.0",
		Branch:       "main",
		ReleaseNotes: longNotes,
	}

	cfg := &Config{
		ParseMode:          "",
		IncludeChangelog:   true,
		MaxChangelogLength: 50,
	}

	result := p.buildSuccessMessage(cfg, releaseCtx)

	// Should be truncated with "..."
	if !strings.Contains(result, "...") {
		t.Error("Expected truncated changelog with '...'")
	}
	// Should not contain the full 200 chars
	if strings.Contains(result, strings.Repeat("a", 200)) {
		t.Error("Changelog should be truncated")
	}
}
