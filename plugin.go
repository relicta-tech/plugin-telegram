// Package main implements the Telegram plugin for Relicta.
package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/relicta-tech/relicta-plugin-sdk/helpers"
	"github.com/relicta-tech/relicta-plugin-sdk/plugin"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// Shared HTTP client for connection reuse across requests.
var defaultHTTPClient = &http.Client{
	Timeout: 30 * time.Second,
	Transport: &http.Transport{
		MaxIdleConns:        10,
		MaxIdleConnsPerHost: 5,
		IdleConnTimeout:     90 * time.Second,
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
		},
	},
}

// TelegramPlugin implements the Telegram notification plugin.
type TelegramPlugin struct{}

// Config represents the Telegram plugin configuration.
type Config struct {
	// BotToken is the Telegram bot token from @BotFather.
	BotToken string `json:"bot_token,omitempty"`
	// ChatID is the target chat ID (channel, group, or user).
	ChatID string `json:"chat_id,omitempty"`
	// MessageThreadID is the thread ID for topic-based groups.
	MessageThreadID int64 `json:"message_thread_id,omitempty"`
	// ParseMode is the message parse mode (MarkdownV2 or HTML).
	ParseMode string `json:"parse_mode,omitempty"`
	// DisableWebPagePreview disables link previews.
	DisableWebPagePreview bool `json:"disable_web_page_preview"`
	// DisableNotification sends the message silently.
	DisableNotification bool `json:"disable_notification"`
	// NotifyOnSuccess sends notification on successful release.
	NotifyOnSuccess bool `json:"notify_on_success"`
	// NotifyOnError sends notification on failed release.
	NotifyOnError bool `json:"notify_on_error"`
	// IncludeChangelog includes changelog in the notification.
	IncludeChangelog bool `json:"include_changelog"`
	// MaxChangelogLength is the maximum changelog length before truncation.
	MaxChangelogLength int `json:"max_changelog_length"`
	// Template is a custom message template.
	Template string `json:"template,omitempty"`
}

// TelegramMessage represents a sendMessage request.
type TelegramMessage struct {
	ChatID                string `json:"chat_id"`
	Text                  string `json:"text"`
	ParseMode             string `json:"parse_mode,omitempty"`
	MessageThreadID       int64  `json:"message_thread_id,omitempty"`
	DisableWebPagePreview bool   `json:"disable_web_page_preview,omitempty"`
	DisableNotification   bool   `json:"disable_notification,omitempty"`
}

// TelegramResponse represents a Telegram API response.
type TelegramResponse struct {
	OK          bool   `json:"ok"`
	Description string `json:"description,omitempty"`
	ErrorCode   int    `json:"error_code,omitempty"`
}

// GetInfo returns plugin metadata.
func (p *TelegramPlugin) GetInfo() plugin.Info {
	return plugin.Info{
		Name:        "telegram",
		Version:     "1.0.0",
		Description: "Send Telegram notifications for releases",
		Author:      "Relicta Team",
		Hooks: []plugin.Hook{
			plugin.HookPostPublish,
			plugin.HookOnSuccess,
			plugin.HookOnError,
		},
		ConfigSchema: `{
			"type": "object",
			"properties": {
				"bot_token": {"type": "string", "description": "Telegram bot token (or use TELEGRAM_BOT_TOKEN env)"},
				"chat_id": {"type": "string", "description": "Chat ID or @channel_username"},
				"message_thread_id": {"type": "integer", "description": "Thread ID for topic-based groups"},
				"parse_mode": {"type": "string", "enum": ["MarkdownV2", "HTML", ""], "description": "Message parse mode", "default": "MarkdownV2"},
				"disable_web_page_preview": {"type": "boolean", "description": "Disable link previews", "default": true},
				"disable_notification": {"type": "boolean", "description": "Send silently", "default": false},
				"notify_on_success": {"type": "boolean", "description": "Notify on success", "default": true},
				"notify_on_error": {"type": "boolean", "description": "Notify on error", "default": true},
				"include_changelog": {"type": "boolean", "description": "Include changelog", "default": false},
				"max_changelog_length": {"type": "integer", "description": "Max changelog length", "default": 3000},
				"template": {"type": "string", "description": "Custom message template"}
			},
			"required": ["chat_id"]
		}`,
	}
}

