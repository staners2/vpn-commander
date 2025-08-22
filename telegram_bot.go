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
	bot             *tgbotapi.BotAPI
	authCode        string
	vpnManager      *VPNManager
	logger          *logrus.Logger
	authorizedUsers map[int64]VPNStatus
	userMutex       sync.RWMutex
	lastMessages    map[int64]int    // userID -> last bot message ID for editing
	lastMsgType     map[int64]string // userID -> last message type 
	lastUserMsg     map[int64]int    // userID -> last user command message ID
	messageMutex    sync.RWMutex
}

// Command constants
const (
	CommandStart         = "/start"
	CommandAuth          = "/auth"
	CommandStatus        = "🔍 Quick Status"
	CommandEnableVPN     = "🔐 Route via VPN"
	CommandDisableVPN    = "🔓 Route Direct"
	CommandStartVPN      = "🟢 Start VPN"
	CommandStopVPN       = "🔴 Stop VPN"
	CommandServiceStatus = "🔋 Service Status"
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
		lastMessages:    make(map[int64]int),
		lastMsgType:     make(map[int64]string),
		lastUserMsg:     make(map[int64]int),
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
	welcomeText := `🚀 **VPN Commander Bot**

🎯 **What this bot controls:**
• 🔋 **VPN Service** - Start/stop the VPN daemon
• 🔐 **Traffic Routing** - Choose VPN tunnel or direct internet

🔐 **Authentication required**
Send: /auth YOUR_CODE

📋 **Available controls after authentication:**
🔍 Quick Status - Check current traffic routing
🔋 Service Status - Check if VPN daemon is running
🔐 Route via VPN - Send traffic through secure tunnel
🔓 Route Direct - Send traffic directly to internet
🟢 Start VPN - Power on the VPN service
🔴 Stop VPN - Power off the VPN service

💡 **Pro tip:** Check status first, then choose your routing preference!`

	msg := tgbotapi.NewMessage(message.Chat.ID, welcomeText)
	msg.ParseMode = "Markdown"
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
			statusText = "🔐 Current routing: VPN TUNNEL"
		case VPNStatusDisabled:
			statusText = "🔓 Current routing: DIRECT"
		default:
			statusText = "❓ Current routing: UNKNOWN"
		}
		
		responseText := fmt.Sprintf("✅ **Authentication successful!**\n\n%s\n\n🎛️ You now have access to VPN controls.", statusText)
		msg := tgbotapi.NewMessage(message.Chat.ID, responseText)
		msg.ParseMode = "Markdown"
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
	// Store user message ID for deletion
	tb.storeUserMessageID(message.From.ID, message.MessageID)
	
	switch message.Text {
	case CommandStatus:
		tb.handleStatus(message)
	case CommandEnableVPN:
		tb.handleEnableVPN(message)
	case CommandDisableVPN:
		tb.handleDisableVPN(message)
	case CommandStartVPN:
		tb.handleStartVPN(message)
	case CommandStopVPN:
		tb.handleStopVPN(message)
	case CommandServiceStatus:
		tb.handleServiceStatus(message)
	default:
		// Delete user command message for unknown commands too
		tb.deleteUserMessage(message.Chat.ID, message.MessageID)
		msg := tgbotapi.NewMessage(message.Chat.ID, "❓ Unknown command. Please use the keyboard buttons.")
		msg.ReplyMarkup = tb.createMainKeyboard()
		tb.sendMessage(msg)
	}
}

