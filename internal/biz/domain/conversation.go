package domain

import "time"

// ChatType represents the chat type
type ChatType string

const (
	ChatTypeGroup ChatType = "group"
	ChatTypeP2P   ChatType = "p2p"
)

// Conversation represents the conversation aggregate root
type Conversation struct {
	ChatID   string
	ChatType ChatType
	Members  []Member
	History  []Message
	Current  *Message
}

// HistorySince gets history messages after specified time
func (c *Conversation) HistorySince(t time.Time) []Message {
	var result []Message
	for _, m := range c.History {
		if m.IsAfter(t) {
			result = append(result, m)
		}
	}
	return result
}

// HistoryExcludingCurrent gets history excluding current message
func (c *Conversation) HistoryExcludingCurrent() []Message {
	if c.Current == nil || len(c.History) == 0 {
		return c.History
	}

	var result []Message
	for _, m := range c.History {
		if m.ID != c.Current.ID {
			result = append(result, m)
		}
	}
	return result
}

// HistorySinceExcludingCurrent gets history after specified time (excluding current message)
func (c *Conversation) HistorySinceExcludingCurrent(t time.Time) []Message {
	var result []Message
	for _, m := range c.History {
		if m.IsAfter(t) && (c.Current == nil || m.ID != c.Current.ID) {
			result = append(result, m)
		}
	}
	return result
}

// HistoryAfterMsgID gets history after specified message ID (excluding current message)
// Falls back to using fallbackTime if msgID is not found
func (c *Conversation) HistoryAfterMsgID(msgID string, fallbackTime time.Time) []Message {
	if msgID == "" {
		// No anchor point, fall back to time-based filtering
		return c.HistorySinceExcludingCurrent(fallbackTime)
	}

	// Find the position of msgID in history
	anchorIdx := -1
	for i, m := range c.History {
		if m.ID == msgID {
			anchorIdx = i
			break
		}
	}

	if anchorIdx == -1 {
		// Anchor message not found (may have been deleted), fall back to time
		return c.HistorySinceExcludingCurrent(fallbackTime)
	}

	// Return messages after anchor (excluding current message)
	var result []Message
	for i := anchorIdx + 1; i < len(c.History); i++ {
		m := c.History[i]
		if c.Current == nil || m.ID != c.Current.ID {
			result = append(result, m)
		}
	}
	return result
}

// FindMemberByID finds a member by ID
func (c *Conversation) FindMemberByID(userID string) *Member {
	for _, m := range c.Members {
		if m.UserID == userID {
			return &m
		}
	}
	return nil
}

// IsGroup checks if this is a group chat
func (c *Conversation) IsGroup() bool {
	return c.ChatType == ChatTypeGroup
}