// Execute runs the plugin for a given hook.
func (p *TelegramPlugin) Execute(ctx context.Context, req plugin.ExecuteRequest) (*plugin.ExecuteResponse, error) {
	cfg := p.parseConfig(req.Config)

	switch req.Hook {
	case plugin.HookPostPublish, plugin.HookOnSuccess:
		if !cfg.NotifyOnSuccess {
			return &plugin.ExecuteResponse{
				Success: true,
				Message: "Success notification disabled",
			}, nil
		}
		return p.sendSuccessNotification(ctx, cfg, req.Context, req.DryRun)

	case plugin.HookOnError:
		if !cfg.NotifyOnError {
			return &plugin.ExecuteResponse{
				Success: true,
				Message: "Error notification disabled",
			}, nil
		}
		return p.sendErrorNotification(ctx, cfg, req.Context, req.DryRun)

	default:
		return &plugin.ExecuteResponse{
			Success: true,
			Message: fmt.Sprintf("Hook %s not handled", req.Hook),
		}, nil
	}
}

// sendSuccessNotification sends a success notification.
func (p *TelegramPlugin) sendSuccessNotification(ctx context.Context, cfg *Config, releaseCtx plugin.ReleaseContext, dryRun bool) (*plugin.ExecuteResponse, error) {
	var text string

	if cfg.Template != "" {
		// Use custom template
		var err error
		text, err = renderTemplate(cfg.Template, releaseCtx)
		if err != nil {
			return &plugin.ExecuteResponse{
				Success: false,
				Error:   fmt.Sprintf("failed to render template: %v", err),
			}, nil
		}
	} else {
		// Build default message
		text = p.buildSuccessMessage(cfg, releaseCtx)
	}

	msg := TelegramMessage{
		ChatID:                cfg.ChatID,
		Text:                  text,
		ParseMode:             cfg.ParseMode,
		MessageThreadID:       cfg.MessageThreadID,
		DisableWebPagePreview: cfg.DisableWebPagePreview,
		DisableNotification:   cfg.DisableNotification,
	}

	if dryRun {
		return &plugin.ExecuteResponse{
			Success: true,
			Message: "Would send Telegram success notification",
			Outputs: map[string]any{
				"chat_id":        cfg.ChatID,
				"version":        releaseCtx.Version,
				"message_length": len(text),
			},
		}, nil
	}

	if err := p.sendMessage(ctx, cfg.BotToken, msg); err != nil {
		return &plugin.ExecuteResponse{
			Success: false,
			Error:   fmt.Sprintf("failed to send Telegram message: %v", err),
		}, nil
	}

	return &plugin.ExecuteResponse{
		Success: true,
		Message: "Sent Telegram success notification",
		Outputs: map[string]any{
			"chat_id": cfg.ChatID,
			"version": releaseCtx.Version,
		},
	}, nil
}

// sendErrorNotification sends an error notification.
func (p *TelegramPlugin) sendErrorNotification(ctx context.Context, cfg *Config, releaseCtx plugin.ReleaseContext, dryRun bool) (*plugin.ExecuteResponse, error) {
	text := p.buildErrorMessage(cfg, releaseCtx)

	msg := TelegramMessage{
		ChatID:                cfg.ChatID,
		Text:                  text,
		ParseMode:             cfg.ParseMode,
		MessageThreadID:       cfg.MessageThreadID,
		DisableWebPagePreview: cfg.DisableWebPagePreview,
		DisableNotification:   false, // Always notify on error
	}

	if dryRun {
		return &plugin.ExecuteResponse{
			Success: true,
			Message: "Would send Telegram error notification",
			Outputs: map[string]any{
				"chat_id": cfg.ChatID,
				"version": releaseCtx.Version,
			},
		}, nil
	}

	if err := p.sendMessage(ctx, cfg.BotToken, msg); err != nil {
		return &plugin.ExecuteResponse{
			Success: false,
			Error:   fmt.Sprintf("failed to send Telegram message: %v", err),
		}, nil
	}

	return &plugin.ExecuteResponse{
		Success: true,
		Message: "Sent Telegram error notification",
	}, nil
}

