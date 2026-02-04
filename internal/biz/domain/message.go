package domain

import "time"

// Message represents a message entity
type Message struct {
	ID         string
	ChatID     string
	Content    string
	SenderID   string
	SenderName string
	MsgType    string // text, image, post, etc.
	CreateTime time.Time
	IsBot      bool // Whether the message was sent by the bot
}

// IsFromBot checks if the message is from the bot
func (m *Message) IsFromBot(botID string) bool {
	return m.SenderID == botID
}

// IsAfter checks if the message is after the specified time
func (m *Message) IsAfter(t time.Time) bool {
	return m.CreateTime.After(t)
}

// IsBefore checks if the message is before the specified time
func (m *Message) IsBefore(t time.Time) bool {
	return m.CreateTime.Before(t)
}
