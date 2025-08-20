package main

import (
	"context"
	"fmt"
	"strings"
	"sync"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/sirupsen/logrus"
)

// TelegramBot represents the Telegram bot instance
type TelegramBot struct {
	bot           *tgbotapi.BotAPI
	authCode      string
	vpnManager    *VPNManager
	logger        *logrus.Logger
	authorizedUsers map[int64]VPNStatus
	userMutex     sync.RWMutex
}

// Command constants
const (
	CommandStart         = "/start"
	CommandAuth          = "/auth"
	CommandStatus        = "📊 Status"
	CommandEnableVPN     = "🔒 Default to VPN"
	CommandDisableVPN    = "🌐 Default to Direct"
	CommandCancel        = "❌ Cancel"
)

// NewTelegramBot creates a new Telegram bot instance
func NewTelegramBot(token, authCode string, vpnManager *VPNManager, logger *logrus.Logger) (*TelegramBot, error) {
	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, fmt.Errorf("failed to create bot: %w", err)
	}

	bot.Debug = false

	return &TelegramBot{
		bot:             bot,
		authCode:        authCode,
		vpnManager:      vpnManager,
		logger:          logger,
		authorizedUsers: make(map[int64]VPNStatus),
	}, nil
}

// Start starts the Telegram bot
func (tb *TelegramBot) Start(ctx context.Context) error {
	tb.logger.WithField("username", tb.bot.Self.UserName).Info("Telegram bot started")

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := tb.bot.GetUpdatesChan(u)

	for {
		select {
		case update := <-updates:
			tb.handleUpdate(update)
		case <-ctx.Done():
			tb.logger.Info("Telegram bot shutting down")
			tb.bot.StopReceivingUpdates()
			return nil
		}
	}
}

// handleUpdate processes incoming updates
func (tb *TelegramBot) handleUpdate(update tgbotapi.Update) {
	if update.Message == nil {
		return
	}

	userID := update.Message.From.ID
	username := update.Message.From.UserName
	text := update.Message.Text

	tb.logger.WithFields(logrus.Fields{
		"user_id":  userID,
		"username": username,
		"text":     text,
	}).Debug("Received message")

	// Handle commands and messages
	switch {
	case strings.HasPrefix(text, CommandStart):
		tb.handleStart(update.Message)
	case strings.HasPrefix(text, CommandAuth):
		tb.handleAuth(update.Message)
	case tb.isUserAuthorized(userID):
		tb.handleAuthorizedCommand(update.Message)
	default:
		tb.sendUnauthorizedMessage(update.Message.Chat.ID)
	}
}

// handleStart handles the /start command
func (tb *TelegramBot) handleStart(message *tgbotapi.Message) {
	welcomeText := `🤖 Welcome to VPN Commander Bot!

This bot allows you to control VPN routing on your Xkeen router.

To get started, please authenticate using:
/auth YOUR_CODE

Available commands after authentication:
📊 Status - Check current VPN status
🔒 Default to VPN - Route traffic through VPN by default
🌐 Default to Direct - Route traffic directly by default

Security note: This bot requires authentication for all operations.`

	msg := tgbotapi.NewMessage(message.Chat.ID, welcomeText)
	tb.sendMessage(msg)
}

// handleAuth handles the /auth command
func (tb *TelegramBot) handleAuth(message *tgbotapi.Message) {
	args := strings.Fields(message.Text)
	if len(args) != 2 {
		msg := tgbotapi.NewMessage(message.Chat.ID, "❌ Please provide the authentication code: /auth YOUR_CODE")
		tb.sendMessage(msg)
		return
	}

	providedCode := args[1]
	if providedCode == tb.authCode {
		// Check current VPN status and authorize user with this status
		currentStatus, err := tb.vpnManager.GetStatus()
		if err != nil {
			tb.logger.WithError(err).Error("Failed to get initial VPN status during auth")
			currentStatus = VPNStatusUnknown
		}
		
		tb.authorizeUser(message.From.ID, currentStatus)
		
		var statusText string
		switch currentStatus {
		case VPNStatusEnabled:
			statusText = "🔒 Default routing: VPN"
		case VPNStatusDisabled:
			statusText = "🌐 Default routing: DIRECT"
		default:
			statusText = "❓ Default routing: UNKNOWN"
		}
		
		responseText := fmt.Sprintf("✅ Authentication successful!\n\n%s\n\nYou now have access to VPN controls.", statusText)
		msg := tgbotapi.NewMessage(message.Chat.ID, responseText)
		msg.ReplyMarkup = tb.createMainKeyboard()
		tb.sendMessage(msg)
		
		tb.logger.WithFields(logrus.Fields{
			"user_id":     message.From.ID,
			"username":    message.From.UserName,
			"vpn_status":  currentStatus,
		}).Info("User authenticated successfully with initial VPN status")
	} else {
		msg := tgbotapi.NewMessage(message.Chat.ID, "❌ Invalid authentication code. Access denied.")
		tb.sendMessage(msg)
		
		tb.logger.WithFields(logrus.Fields{
			"user_id":  message.From.ID,
			"username": message.From.UserName,
		}).Warn("Authentication failed - invalid code")
	}
}

