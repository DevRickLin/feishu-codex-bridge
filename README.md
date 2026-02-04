# Feishu-Codex Bridge

A bridge that connects Feishu (Lark) group chats to Claude Codex, enabling AI-powered conversations in your team's chat groups.

## Features

- Real-time message handling via Feishu WebSocket
- Session management with conversation history
- Smart message filtering using Moonshot API (optional)
- Message buffering for non-urgent chats with scheduled processing
- MCP (Model Context Protocol) server for Feishu operations
- Support for @mentions, reactions, and rich text messages

## Architecture

```
Feishu WebSocket --> Bridge --> Codex (Claude)
                       |
                       +--> Moonshot (Filter)
                       |
                       +--> SQLite (Sessions/Buffer)
                       |
                       +--> MCP Server (Feishu Tools)
```

## Prerequisites

- Go 1.21+
- [Codex CLI](https://github.com/anthropics/claude-code) installed and configured
- Feishu app with WebSocket enabled
- (Optional) Moonshot API key for message filtering

## Installation

1. Clone the repository:
```bash
git clone https://github.com/anthropics/feishu-codex-bridge.git
cd feishu-codex-bridge
```

2. Copy the example environment file and configure:
```bash
cp .env.example .env
# Edit .env with your credentials
```

3. Build the project:
```bash
go build -o bridge ./cmd/bridge
go build -o feishu-mcp ./cmd/feishu-mcp
```

## Configuration

### Environment Variables

| Variable | Required | Description |
|----------|----------|-------------|
| `FEISHU_APP_ID` | Yes | Feishu app ID |
| `FEISHU_APP_SECRET` | Yes | Feishu app secret |
| `BOT_NAME` | Yes | Bot's display name (for @mention detection) |
| `WORKING_DIR` | Yes | Working directory for Codex |
| `CODEX_MODEL` | No | Codex model (default: claude-sonnet-4-20250514) |
| `MOONSHOT_API_KEY` | No | Moonshot API key for message filtering |
| `MOONSHOT_MODEL` | No | Moonshot model (default: moonshot-v1-8k) |
| `SESSION_DB_PATH` | No | SQLite database path (default: ~/.feishu-codex/sessions.db) |
| `SESSION_IDLE_MINUTES` | No | Session idle timeout in minutes (default: 60) |
| `SESSION_RESET_HOUR` | No | Hour to reset sessions daily (default: 4) |

### Feishu App Setup

1. Create a new app in [Feishu Open Platform](https://open.feishu.cn/)
2. Enable the following permissions:
   - `im:message` - Send and receive messages
   - `im:message.group_at_msg` - Receive @mentions in groups
   - `im:chat` - Access chat information
   - `im:chat:readonly` - Read chat history
   - `contact:user.base:readonly` - Read user information
3. Enable WebSocket in "Event Subscriptions"
4. Subscribe to event: `im.message.receive_v1`

## Running

### Direct execution
```bash
./bridge
```

### Using PM2 (recommended for production)
```bash
pm2 start ./bridge --name feishu-codex-bridge
pm2 save
```

## Message Filtering (Moonshot)

When `MOONSHOT_API_KEY` is configured, the bridge uses Moonshot to filter incoming messages:

- Messages that @mention the bot directly are always processed
- Technical questions are processed even without @mention
- Casual chat is ignored unless the chat is whitelisted

## MCP Tools

The bridge provides MCP tools that Codex can use:

- `feishu_get_chat_history` - Get recent chat messages
- `feishu_add_to_whitelist` - Add chat to instant notification whitelist
- `feishu_remove_from_whitelist` - Remove chat from whitelist
- `feishu_add_keyword` - Add keyword trigger
- `feishu_remove_keyword` - Remove keyword trigger
- `feishu_add_interest_topic` - Add topic of interest
- `feishu_get_buffer_summary` - Get buffered messages summary

## Development

```bash
# Run tests
go test ./...

# Build all binaries
go build -o bin/bridge ./cmd/bridge
go build -o bin/feishu-mcp ./cmd/feishu-mcp
```

## License

MIT
