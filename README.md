# Relicta Telegram Plugin

Official Telegram plugin for [Relicta](https://github.com/relicta-tech/relicta) - AI-powered release management.

## Features

- Send release notifications to Telegram channels, groups, or users
- Support for MarkdownV2 and HTML formatting
- Configurable success/error notifications
- Include changelog in notifications
- Support for message threads (topics)
- Custom message templates

## Installation

```bash
relicta plugin install telegram
relicta plugin enable telegram
```

## Configuration

Add to your `relicta.config.yaml`:

```yaml
plugins:
  - name: telegram
    enabled: true
    config:
      chat_id: "@myproject_releases"
      parse_mode: "MarkdownV2"
      notify_on_success: true
      notify_on_error: true
      include_changelog: true
      disable_web_page_preview: true
```

### Environment Variables

| Variable | Description | Required |
|----------|-------------|----------|
| `TELEGRAM_BOT_TOKEN` | Bot token from @BotFather | Yes |
| `TELEGRAM_CHAT_ID` | Default chat ID | No |

### Configuration Options

| Option | Description | Default |
|--------|-------------|---------|
| `bot_token` | Telegram bot token (prefer using env var) | - |
| `chat_id` | Chat ID or @channel_username | - |
| `message_thread_id` | Thread ID for topic-based groups | - |
| `parse_mode` | Message format: `MarkdownV2`, `HTML`, or empty | `MarkdownV2` |
| `disable_web_page_preview` | Disable link previews | `true` |
| `disable_notification` | Send message silently | `false` |
| `notify_on_success` | Send notification on success | `true` |
| `notify_on_error` | Send notification on error | `true` |
| `include_changelog` | Include changelog in message | `false` |
| `max_changelog_length` | Max changelog length before truncation | `3000` |
| `template` | Custom message template | - |

## Creating a Bot

1. Open Telegram and search for [@BotFather](https://t.me/BotFather)
2. Send `/newbot` and follow the prompts
3. Copy the bot token and set it as `TELEGRAM_BOT_TOKEN`
4. Add your bot to your channel/group as an admin

## Getting Chat ID

### For Channels
Use the channel username with `@` prefix: `@mychannel`

### For Groups
1. Add [@userinfobot](https://t.me/userinfobot) to your group
2. It will display the group ID (negative number)

### For Users
1. Send a message to [@userinfobot](https://t.me/userinfobot)
2. It will display your user ID

## Custom Templates

You can use a custom template for messages:

```yaml
plugins:
  - name: telegram
    config:
      chat_id: "@releases"
      parse_mode: ""
      template: |
        🚀 New Release: {{.Version}}

        Branch: {{.Branch}}
        Tag: {{.TagName}}

        {{.ReleaseNotes}}
```

### Available Template Variables

| Variable | Description |
|----------|-------------|
| `{{.Version}}` | Release version |
| `{{.TagName}}` | Git tag name |
| `{{.Branch}}` | Git branch |
| `{{.ReleaseType}}` | Type (major, minor, patch) |
| `{{.ReleaseNotes}}` | Generated release notes |
| `{{.Date}}` | Current date (YYYY-MM-DD) |

## Message Threads (Topics)

For topic-based supergroups, specify the thread ID:

```yaml
plugins:
  - name: telegram
    config:
      chat_id: "-1001234567890"
      message_thread_id: 12345
```

## Hooks

This plugin responds to the following hooks:

- `post_publish` - Sends success notification
- `on_success` - Sends success notification
- `on_error` - Sends error notification

## Example Messages

### Success (MarkdownV2)

```
🚀 Release v1.2.3 Published!

📦 Version: v1.2.3
📋 Type: Minor
🌿 Branch: main
🏷️ Tag: v1.2.3

Changes:
• 3 features
• 5 bug fixes
```

### Error

```
❌ Release v1.2.3 Failed

📦 Version: v1.2.3
🌿 Branch: main

Please check the CI logs for details.
```

## Development

```bash
# Build
go build -o telegram .

# Test
go test -v ./...

# Test locally
relicta plugin install ./telegram
relicta plugin enable telegram
relicta publish --dry-run
```

## License

MIT License - see [LICENSE](LICENSE) for details.
