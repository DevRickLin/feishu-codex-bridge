# Codex ACP (App Server Protocol) 完整文檔

> 來源：Codex 源碼分析 (`/home/rick/codex-source/codex-rs/app-server-protocol/`)

## 1. 協議概述

ACP 是基於 **JSON-RPC 2.0** 的雙向通信協議，通過 **stdio JSONL** 流傳輸：
- Client → Server: Requests, Notifications
- Server → Client: Responses, Notifications, Requests (approval)

**注意**: Codex ACP 不包含 `"jsonrpc":"2.0"` 標頭（與標準 JSON-RPC 略有不同）。

## 2. 消息類型

### 2.1 Client → Server

**Request (需要響應)**
```json
{
  "id": 1,
  "method": "thread/start",
  "params": { ... }
}
```

**Notification (無響應)**
```json
{
  "method": "initialized"
}
```

### 2.2 Server → Client

**Response**
```json
{
  "id": 1,
  "result": { ... }
}
```

**Error Response**
```json
{
  "id": 1,
  "error": {
    "code": -32600,
    "message": "Invalid request",
    "data": { ... }
  }
}
```

**Notification (流式事件)**
```json
{
  "method": "turn/started",
  "params": { "threadId": "xxx", "turn": { ... } }
}
```

**Request (需要 Client 響應，如 approval)**
```json
{
  "id": 100,
  "method": "item/commandExecution/requestApproval",
  "params": { "threadId": "xxx", "command": "rm -rf /tmp/foo" }
}
```

## 3. 初始化握手

必須在發送其他請求前完成：

```
Client → Server: {"id": 0, "method": "initialize", "params": {"clientInfo": {...}}}
Server → Client: {"id": 0, "result": {"userAgent": "Codex/x.y.z"}}
Client → Server: {"method": "initialized"}
```

## 4. 核心 API 端點

### 4.1 Thread 生命週期

| Method | 描述 |
|--------|------|
| `thread/start` | 創建新 thread |
| `thread/resume` | 恢復現有 thread |
| `thread/fork` | 分支對話 |
| `thread/list` | 列出 threads（支持分頁） |
| `thread/read` | 讀取 thread 但不恢復 |
| `thread/archive` | 歸檔 thread |
| `thread/rollback` | 回滾最後 N 個 turn |
| `thread/name/set` | 設置 thread 名稱 |

### 4.2 Turn 生命週期

| Method | 描述 |
|--------|------|
| `turn/start` | 發送用戶輸入，開始生成 |
| `turn/interrupt` | 中斷當前 turn |
| `review/start` | 啟動自動審查 |

### 4.3 配置

| Method | 描述 |
|--------|------|
| `config/read` | 讀取有效配置 |
| `config/value/write` | 寫入單個配置值 |
| `config/batchWrite` | 批量寫入配置 |
| `model/list` | 列出可用模型 |

## 5. Thread/Turn/Item 數據結構

### 5.1 Thread

```typescript
interface Thread {
  id: string;              // UUID
  preview: string;         // 預覽文本
  model_provider: string;  // e.g., "anthropic"
  created_at: number;      // Unix timestamp
  updated_at?: number;
  archived: boolean;
  turns: Turn[];
}
```

### 5.2 Turn

```typescript
interface Turn {
  id: string;
  status: "completed" | "interrupted" | "failed" | "inProgress";
  items: ThreadItem[];
  error?: TurnError;
}
```

### 5.3 ThreadItem (13 種類型)

```typescript
type ThreadItem =
  | { type: "userMessage"; id: string; content: ContentPart[] }
  | { type: "agentMessage"; id: string; text: string }
  | { type: "reasoning"; id: string; content: string; summary?: string }
  | { type: "commandExecution"; id: string; command: string; status: ExecutionStatus; output?: string }
  | { type: "fileChange"; id: string; changes: FileChange[]; status: ExecutionStatus }
  | { type: "mcpToolCall"; id: string; server: string; tool: string; status: ExecutionStatus }
  | { type: "plan"; id: string; text: string }
  | { type: "webSearch"; id: string; query: string }
  | { type: "imageView"; id: string; path: string }
  | { type: "collabAgentToolCall"; id: string; ... }
  | { type: "enteredReviewMode"; id: string }
  | { type: "exitedReviewMode"; id: string }
  | { type: "contextCompaction"; id: string };
```

## 6. 流式事件 (Server Notifications)

### 6.1 Thread 事件

| Event | 描述 |
|-------|------|
| `thread/started` | Thread 創建 |
| `thread/started/updated` | Thread 名稱更新 |
| `thread/tokenUsage/updated` | Token 使用量更新 |

### 6.2 Turn 事件

| Event | 描述 |
|-------|------|
| `turn/started` | Turn 開始（包含初始 turn 對象） |
| `turn/completed` | Turn 完成（包含最終狀態） |
| `turn/diff/updated` | 聚合的文件差異 |
| `turn/plan/updated` | 計劃狀態更新 |

### 6.3 Item 事件

每個 Item 的生命週期：`item/started` → [deltas] → `item/completed`

**Agent Message Delta**
```json
{
  "method": "item/agentMessage/delta",
  "params": {
    "threadId": "xxx",
    "turnId": "yyy",
    "itemId": "zzz",
    "delta": "Hello, "
  }
}
```