// handleAuthorizedCommand handles commands from authorized users
func (tb *TelegramBot) handleAuthorizedCommand(message *tgbotapi.Message) {
	switch message.Text {
	case CommandStatus:
		tb.handleStatus(message)
	case CommandEnableVPN:
		tb.handleEnableVPN(message)
	case CommandDisableVPN:
		tb.handleDisableVPN(message)
	default:
		msg := tgbotapi.NewMessage(message.Chat.ID, "❓ Unknown command. Please use the keyboard buttons.")
		msg.ReplyMarkup = tb.createMainKeyboard()
		tb.sendMessage(msg)
	}
}

// handleStatus checks and displays current VPN status
func (tb *TelegramBot) handleStatus(message *tgbotapi.Message) {
	tb.logger.WithField("user_id", message.From.ID).Info("Status check requested")
	
	userID := message.From.ID
	
	// Get cached status first
	cachedStatus := tb.getCachedStatus(userID)
	
	msg := tgbotapi.NewMessage(message.Chat.ID, "🔍 Checking current routing status...")
	tb.sendMessage(msg)

	status, err := tb.vpnManager.GetStatus()
	if err != nil {
		tb.logger.WithError(err).Error("Failed to get VPN status")
		
		// Use cached status if available
		if cachedStatus != VPNStatusUnknown {
			status = cachedStatus
			tb.logger.WithField("cached_status", cachedStatus).Warn("Using cached status due to error")
		} else {
			errorMsg := tgbotapi.NewMessage(message.Chat.ID, 
				fmt.Sprintf("❌ Failed to check status: %v", err))
			errorMsg.ReplyMarkup = tb.createMainKeyboard()
			tb.sendMessage(errorMsg)
			return
		}
	} else {
		// Update cached status
		tb.updateCachedStatus(userID, status)
	}

	var statusText string
	var statusIcon string
	
	switch status {
	case VPNStatusEnabled:
		statusIcon = "🔒"
		statusText = "Default to VPN - Traffic routes through VPN by default"
	case VPNStatusDisabled:
		statusIcon = "🌐"
		statusText = "Default to Direct - Traffic routes directly by default"
	default:
		statusIcon = "❓"
		statusText = "Unknown routing status"
	}

	responseText := fmt.Sprintf("%s Current Status: %s\n\nLast checked: %s", 
		statusIcon, statusText, message.Time().Format("15:04:05"))
	
	statusMsg := tgbotapi.NewMessage(message.Chat.ID, responseText)
	statusMsg.ReplyMarkup = tb.createMainKeyboard()
	tb.sendMessage(statusMsg)
}

// handleEnableVPN enables VPN routing
func (tb *TelegramBot) handleEnableVPN(message *tgbotapi.Message) {
	tb.logger.WithField("user_id", message.From.ID).Info("VPN enable requested")
	
	msg := tgbotapi.NewMessage(message.Chat.ID, "🔄 Setting default routing to VPN...")
	tb.sendMessage(msg)

	if err := tb.vpnManager.EnableVPN(); err != nil {
		tb.logger.WithError(err).Error("Failed to enable VPN")
		errorMsg := tgbotapi.NewMessage(message.Chat.ID, 
			fmt.Sprintf("❌ Failed to enable VPN: %v", err))
		errorMsg.ReplyMarkup = tb.createMainKeyboard()
		tb.sendMessage(errorMsg)
		return
	}

	// Update cached status
	tb.updateCachedStatus(message.From.ID, VPNStatusEnabled)

	successMsg := tgbotapi.NewMessage(message.Chat.ID, 
		"✅ Configuration updated successfully!\n🔒 Current status: Default to VPN\n\nTraffic will now route through VPN by default.")
	successMsg.ReplyMarkup = tb.createMainKeyboard()
	tb.sendMessage(successMsg)
}

