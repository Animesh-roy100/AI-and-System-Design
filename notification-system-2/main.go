package main

import (
	"context"
	"errors"
	"time"
)

// Notification Schema ----------------------------------------
type NotificationStatus string

const (
	StatusPending   NotificationStatus = "PENDING"
	StatusScheduled NotificationStatus = "SCHEDULED"
	StatusQueued    NotificationStatus = "QUEUED"
	StatusSent      NotificationStatus = "SENT"
	StatusDelivered NotificationStatus = "DELIVERED"
	StatusFailed    NotificationStatus = "FAILED"
)

type ChannelType string

const (
	ChannelSMS   ChannelType = "SMS"
	ChannelEmail ChannelType = "EMAIL"
	ChannelInApp ChannelType = "IN_APP"
	ChannelPush  ChannelType = "PUSH"
)

// Represents the universal history table for notifications
type Notification struct {
	ID             string             `gorm:"primaryKey;type:uuid;default:uuid_generate_v4()"`
	UserID         string             `gorm:"index"`
	IdempotencyKey string             `gorm:"uniqueIndex"` // Prevent duplicate sends
	Channel        ChannelType        `gorm:"index"`
	Status         NotificationStatus `gorm:"index"`

	// Content
	TemplateID *string
	Title      string
	Body       string `gorm:"type:text"`

	// Metadata/Vendor Info
	ProviderID   string // e.g., Twilio Message SID for webhooks
	ErrorMessage *string

	// Timing
	ScheduledFor *time.Time `gorm:"index"` // Useful for finding delayed tasks
	ReadAt       *time.Time // For in-app read receipts
	CreatedAt    time.Time  `gorm:"index"`
	UpdatedAt    time.Time
}

// Represents user opt-ins/opt-outs securely
type UserPreference struct {
	UserID        string  `gorm:"primaryKey"`
	EmailOptIn    bool    `gorm:"default:true"`
	SMSOptIn      bool    `gorm:"default:true"`
	PushOptIn     bool    `gorm:"default:true"`
	QuietHoursIn  *string // e.g., "22:00"
	QuietHoursOut *string // e.g., "08:00"
	UpdatedAt     time.Time
}

// Represent templates that the Marketing team can tweak without code changes
type Template struct {
	ID          string `gorm:"primaryKey"`
	Name        string
	Subject     string // For emails
	Body        string `gorm:"type:text"` // Can contain Go html/template vars like {{.Name}}
	ChannelType string
	CreatedAt   time.Time
}

// Buidler Pattern (Message Construction) -----------------------
type NotificationMessage struct {
	UserID      string
	Title       string
	Body        string
	Channels    []string
	ScheduledAt *time.Time
}

type MessageBuilder struct {
	msg NotificationMessage
}

func NewMessageBuilder() *MessageBuilder {
	return &MessageBuilder{}
}

func (b *MessageBuilder) SetUser(uid string) *MessageBuilder {
	b.msg.UserID = uid
	return b
}

func (b *MessageBuilder) SetContent(title, body string) *MessageBuilder {
	b.msg.Title = title
	b.msg.Body = body
	return b
}

func (b *MessageBuilder) AddChannel(channel string) *MessageBuilder {
	b.msg.Channels = append(b.msg.Channels, channel)
	return b
}

func (b *MessageBuilder) Build() NotificationMessage {
	return b.msg
}

// Usage:
// msg := NewMessageBuilder().SetUser("u123").SetContent("Hello", "World").AddChannel("SMS").Build()

// Strategy Pattern (Sender Selection) -------------------------
type NotificationSender interface {
	Send(ctx context.Context, msg NotificationMessage) error
	Supports() string // e.g., "SMS", "EMAIL"
}

// Concrete Implementations
type TwilioSMSSender struct {
	// client *twilio.Client
}

func (t *TwilioSMSSender) Send(ctx context.Context, msg NotificationMessage) error {
	// Talk to Twilio API
	return nil
}
func (t *TwilioSMSSender) Supports() string { return "SMS" }

type SendGridEmailSender struct {
	// client *sendgrid.Client
}

func (s *SendGridEmailSender) Send(ctx context.Context, msg NotificationMessage) error {
	// Talk to Sendgrid API
	return nil
}
func (s *SendGridEmailSender) Supports() string { return "EMAIL" }

// Factory Pattern (Sender Factory) -------------------------------
type SenderFactory struct {
	senders map[string]NotificationSender
}

func NewSenderFactory(senders ...NotificationSender) *SenderFactory {
	sf := &SenderFactory{senders: make(map[string]NotificationSender)}
	for _, s := range senders {
		sf.senders[s.Supports()] = s
	}
	return sf
}

func (sf *SenderFactory) GetSender(channel string) (NotificationSender, error) {
	if sender, ok := sf.senders[channel]; ok {
		return sender, nil
	}
	return nil, errors.New("unsupported channel")
}