// handleStatus checks and displays current VPN status
func (tb *TelegramBot) handleStatus(message *tgbotapi.Message) {
	tb.logger.WithField("user_id", message.From.ID).Info("Status check requested")
	
	userID := message.From.ID
	
	// Send progressive message
	msgID := tb.sendProgressiveMessage(message.Chat.ID, "🔍 Checking traffic routing status...", "vpn_status", message.MessageID)
	
	cachedStatus := tb.getCachedStatus(userID)
	status, err := tb.vpnManager.GetStatus()
	
	if err != nil {
		tb.logger.WithError(err).Error("Failed to get VPN status")
		
		// Use cached status if available
		if cachedStatus != VPNStatusUnknown {
			status = cachedStatus
			tb.logger.WithField("cached_status", cachedStatus).Warn("Using cached status due to error")
		} else {
			tb.updateProgressiveMessage(message.Chat.ID, msgID, "❌ Status check failed")
			return
		}
	} else {
		// Update cached status
		tb.updateCachedStatus(userID, status)
	}

	var responseText string
	switch status {
	case VPNStatusEnabled:
		responseText = "🔐 **VPN ROUTING ACTIVE**\n↳ All traffic routes through VPN tunnel\n📊 Checked at " + message.Time().Format("15:04")
	case VPNStatusDisabled:
		responseText = "🔓 **DIRECT ROUTING ACTIVE**\n↳ Traffic goes directly to internet\n📊 Checked at " + message.Time().Format("15:04")
	default:
		responseText = "❓ **ROUTING STATUS UNKNOWN** • " + message.Time().Format("15:04")
	}
	
	tb.updateProgressiveMessage(message.Chat.ID, msgID, responseText)
}

// handleEnableVPN enables VPN routing
func (tb *TelegramBot) handleEnableVPN(message *tgbotapi.Message) {
	tb.logger.WithField("user_id", message.From.ID).Info("VPN enable requested")
	
	// Delete user command message
	tb.deleteUserMessage(message.Chat.ID, message.MessageID)

	if err := tb.vpnManager.EnableVPN(); err != nil {
		tb.logger.WithError(err).Error("Failed to enable VPN")
		errorMsg := tgbotapi.NewMessage(message.Chat.ID, "❌ Failed to enable VPN")
		errorMsg.ReplyMarkup = tb.createMainKeyboard()
		tb.sendMessage(errorMsg)
		return
	}

	// Update cached status
	tb.updateCachedStatus(message.From.ID, VPNStatusEnabled)

	tb.sendOrEditMessage(message.Chat.ID, "✅ **ROUTING SWITCHED TO VPN**\n🔐 Traffic now flows through secure tunnel\n⚡ Applied instantly", tb.createMainKeyboard())
}

// handleDisableVPN disables VPN routing
func (tb *TelegramBot) handleDisableVPN(message *tgbotapi.Message) {
	tb.logger.WithField("user_id", message.From.ID).Info("VPN disable requested")
	
	// Delete user command message
	tb.deleteUserMessage(message.Chat.ID, message.MessageID)

	if err := tb.vpnManager.DisableVPN(); err != nil {
		tb.logger.WithError(err).Error("Failed to disable VPN")
		errorMsg := tgbotapi.NewMessage(message.Chat.ID, "❌ Failed to disable VPN")
		errorMsg.ReplyMarkup = tb.createMainKeyboard()
		tb.sendMessage(errorMsg)
		return
	}

	// Update cached status
	tb.updateCachedStatus(message.From.ID, VPNStatusDisabled)

	tb.sendOrEditMessage(message.Chat.ID, "✅ **ROUTING SWITCHED TO DIRECT**\n🔓 Traffic now goes directly to internet\n⚡ Applied instantly", tb.createMainKeyboard())
}

// handleStartVPN starts the VPN service using xkeen
func (tb *TelegramBot) handleStartVPN(message *tgbotapi.Message) {
	tb.logger.WithField("user_id", message.From.ID).Info("VPN service start requested")
	
	// Delete user command message
	tb.deleteUserMessage(message.Chat.ID, message.MessageID)

	if err := tb.vpnManager.StartVPNService(); err != nil {
		tb.logger.WithError(err).Error("Failed to start VPN service")
		errorMsg := tgbotapi.NewMessage(message.Chat.ID, "❌ Failed to start service")
		errorMsg.ReplyMarkup = tb.createMainKeyboard()
		tb.sendMessage(errorMsg)
		return
	}

	tb.sendOrEditMessage(message.Chat.ID, "✅ **VPN SERVICE STARTED**\n🟢 Daemon is now running and ready\n⚙️ Service initialized", tb.createMainKeyboard())
}