// buildSuccessMessage builds the success notification message.
func (p *TelegramPlugin) buildSuccessMessage(cfg *Config, releaseCtx plugin.ReleaseContext) string {
	var sb strings.Builder

	switch cfg.ParseMode {
	case "MarkdownV2":
		sb.WriteString(fmt.Sprintf("🚀 *Release %s Published\\!*\n\n", escapeMarkdownV2(releaseCtx.Version)))
		sb.WriteString(fmt.Sprintf("📦 *Version:* `%s`\n", escapeMarkdownV2(releaseCtx.Version)))
		sb.WriteString(fmt.Sprintf("📋 *Type:* %s\n", escapeMarkdownV2(cases.Title(language.English).String(releaseCtx.ReleaseType))))
		sb.WriteString(fmt.Sprintf("🌿 *Branch:* `%s`\n", escapeMarkdownV2(releaseCtx.Branch)))
		sb.WriteString(fmt.Sprintf("🏷️ *Tag:* `%s`\n", escapeMarkdownV2(releaseCtx.TagName)))

		if releaseCtx.Changes != nil {
			features := len(releaseCtx.Changes.Features)
			fixes := len(releaseCtx.Changes.Fixes)
			breaking := len(releaseCtx.Changes.Breaking)

			sb.WriteString("\n*Changes:*\n")
			sb.WriteString(fmt.Sprintf("• %d features\n", features))
			sb.WriteString(fmt.Sprintf("• %d bug fixes\n", fixes))
			if breaking > 0 {
				sb.WriteString(fmt.Sprintf("• %d breaking changes\n", breaking))
			}
		}

		if cfg.IncludeChangelog && releaseCtx.ReleaseNotes != "" {
			notes := releaseCtx.ReleaseNotes
			if cfg.MaxChangelogLength > 0 && len(notes) > cfg.MaxChangelogLength {
				notes = notes[:cfg.MaxChangelogLength] + "..."
			}
			sb.WriteString("\n*Release Notes:*\n")
			sb.WriteString(escapeMarkdownV2(notes))
		}
	case "HTML":
		sb.WriteString(fmt.Sprintf("🚀 <b>Release %s Published!</b>\n\n", html.EscapeString(releaseCtx.Version)))
		sb.WriteString(fmt.Sprintf("📦 <b>Version:</b> <code>%s</code>\n", html.EscapeString(releaseCtx.Version)))
		sb.WriteString(fmt.Sprintf("📋 <b>Type:</b> %s\n", html.EscapeString(cases.Title(language.English).String(releaseCtx.ReleaseType))))
		sb.WriteString(fmt.Sprintf("🌿 <b>Branch:</b> <code>%s</code>\n", html.EscapeString(releaseCtx.Branch)))
		sb.WriteString(fmt.Sprintf("🏷️ <b>Tag:</b> <code>%s</code>\n", html.EscapeString(releaseCtx.TagName)))

		if releaseCtx.Changes != nil {
			features := len(releaseCtx.Changes.Features)
			fixes := len(releaseCtx.Changes.Fixes)
			breaking := len(releaseCtx.Changes.Breaking)

			sb.WriteString("\n<b>Changes:</b>\n")
			sb.WriteString(fmt.Sprintf("• %d features\n", features))
			sb.WriteString(fmt.Sprintf("• %d bug fixes\n", fixes))
			if breaking > 0 {
				sb.WriteString(fmt.Sprintf("• %d breaking changes\n", breaking))
			}
		}

		if cfg.IncludeChangelog && releaseCtx.ReleaseNotes != "" {
			notes := releaseCtx.ReleaseNotes
			if cfg.MaxChangelogLength > 0 && len(notes) > cfg.MaxChangelogLength {
				notes = notes[:cfg.MaxChangelogLength] + "..."
			}
			sb.WriteString("\n<b>Release Notes:</b>\n")
			sb.WriteString(html.EscapeString(notes))
		}
	default:
		sb.WriteString(fmt.Sprintf("🚀 Release %s Published!\n\n", releaseCtx.Version))
		sb.WriteString(fmt.Sprintf("📦 Version: %s\n", releaseCtx.Version))
		sb.WriteString(fmt.Sprintf("📋 Type: %s\n", cases.Title(language.English).String(releaseCtx.ReleaseType)))
		sb.WriteString(fmt.Sprintf("🌿 Branch: %s\n", releaseCtx.Branch))
		sb.WriteString(fmt.Sprintf("🏷️ Tag: %s\n", releaseCtx.TagName))

		if releaseCtx.Changes != nil {
			features := len(releaseCtx.Changes.Features)
			fixes := len(releaseCtx.Changes.Fixes)
			breaking := len(releaseCtx.Changes.Breaking)

			sb.WriteString("\nChanges:\n")
			sb.WriteString(fmt.Sprintf("• %d features\n", features))
			sb.WriteString(fmt.Sprintf("• %d bug fixes\n", fixes))
			if breaking > 0 {
				sb.WriteString(fmt.Sprintf("• %d breaking changes\n", breaking))
			}
		}

		if cfg.IncludeChangelog && releaseCtx.ReleaseNotes != "" {
			notes := releaseCtx.ReleaseNotes
			if cfg.MaxChangelogLength > 0 && len(notes) > cfg.MaxChangelogLength {
				notes = notes[:cfg.MaxChangelogLength] + "..."
			}
			sb.WriteString("\nRelease Notes:\n")
			sb.WriteString(notes)
		}
	}

	return sb.String()
}

