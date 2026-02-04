package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

func main() {
	appID := os.Getenv("FEISHU_APP_ID")
	appSecret := os.Getenv("FEISHU_APP_SECRET")
	chatID := "oc_6c6de5b1a6ed3fa1c2189c028387308a" // Test group

	if len(os.Args) > 1 {
		chatID = os.Args[1]
	}

	// 1. Get access token
	token, err := getTenantToken(appID, appSecret)
	if err != nil {
		fmt.Printf("Failed to get token: %v\n", err)
		return
	}
	fmt.Printf("Token: %s...\n\n", token[:20])

	// Test the fix: fetch in descending order then reverse
	fmt.Println("=== Simulating fixed behavior: ByCreateTimeDesc + Reverse ===")
	testGetMessagesWithReverse(token, chatID, 10)
}

func getTenantToken(appID, appSecret string) (string, error) {
	body := fmt.Sprintf(`{"app_id":"%s","app_secret":"%s"}`, appID, appSecret)
	resp, err := http.Post(
		"https://open.feishu.cn/open-apis/auth/v3/tenant_access_token/internal",
		"application/json",
		strings.NewReader(body),
	)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		Code              int    `json:"code"`
		TenantAccessToken string `json:"tenant_access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	return result.TenantAccessToken, nil
}

func testGetMessagesWithReverse(token, chatID string, pageSize int) {
	url := fmt.Sprintf("https://open.feishu.cn/open-apis/im/v1/messages?container_id_type=chat&container_id=%s&page_size=%d&sort_type=ByCreateTimeDesc",
		chatID, pageSize)

	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Printf("Request failed: %v\n", err)
		return
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)

	var result struct {
		Code int `json:"code"`
		Data struct {
			Items []struct {
				MessageID  string `json:"message_id"`
				CreateTime string `json:"create_time"`
				MsgType    string `json:"msg_type"`
				Body       struct {
					Content string `json:"content"`
				} `json:"body"`
			} `json:"items"`
		} `json:"data"`
	}

	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		fmt.Printf("Failed to parse response: %v\n", err)
		return
	}

	// Reverse to chronological order
	items := result.Data.Items
	for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
		items[i], items[j] = items[j], items[i]
	}

	fmt.Printf("Returned %d messages (sorted in chronological order):\n", len(items))
	for i, item := range items {
		// Parse time
		var ts int64
		fmt.Sscanf(item.CreateTime, "%d", &ts)
		t := time.UnixMilli(ts)

		// Parse content
		content := item.Body.Content
		if item.MsgType == "text" {
			var parsed struct {
				Text string `json:"text"`
			}
			json.Unmarshal([]byte(content), &parsed)
			content = parsed.Text
		}
		if len(content) > 50 {
			content = content[:50] + "..."
		}

		fmt.Printf("  %d. [%s] %s: %s\n", i+1, t.Format("15:04:05"), item.MsgType, content)
	}
}
