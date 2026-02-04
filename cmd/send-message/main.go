package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
)

func main() {
	appID := os.Getenv("FEISHU_APP_ID")
	appSecret := os.Getenv("FEISHU_APP_SECRET")

	if appID == "" || appSecret == "" {
		fmt.Println("Error: FEISHU_APP_ID and FEISHU_APP_SECRET must be set")
		os.Exit(1)
	}

	if len(os.Args) < 4 {
		fmt.Println("Usage: send-message <chat_id> <user_id> <message>")
		os.Exit(1)
	}

	chatID := os.Args[1]
	userID := os.Args[2]
	message := os.Args[3]

	// Create Lark client
	client := lark.NewClient(appID, appSecret)

	// Build message with mention
	mentionText := fmt.Sprintf("<at user_id=\"%s\">@User</at> %s", userID, message)
	content := map[string]string{"text": mentionText}
	contentJSON, _ := json.Marshal(content)

	// Send message
	req := larkim.NewCreateMessageReqBuilder().
		ReceiveIdType(larkim.ReceiveIdTypeChatId).
		Body(larkim.NewCreateMessageReqBodyBuilder().
			ReceiveId(chatID).
			MsgType(larkim.MsgTypeText).
			Content(string(contentJSON)).
			Build()).
		Build()

	resp, err := client.Im.Message.Create(context.Background(), req)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
	if !resp.Success() {
		fmt.Printf("Error: %s\n", resp.Msg)
		os.Exit(1)
	}

	fmt.Println("Message sent successfully!")
}
