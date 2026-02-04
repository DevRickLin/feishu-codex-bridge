package domain

import "fmt"

// Member represents a chat member (value object)
type Member struct {
	UserID string
	Name   string
}

// FormatMention formats the @ mention syntax
func (m *Member) FormatMention() string {
	return fmt.Sprintf("[MENTION:%s:%s]", m.UserID, m.Name)
}

// FormatDisplay formats for display
func (m *Member) FormatDisplay() string {
	return fmt.Sprintf("%s (user_id: %s)", m.Name, m.UserID)
}