// buildErrorMessage builds the error notification message.
func (p *TelegramPlugin) buildErrorMessage(cfg *Config, releaseCtx plugin.ReleaseContext) string {
	var sb strings.Builder

	switch cfg.ParseMode {
	case "MarkdownV2":
		sb.WriteString(fmt.Sprintf("❌ *Release %s Failed*\n\n", escapeMarkdownV2(releaseCtx.Version)))
		sb.WriteString(fmt.Sprintf("📦 *Version:* `%s`\n", escapeMarkdownV2(releaseCtx.Version)))
		sb.WriteString(fmt.Sprintf("🌿 *Branch:* `%s`\n", escapeMarkdownV2(releaseCtx.Branch)))
		sb.WriteString("\nPlease check the CI logs for details\\.")
	case "HTML":
		sb.WriteString(fmt.Sprintf("❌ <b>Release %s Failed</b>\n\n", html.EscapeString(releaseCtx.Version)))
		sb.WriteString(fmt.Sprintf("📦 <b>Version:</b> <code>%s</code>\n", html.EscapeString(releaseCtx.Version)))
		sb.WriteString(fmt.Sprintf("🌿 <b>Branch:</b> <code>%s</code>\n", html.EscapeString(releaseCtx.Branch)))
		sb.WriteString("\nPlease check the CI logs for details.")
	default:
		sb.WriteString(fmt.Sprintf("❌ Release %s Failed\n\n", releaseCtx.Version))
		sb.WriteString(fmt.Sprintf("📦 Version: %s\n", releaseCtx.Version))
		sb.WriteString(fmt.Sprintf("🌿 Branch: %s\n", releaseCtx.Branch))
		sb.WriteString("\nPlease check the CI logs for details.")
	}

	return sb.String()
}

// sendMessage sends a message to Telegram.
func (p *TelegramPlugin) sendMessage(ctx context.Context, botToken string, msg TelegramMessage) error {
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", botToken)

	payload, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := defaultHTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var telegramResp TelegramResponse
	if err := json.NewDecoder(resp.Body).Decode(&telegramResp); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	if !telegramResp.OK {
		return fmt.Errorf("telegram API error (%d): %s", telegramResp.ErrorCode, telegramResp.Description)
	}

	return nil
}

// parseConfig parses the plugin configuration.
func (p *TelegramPlugin) parseConfig(raw map[string]any) *Config {
	parser := helpers.NewConfigParser(raw)

	// Get bot token with env fallback
	botToken := parser.GetString("bot_token", "TELEGRAM_BOT_TOKEN", "")

	// Get chat ID with env fallback
	chatID := parser.GetString("chat_id", "TELEGRAM_CHAT_ID", "")

	// Get message thread ID
	var messageThreadID int64
	if v, ok := raw["message_thread_id"]; ok {
		switch val := v.(type) {
		case int64:
			messageThreadID = val
		case int:
			messageThreadID = int64(val)
		case float64:
			messageThreadID = int64(val)
		}
	}

	// Get max changelog length
	maxChangelogLength := 3000
	if v, ok := raw["max_changelog_length"]; ok {
		switch val := v.(type) {
		case int:
			maxChangelogLength = val
		case int64:
			maxChangelogLength = int(val)
		case float64:
			maxChangelogLength = int(val)
		}
	}

	return &Config{
		BotToken:              botToken,
		ChatID:                chatID,
		MessageThreadID:       messageThreadID,
		ParseMode:             parser.GetString("parse_mode", "", "MarkdownV2"),
		DisableWebPagePreview: parser.GetBool("disable_web_page_preview", true),
		DisableNotification:   parser.GetBool("disable_notification", false),
		NotifyOnSuccess:       parser.GetBool("notify_on_success", true),
		NotifyOnError:         parser.GetBool("notify_on_error", true),
		IncludeChangelog:      parser.GetBool("include_changelog", false),
		MaxChangelogLength:    maxChangelogLength,
		Template:              parser.GetString("template", "", ""),
	}
}