**Command Execution Delta**
```json
{
  "method": "item/commandExecution/outputDelta",
  "params": {
    "threadId": "xxx",
    "turnId": "yyy",
    "itemId": "zzz",
    "delta": "file1.txt\nfile2.txt\n"
  }
}
```

**Reasoning Delta**
```json
{
  "method": "item/reasoning/textDelta",
  "params": {
    "threadId": "xxx",
    "turnId": "yyy",
    "itemId": "zzz",
    "delta": "Let me think..."
  }
}
```

## 7. Approval 工作流

### 7.1 命令執行批准

```
Server → Client: {
  "id": 100,
  "method": "item/commandExecution/requestApproval",
  "params": {
    "threadId": "xxx",
    "turnId": "yyy",
    "itemId": "zzz",
    "command": "rm -rf /tmp/foo",
    "cwd": "/home/user/project"
  }
}

Client → Server: {
  "id": 100,
  "result": {
    "decision": "accept",  // or "decline"
    "acceptSettings": { ... }
  }
}
```

### 7.2 文件變更批准

```
Server → Client: {
  "id": 101,
  "method": "item/fileChange/requestApproval",
  "params": {
    "threadId": "xxx",
    "turnId": "yyy",
    "itemId": "zzz",
    "changes": [
      { "path": "/path/to/file.ts", "diff": "..." }
    ]
  }
}

Client → Server: {
  "id": 101,
  "result": {
    "decision": "accept"
  }
}
```

## 8. 錯誤處理

### 8.1 JSON-RPC 錯誤碼

| Code | 描述 |
|------|------|
| -32600 | Invalid Request |
| -32603 | Internal Error |

### 8.2 Codex 特定錯誤

```typescript
type CodexErrorInfo =
  | { type: "contextWindowExceeded" }
  | { type: "usageLimitExceeded" }
  | { type: "modelCap"; model: string; resetAfterSeconds: number }
  | { type: "httpConnectionFailed"; httpStatusCode: number }
  | { type: "unauthorized" }
  | { type: "sandboxError" }
  | { type: "internalServerError" }
  | { type: "other" };
```

## 9. 典型消息流程

### 9.1 創建 Thread 並發送消息

```
# 1. 初始化
Client → {"id":0, "method":"initialize", "params":{"clientInfo":{"name":"my-client"}}}
Server → {"id":0, "result":{"userAgent":"Codex/1.0"}}
Client → {"method":"initialized"}

# 2. 創建 Thread
Client → {"id":1, "method":"thread/start", "params":{}}
Server → {"id":1, "result":{"threadId":"abc-123"}}
Server → {"method":"thread/started", "params":{"threadId":"abc-123","thread":{...}}}

# 3. 發送消息
Client → {"id":2, "method":"turn/start", "params":{"threadId":"abc-123","prompt":"Hello"}}
Server → {"id":2, "result":{"turnId":"turn-1"}}
Server → {"method":"turn/started", "params":{"threadId":"abc-123","turn":{...}}}
Server → {"method":"item/started", "params":{"threadId":"abc-123","turnId":"turn-1","item":{"type":"agentMessage",...}}}
Server → {"method":"item/agentMessage/delta", "params":{"delta":"Hi there!"}}
Server → {"method":"item/completed", "params":{"threadId":"abc-123","turnId":"turn-1","item":{...}}}
Server → {"method":"turn/completed", "params":{"threadId":"abc-123","turnId":"turn-1","status":"completed"}}
```

### 9.2 恢復 Thread

```
Client → {"id":3, "method":"thread/resume", "params":{"threadId":"abc-123"}}
Server → {"id":3, "result":{"thread":{...}}}  # 包含完整歷史
```

## 10. 配置覆蓋層級

解析順序（後者覆蓋前者）：
1. 系統默認
2. 用戶配置 (`~/.config/codex/config.toml`)
3. CLI 參數
4. `thread/start` params
5. `turn/start` params

可覆蓋的配置：
- `model`
- `model_provider_id`
- `cwd`
- `approval_policy`
- `sandbox_policy`
- `reasoning_effort`
- `personality`

## 11. Schema 生成

```bash
# 生成 JSON Schema
codex app-server generate-json-schema --out /path/to/output

# 生成 TypeScript 類型
codex app-server generate-ts --out /path/to/output
```

已生成的 schema 位於：
- `/home/rick/codex-source/codex-rs/app-server-protocol/schema/json/v2/`

## 12. 與 Feishu Bridge 整合要點

### 12.1 Session 映射

```
Feishu Chat ID  →  Codex Thread ID
oc_xxx123       →  abc-123-def-456
```

### 12.2 消息轉發

1. **Feishu → Codex**
   - 收到飛書消息 → `turn/start` with prompt

2. **Codex → Feishu**
   - `item/agentMessage/delta` → 累積文本
   - `turn/completed` → 發送最終響應到飛書

### 12.3 流式更新（可選）

- 可以在收到 delta 時更新飛書消息
- 或等待 `turn/completed` 後一次性發送

### 12.4 Approval 處理

- 對於 Feishu 場景，可以自動批准或禁用需要批准的操作
- 使用 `--full-auto` 或設置 `approval_policy`

---

## 參考

- 源碼：`/home/rick/codex-source/codex-rs/app-server-protocol/`
- Schema：`/home/rick/codex-source/codex-rs/app-server-protocol/schema/json/v2/`
- 測試：`/home/rick/codex-source/codex-rs/app-server/tests/suite/v2/`