// handleStopVPN stops the VPN service using xkeen
func (tb *TelegramBot) handleStopVPN(message *tgbotapi.Message) {
	tb.logger.WithField("user_id", message.From.ID).Info("VPN service stop requested")
	
	// Delete user command message
	tb.deleteUserMessage(message.Chat.ID, message.MessageID)

	if err := tb.vpnManager.StopVPNService(); err != nil {
		tb.logger.WithError(err).Error("Failed to stop VPN service")
		errorMsg := tgbotapi.NewMessage(message.Chat.ID, "❌ Failed to stop service")
		errorMsg.ReplyMarkup = tb.createMainKeyboard()
		tb.sendMessage(errorMsg)
		return
	}

	tb.sendOrEditMessage(message.Chat.ID, "✅ **VPN SERVICE STOPPED**\n🔴 Daemon has been shut down\n⚙️ Service terminated", tb.createMainKeyboard())
}

// handleServiceStatus checks and displays VPN service status using xkeen
func (tb *TelegramBot) handleServiceStatus(message *tgbotapi.Message) {
	tb.logger.WithField("user_id", message.From.ID).Info("VPN service status check requested")

	// Send progressive message
	msgID := tb.sendProgressiveMessage(message.Chat.ID, "🔋 Checking VPN daemon status...", "service_status", message.MessageID)

	status, err := tb.vpnManager.GetVPNServiceStatus()
	if err != nil {
		tb.logger.WithError(err).Error("Failed to get VPN service status")
		tb.updateProgressiveMessage(message.Chat.ID, msgID, "❌ Service status check failed")
		return
	}

	// Clean status text from ANSI color codes and extra whitespace
	cleanStatus := strings.ReplaceAll(status, "\033[31m", "")
	cleanStatus = strings.ReplaceAll(cleanStatus, "\033[0m", "")
	cleanStatus = strings.ReplaceAll(cleanStatus, "[31m", "")
	cleanStatus = strings.ReplaceAll(cleanStatus, "[0m", "")
	cleanStatus = strings.TrimSpace(cleanStatus)
	
	// Determine status with simple logic
	var responseText string
	if strings.Contains(cleanStatus, "не запущен") {
		responseText = "🔴 **VPN SERVICE STOPPED**\n↳ Daemon is not running\n🔋 Checked at " + message.Time().Format("15:04")
		tb.logger.WithField("decision", "not running - found 'не запущен'").Info("Status decision")
	} else if strings.Contains(cleanStatus, "запущен") || cleanStatus != "" {
		responseText = "🟢 **VPN SERVICE RUNNING**\n↳ Daemon is active and ready\n🔋 Checked at " + message.Time().Format("15:04")
		tb.logger.WithField("decision", "running - found service active").Info("Status decision")
	} else {
		responseText = "🟡 **VPN SERVICE STATUS UNKNOWN** • " + message.Time().Format("15:04")
		tb.logger.WithField("decision", "unknown - empty output after cleaning").Info("Status decision")
	}
	
	tb.updateProgressiveMessage(message.Chat.ID, msgID, responseText)
}

// getCombinedStatus returns a combined status display showing both routing and service status
func (tb *TelegramBot) getCombinedStatus() (string, error) {
	// Get routing status
	routingStatus, err := tb.vpnManager.GetStatus()
	if err != nil {
		tb.logger.WithError(err).Warn("Failed to get routing status for combined display")
		routingStatus = VPNStatusUnknown
	}

	// Get service status
	serviceStatusRaw, err := tb.vpnManager.GetVPNServiceStatus()
	var serviceRunning bool
	if err != nil {
		tb.logger.WithError(err).Warn("Failed to get service status for combined display")
		serviceRunning = false
	} else {
		// Clean and check service status
		cleanStatus := strings.ReplaceAll(serviceStatusRaw, "\033[31m", "")
		cleanStatus = strings.ReplaceAll(cleanStatus, "\033[0m", "")
		cleanStatus = strings.TrimSpace(cleanStatus)
		serviceRunning = !strings.Contains(cleanStatus, "не запущен")
	}

	// Build combined status message
	var routingIcon, serviceIcon string
	switch routingStatus {
	case VPNStatusEnabled:
		routingIcon = "🔒"
	case VPNStatusDisabled:
		routingIcon = "🌐"
	default:
		routingIcon = "❓"
	}

	if serviceRunning {
		serviceIcon = "🟢"
	} else {
		serviceIcon = "🔴"
	}

	return fmt.Sprintf("%s%s Combined Status", routingIcon, serviceIcon), nil
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

// createMainKeyboard creates reply keyboard with VPN control buttons grouped by functionality
func (tb *TelegramBot) createMainKeyboard() tgbotapi.ReplyKeyboardMarkup {
	return tgbotapi.NewReplyKeyboard(
		// Status monitoring group
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton(CommandStatus),
			tgbotapi.NewKeyboardButton(CommandServiceStatus),
		),
		// VPN routing configuration group  
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton(CommandEnableVPN),
			tgbotapi.NewKeyboardButton(CommandDisableVPN),
		),
		// Service control group
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton(CommandStartVPN),
			tgbotapi.NewKeyboardButton(CommandStopVPN),
		),
	)
}

