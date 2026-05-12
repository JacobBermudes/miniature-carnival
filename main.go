package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func main() {
	botToken := os.Getenv("TELEGRAM_BOT_TOKEN")
	secretToken := os.Getenv("TELEGRAM_SECRET_TOKEN")
	channelID := os.Getenv("TG_PUBLIC")

	if botToken == "" {
		log.Fatal("TELEGRAM_BOT_TOKEN environment variable is not set")
	}
	if secretToken == "" {
		log.Fatal("TELEGRAM_SECRET_TOKEN environment variable is not set")
	}
	if channelID == "" {
		log.Fatal("TG_PUBLIC environment variable is not set")
	}

	bot, err := tgbotapi.NewBotAPI(botToken)
	if err != nil {
		log.Panic(err)
	}

	r := gin.Default()

	r.POST("/webhook", func(c *gin.Context) {
		headerToken := c.GetHeader("X-Telegram-Bot-Api-Secret-Token")
		if headerToken != secretToken {
			log.Printf("Unauthorized access attempt. Token: %s", headerToken)
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}

		var update tgbotapi.Update
		if err := c.ShouldBindJSON(&update); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"status": "ok"})

		if update.Message == nil {
			return
		}

		userID := update.Message.From.ID
		chatID := update.Message.Chat.ID

		memberConfig := tgbotapi.GetChatMemberConfig{
			ChatConfigWithUser: tgbotapi.ChatConfigWithUser{
				SuperGroupUsername: channelID,
				UserID:             userID,
			},
		}

		chatMember, err := bot.GetChatMember(memberConfig)
		if err != nil {
			log.Printf("Error checking chat member: %v", err)
			sendReply(bot, chatID, "Произошла ошибка при проверке подписки.")
			return
		}

		isSubscribed := chatMember.Status == "member" ||
			chatMember.Status == "administrator" ||
			chatMember.Status == "creator"

		if isSubscribed {
			sendReply(bot, chatID, "Спасибо! Вы подписаны на канал. Доступ разрешен.")
		} else {
			sendReply(bot, chatID, fmt.Sprintf("Вы не подписаны на канал %s. Пожалуйста, подпишитесь, чтобы продолжить.", channelID))
		}
	})

	log.Fatal(r.Run(":8080"))
}

func sendReply(bot *tgbotapi.BotAPI, chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	if _, err := bot.Send(msg); err != nil {
		log.Printf("Failed to send message: %v", err)
	}
}