// Validate validates the plugin configuration.
func (p *TelegramPlugin) Validate(ctx context.Context, config map[string]any) (*plugin.ValidateResponse, error) {
	vb := helpers.NewValidationBuilder()

	parser := helpers.NewConfigParser(config)
	botToken := parser.GetString("bot_token", "TELEGRAM_BOT_TOKEN", "")
	chatID := parser.GetString("chat_id", "TELEGRAM_CHAT_ID", "")

	// Check environment fallback if not in config
	if botToken == "" {
		botToken = os.Getenv("TELEGRAM_BOT_TOKEN")
	}
	if chatID == "" {
		chatID = os.Getenv("TELEGRAM_CHAT_ID")
	}

	// Validate bot token
	if botToken == "" {
		vb.AddErrorWithCode("bot_token",
			"Telegram bot token is required (set TELEGRAM_BOT_TOKEN env var or configure bot_token)",
			"required")
	} else if err := validateBotToken(botToken); err != nil {
		vb.AddErrorWithCode("bot_token", err.Error(), "format")
	}

	// Validate chat ID
	if chatID == "" {
		vb.AddErrorWithCode("chat_id",
			"Chat ID is required (set TELEGRAM_CHAT_ID env var or configure chat_id)",
			"required")
	}

	// Validate parse mode
	parseMode := parser.GetString("parse_mode", "", "MarkdownV2")
	if parseMode != "" && parseMode != "MarkdownV2" && parseMode != "HTML" {
		vb.AddErrorWithCode("parse_mode",
			"Parse mode must be 'MarkdownV2', 'HTML', or empty",
			"enum")
	}

	// Note: We don't verify chat access during validation to avoid network calls
	// The actual send will fail if the chat is inaccessible

	return vb.Build(), nil
}


// validateBotToken validates a Telegram bot token format.
func validateBotToken(token string) error {
	// Bot token format: 123456789:ABCdefGHIjklMNOpqrsTUVwxyz123456789
	pattern := regexp.MustCompile(`^\d+:[A-Za-z0-9_-]{35,}$`)
	if !pattern.MatchString(token) {
		return fmt.Errorf("invalid bot token format")
	}
	return nil
}

// escapeMarkdownV2 escapes special characters for Telegram MarkdownV2.
func escapeMarkdownV2(text string) string {
	// Characters that need escaping in MarkdownV2
	specialChars := []string{"_", "*", "[", "]", "(", ")", "~", "`", ">", "#", "+", "-", "=", "|", "{", "}", ".", "!"}

	result := text
	for _, char := range specialChars {
		result = strings.ReplaceAll(result, char, "\\"+char)
	}
	return result
}

// renderTemplate renders a custom template with release context.
func renderTemplate(templateStr string, releaseCtx plugin.ReleaseContext) (string, error) {
	// Simple template replacement
	result := templateStr
	result = strings.ReplaceAll(result, "{{.Version}}", releaseCtx.Version)
	result = strings.ReplaceAll(result, "{{.TagName}}", releaseCtx.TagName)
	result = strings.ReplaceAll(result, "{{.Branch}}", releaseCtx.Branch)
	result = strings.ReplaceAll(result, "{{.ReleaseType}}", releaseCtx.ReleaseType)
	result = strings.ReplaceAll(result, "{{.ReleaseNotes}}", releaseCtx.ReleaseNotes)
	result = strings.ReplaceAll(result, "{{.Date}}", time.Now().Format("2006-01-02"))
	return result, nil
}