// sendProgressiveMessage sends a progressive message that can be edited through process stages
func (tb *TelegramBot) sendProgressiveMessage(chatID int64, initialText string, msgType string, userMsgID int) int {
	tb.messageMutex.Lock()
	defer tb.messageMutex.Unlock()
	
	userID := chatID
	lastMessageID, exists := tb.lastMessages[userID]
	lastType, typeExists := tb.lastMsgType[userID]
	
	// Delete previous bot message if it was the same type
	if exists && lastMessageID > 0 && typeExists && lastType == msgType {
		deleteMsg := tgbotapi.NewDeleteMessage(chatID, lastMessageID)
		tb.bot.Send(deleteMsg) // Don't care about errors here
	}
	
	// Delete user command message
	if userMsgID > 0 {
		deleteUserMsg := tgbotapi.NewDeleteMessage(chatID, userMsgID)
		tb.bot.Send(deleteUserMsg) // Don't care about errors here
	}
	
	// Send new message
	msg := tgbotapi.NewMessage(chatID, initialText)
	// Don't add keyboard to processing message
	
	if sentMsg, err := tb.bot.Send(msg); err != nil {
		tb.logger.WithError(err).Error("Failed to send progressive message")
		return 0
	} else {
		// Store new message ID and type
		tb.lastMessages[chatID] = sentMsg.MessageID
		tb.lastMsgType[chatID] = msgType
		return sentMsg.MessageID
	}
}

// updateProgressiveMessage edits an existing message text only (ReplyKeyboard can't be edited)
func (tb *TelegramBot) updateProgressiveMessage(chatID int64, messageID int, finalText string) {
	if messageID == 0 {
		return
	}
	
	// Edit message text only with markdown support
	editMsg := tgbotapi.NewEditMessageText(chatID, messageID, finalText)
	editMsg.ParseMode = "Markdown"
	if _, err := tb.bot.Send(editMsg); err != nil {
		tb.logger.WithError(err).Debug("Failed to edit message text")
		// If edit fails, send new message as fallback
		tb.sendStatusMessageWithMarkdown(chatID, finalText, "fallback")
	}
}


// sendStatusMessage sends a status message and deletes previous status message (fallback)
func (tb *TelegramBot) sendStatusMessage(chatID int64, text string, msgType string) {
	tb.messageMutex.Lock()
	defer tb.messageMutex.Unlock()
	
	userID := chatID
	lastMessageID, exists := tb.lastMessages[userID]
	lastType, typeExists := tb.lastMsgType[userID]
	
	// Send new message
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ReplyMarkup = tb.createMainKeyboard()
	
	if sentMsg, err := tb.bot.Send(msg); err != nil {
		tb.logger.WithError(err).Error("Failed to send status message")
		return
	} else {
		// Store new message ID and type
		tb.lastMessages[chatID] = sentMsg.MessageID
		tb.lastMsgType[chatID] = msgType
		
		// Delete previous bot message if it was the same type
		if exists && lastMessageID > 0 && typeExists && lastType == msgType {
			deleteMsg := tgbotapi.NewDeleteMessage(chatID, lastMessageID)
			if _, err := tb.bot.Send(deleteMsg); err != nil {
				tb.logger.WithError(err).Debug("Failed to delete previous bot message")
			}
		}
	}
}