// handleDisableVPN disables VPN routing
func (tb *TelegramBot) handleDisableVPN(message *tgbotapi.Message) {
	tb.logger.WithField("user_id", message.From.ID).Info("VPN disable requested")
	
	msg := tgbotapi.NewMessage(message.Chat.ID, "🔄 Setting default routing to Direct...")
	tb.sendMessage(msg)

	if err := tb.vpnManager.DisableVPN(); err != nil {
		tb.logger.WithError(err).Error("Failed to disable VPN")
		errorMsg := tgbotapi.NewMessage(message.Chat.ID, 
			fmt.Sprintf("❌ Failed to disable VPN: %v", err))
		errorMsg.ReplyMarkup = tb.createMainKeyboard()
		tb.sendMessage(errorMsg)
		return
	}

	// Update cached status
	tb.updateCachedStatus(message.From.ID, VPNStatusDisabled)

	successMsg := tgbotapi.NewMessage(message.Chat.ID, 
		"✅ Configuration updated successfully!\n🌐 Current status: Default to Direct\n\nTraffic will now route directly by default.")
	successMsg.ReplyMarkup = tb.createMainKeyboard()
	tb.sendMessage(successMsg)
}

// authorizeUser adds a user to the authorized users list with initial VPN status
func (tb *TelegramBot) authorizeUser(userID int64, initialStatus VPNStatus) {
	tb.userMutex.Lock()
	defer tb.userMutex.Unlock()
	tb.authorizedUsers[userID] = initialStatus
}

// isUserAuthorized checks if a user is authorized
func (tb *TelegramBot) isUserAuthorized(userID int64) bool {
	tb.userMutex.RLock()
	defer tb.userMutex.RUnlock()
	_, exists := tb.authorizedUsers[userID]
	return exists
}

// getCachedStatus gets the cached VPN status for a user
func (tb *TelegramBot) getCachedStatus(userID int64) VPNStatus {
	tb.userMutex.RLock()
	defer tb.userMutex.RUnlock()
	if status, exists := tb.authorizedUsers[userID]; exists {
		return status
	}
	return VPNStatusUnknown
}

// updateCachedStatus updates the cached VPN status for a user
func (tb *TelegramBot) updateCachedStatus(userID int64, status VPNStatus) {
	tb.userMutex.Lock()
	defer tb.userMutex.Unlock()
	if _, exists := tb.authorizedUsers[userID]; exists {
		tb.authorizedUsers[userID] = status
	}
}

// sendUnauthorizedMessage sends an unauthorized access message
func (tb *TelegramBot) sendUnauthorizedMessage(chatID int64) {
	text := "🚫 Unauthorized access. Please authenticate first using /auth YOUR_CODE"
	msg := tgbotapi.NewMessage(chatID, text)
	tb.sendMessage(msg)
}

// createMainKeyboard creates the main keyboard with VPN control buttons
func (tb *TelegramBot) createMainKeyboard() tgbotapi.ReplyKeyboardMarkup {
	return tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton(CommandStatus),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton(CommandEnableVPN),
			tgbotapi.NewKeyboardButton(CommandDisableVPN),
		),
	)
}

// sendMessage sends a message and logs any errors
func (tb *TelegramBot) sendMessage(msg tgbotapi.MessageConfig) {
	if _, err := tb.bot.Send(msg); err != nil {
		tb.logger.WithFields(logrus.Fields{
			"chat_id": msg.ChatID,
			"text":    msg.Text,
			"error":   err,
		}).Error("Failed to send message")
	}
}

// GetBotInfo returns information about the bot
func (tb *TelegramBot) GetBotInfo() *tgbotapi.User {
	return &tb.bot.Self
}