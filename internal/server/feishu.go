package server

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/anthropics/feishu-codex-bridge/feishu"
	"github.com/anthropics/feishu-codex-bridge/internal/biz/domain"
	"github.com/anthropics/feishu-codex-bridge/internal/biz/repo"
	"github.com/anthropics/feishu-codex-bridge/internal/biz/usecase"
	"github.com/anthropics/feishu-codex-bridge/internal/service"
)

// FeishuServer handles Feishu message processing
type FeishuServer struct {
	feishuClient *feishu.Client
	messageRepo  repo.MessageRepo
	convSvc      *service.ConversationService
	bufferUC     *usecase.BufferUsecase
	scheduler    *service.DigestScheduler

	// Message deduplication cache
	seenMsgsMu sync.RWMutex
	seenMsgs   map[string]time.Time // msgID -> timestamp
}

// NewFeishuServer creates a new Feishu server
func NewFeishuServer(
	feishuClient *feishu.Client,
	messageRepo repo.MessageRepo,
	convSvc *service.ConversationService,
	bufferUC *usecase.BufferUsecase,
	codexRepo repo.CodexRepo,
	filterUC *usecase.FilterUsecase,
) *FeishuServer {
	s := &FeishuServer{
		feishuClient: feishuClient,
		messageRepo:  messageRepo,
		convSvc:      convSvc,
		bufferUC:     bufferUC,
		seenMsgs:     make(map[string]time.Time),
	}

	// Set reply callback
	convSvc.SetReplyCallback(s.sendReply)

	// Create digest scheduler (1 hour interval)
	// Pass codexRepo and filterUC to enable Codex smart digest + Moonshot filtering
	if bufferUC != nil {
		s.scheduler = service.NewDigestScheduler(
			bufferUC,
			codexRepo,
			filterUC,
			convSvc,
			1*time.Hour,
		)
	}

	return s
}

// Start starts the server
func (s *FeishuServer) Start() error {
	// Start event loop
	s.convSvc.StartEventLoop()

	// Start digest scheduler
	if s.scheduler != nil {
		s.scheduler.Start(context.Background())
	}

	// Set message handler and start Feishu client
	s.feishuClient.OnMessage(s.handleMessage)
	return s.feishuClient.Start()
}

// Stop stops the server
func (s *FeishuServer) Stop() {
	if s.scheduler != nil {
		s.scheduler.Stop()
	}
	s.feishuClient.Stop()
}

// handleMessage handles Feishu messages
func (s *FeishuServer) handleMessage(msg *feishu.Message) {
	fmt.Printf("[Server] Received %s from %s (chatType=%s): %s\n",
		msg.MsgType, msg.ChatID, msg.ChatType, truncate(msg.Content, 50))

	// Message deduplication: check if already processed
	if s.isMessageSeen(msg.MsgID) {
		fmt.Printf("[Server] Duplicate message ignored: %s\n", msg.MsgID)
		return
	}
	s.markMessageSeen(msg.MsgID)

	ctx := context.Background()

	// Convert chat type
	chatType := domain.ChatTypeP2P
	if msg.ChatType == "group" {
		chatType = domain.ChatTypeGroup
	}

	// Get sender name
	senderName := ""
	senderID := ""
	if msg.Sender != nil {
		senderID = msg.Sender.SenderID
		// Try to get name from member list
		members, err := s.messageRepo.GetChatMembers(ctx, msg.ChatID)
		if err == nil {
			for _, m := range members {
				if m.UserID == senderID {
					senderName = m.Name
					break
				}
			}
		}
	}

	// Check if should process immediately (group chat uses Buffer system)
	if chatType == domain.ChatTypeGroup && s.bufferUC != nil {
		shouldProcess, reason := s.bufferUC.ShouldProcessImmediately(ctx, msg.ChatID, msg.Content, msg.MentionsBot)
		if !shouldProcess {
			// Add to buffer
			bufferedMsg := &domain.BufferedMessage{
				ChatID:     msg.ChatID,
				MsgID:      msg.MsgID,
				Content:    msg.Content,
				SenderID:   senderID,
				SenderName: senderName,
				CreatedAt:  time.Now(),
			}
			if err := s.bufferUC.AddToBuffer(ctx, bufferedMsg); err != nil {
				fmt.Printf("[Server] Failed to buffer message: %v\n", err)
			} else {
				fmt.Printf("[Server] Message buffered for later digest\n")
			}
			return
		}
		fmt.Printf("[Server] Processing immediately: %s\n", reason)
	}

	// Download images
	var imagePaths []string
	for _, imageKey := range msg.ImageKeys {
		path, err := s.feishuClient.DownloadImage(msg.MsgID, imageKey)
		if err != nil {
			fmt.Printf("[Server] Failed to download image %s: %v\n", imageKey, err)
			continue
		}
		imagePaths = append(imagePaths, path)
	}

	// Build request
	req := &service.MessageRequest{
		ChatID:        msg.ChatID,
		MsgID:         msg.MsgID,
		Content:       msg.Content,
		SenderID:      senderID,
		SenderName:    senderName,
		ChatType:      chatType,
		MentionsBot:   msg.MentionsBot,
		ImagePaths:    imagePaths,
		MsgCreateTime: msg.CreateTime,
	}

	// Process message
	if err := s.convSvc.HandleMessage(ctx, req); err != nil {
		if err.Error() == "already processing" {
			_ = s.messageRepo.SendText(ctx, msg.ChatID, "Processing previous request, please wait...")
		} else {
			fmt.Printf("[Server] Handle message error: %v\n", err)
		}
	}
}

// sendReply sends a reply
func (s *FeishuServer) sendReply(chatID, msgID, text string, mentions []domain.Member) {
	ctx := context.Background()

	if len(mentions) > 0 {
		err := s.messageRepo.SendTextWithMentions(ctx, chatID, text, mentions)
		if err != nil {
			fmt.Printf("[Server] Failed to send reply with mentions: %v\n", err)
			// Fallback to plain message
			_ = s.messageRepo.SendText(ctx, chatID, text)
		}
	} else {
		err := s.messageRepo.SendText(ctx, chatID, text)
		if err != nil {
			fmt.Printf("[Server] Failed to send reply: %v\n", err)
		}
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// isMessageSeen checks if a message has been processed
func (s *FeishuServer) isMessageSeen(msgID string) bool {
	s.seenMsgsMu.RLock()
	defer s.seenMsgsMu.RUnlock()
	_, exists := s.seenMsgs[msgID]
	return exists
}

// markMessageSeen marks a message as processed
func (s *FeishuServer) markMessageSeen(msgID string) {
	s.seenMsgsMu.Lock()
	defer s.seenMsgsMu.Unlock()
	s.seenMsgs[msgID] = time.Now()

	// Clean up expired message records (older than 5 minutes)
	// Clean up when marking new messages to prevent memory leaks
	cutoff := time.Now().Add(-5 * time.Minute)
	for id, ts := range s.seenMsgs {
		if ts.Before(cutoff) {
			delete(s.seenMsgs, id)
		}
	}
}