// sendStatusMessageWithMarkdown sends a status message with markdown support
func (tb *TelegramBot) sendStatusMessageWithMarkdown(chatID int64, text string, msgType string) {
	tb.messageMutex.Lock()
	defer tb.messageMutex.Unlock()
	
	userID := chatID
	lastMessageID, exists := tb.lastMessages[userID]
	lastType, typeExists := tb.lastMsgType[userID]
	
	// Send new message
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = tb.createMainKeyboard()
	
	if sentMsg, err := tb.bot.Send(msg); err != nil {
		tb.logger.WithError(err).Error("Failed to send status message with markdown")
		return
	} else {
		// Store new message ID and type
		tb.lastMessages[chatID] = sentMsg.MessageID
		tb.lastMsgType[chatID] = msgType
		
		// Delete previous bot message if it was the same type
		if exists && lastMessageID > 0 && typeExists && lastType == msgType {
			deleteMsg := tgbotapi.NewDeleteMessage(chatID, lastMessageID)
			if _, err := tb.bot.Send(deleteMsg); err != nil {
				tb.logger.WithError(err).Debug("Failed to delete previous bot message")
			}
		}
	}
}

// sendOrEditMessage sends a new message and deletes the previous one (legacy)
func (tb *TelegramBot) sendOrEditMessage(chatID int64, text string, keyboard tgbotapi.ReplyKeyboardMarkup) {
	tb.sendStatusMessageWithMarkdown(chatID, text, "status")
}

// sendNewMessage sends a new message and stores its ID
func (tb *TelegramBot) sendNewMessage(chatID int64, text string, keyboard tgbotapi.ReplyKeyboardMarkup) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ReplyMarkup = keyboard
	
	maxRetries := 3
	for attempt := 1; attempt <= maxRetries; attempt++ {
		if sentMsg, err := tb.bot.Send(msg); err != nil {
			tb.logger.WithFields(logrus.Fields{
				"chat_id": chatID,
				"text":    text,
				"error":   err,
				"attempt": attempt,
			}).Warn("Failed to send message")
			
			if attempt < maxRetries {
				continue
			}
		} else {
			// Store message ID for future editing
			tb.lastMessages[chatID] = sentMsg.MessageID
			return
		}
	}
}

// sendMessage sends a message and logs any errors with retry logic (legacy function)
func (tb *TelegramBot) sendMessage(msg tgbotapi.MessageConfig) {
	maxRetries := 3
	for attempt := 1; attempt <= maxRetries; attempt++ {
		if sentMsg, err := tb.bot.Send(msg); err != nil {
			tb.logger.WithFields(logrus.Fields{
				"chat_id": msg.ChatID,
				"text":    msg.Text,
				"error":   err,
				"attempt": attempt,
			}).Warn("Failed to send message")
			
			if attempt < maxRetries {
				continue
			}
		} else {
			// Store message ID for editing if it's a status-type message
			tb.messageMutex.Lock()
			tb.lastMessages[msg.ChatID] = sentMsg.MessageID
			tb.messageMutex.Unlock()
			return
		}
	}
}

// storeUserMessageID stores the user's message ID for later deletion
func (tb *TelegramBot) storeUserMessageID(userID int64, messageID int) {
	tb.messageMutex.Lock()
	defer tb.messageMutex.Unlock()
	tb.lastUserMsg[userID] = messageID
}

// deleteUserMessage deletes a user's message
func (tb *TelegramBot) deleteUserMessage(chatID int64, messageID int) {
	if messageID > 0 {
		deleteMsg := tgbotapi.NewDeleteMessage(chatID, messageID)
		if _, err := tb.bot.Send(deleteMsg); err != nil {
			tb.logger.WithError(err).Debug("Failed to delete user message")
		}
	}
}

// GetBotInfo returns information about the bot
func (tb *TelegramBot) GetBotInfo() *tgbotapi.User {
	return &tb.bot.Self
}