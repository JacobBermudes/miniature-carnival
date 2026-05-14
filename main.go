package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/gin-gonic/gin"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/redis/go-redis/v9"
)

var (
	bot       *tgbotapi.BotAPI
	channelID string
	rdb       *redis.Client
	ctx       = context.Background()
)

type UserSession struct {
	UserID int64 `json:"user_id"`
	ChatID int64 `json:"chat_id"`
}

func main() {
	botToken := os.Getenv("TELEGRAM_BOT_TOKEN")
	secretToken := os.Getenv("TELEGRAM_SECRET_TOKEN")
	channelID = os.Getenv("TG_PUBLIC")
	channelURL := os.Getenv("TG_CHANNEL_URL")
	externalAPIKey := os.Getenv("EXTERNAL_API_KEY")
	redisAddr := os.Getenv("REDIS_URL")

	rdb = redis.NewClient(&redis.Options{
		Addr: redisAddr,
	})

	var err error
	bot, err = tgbotapi.NewBotAPI(botToken)
	if err != nil {
		log.Panic(err)
	}

	r := gin.Default()

	r.POST("/api/send_message", func(c *gin.Context) {
		if c.GetHeader("X-API-Key") != externalAPIKey {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}

		var req struct {
			ChatID int64  `json:"chat_id"`
			Text   string `json:"text"`
		}

		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
			return
		}

		msg := tgbotapi.NewMessage(req.ChatID, req.Text)
		_, err := bot.Send(msg)

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"status": "sent"})
	})

	r.GET("/api/check", func(c *gin.Context) {
		if c.GetHeader("X-API-Key") != externalAPIKey {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}

		userCode := c.Query("code")

		val, err := rdb.Get(ctx, userCode).Result()
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Code not found"})
			return
		}

		var session UserSession
		json.Unmarshal([]byte(val), &session)

		isSubscribed, _ := isUserSubscribed(session.UserID)

		c.JSON(http.StatusOK, gin.H{
			"is_subscribed": isSubscribed,
			"tg_user_id":    session.UserID,
			"chat_id":       session.ChatID,
		})
	})

	r.POST("/webhook", func(c *gin.Context) {
		if c.GetHeader("X-Telegram-Bot-Api-Secret-Token") != secretToken {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}

		var update tgbotapi.Update
		if err := c.ShouldBindJSON(&update); err != nil {
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})

		if update.Message != nil && update.Message.IsCommand() && update.Message.Command() == "start" {
			param := update.Message.CommandArguments()
			userID := update.Message.From.ID

			if param != "" {
				session := UserSession{
					UserID: userID,
					ChatID: update.Message.Chat.ID,
				}

				sessionData, _ := json.Marshal(session)

				err := rdb.Set(ctx, param, sessionData, 0).Err()
				if err != nil {
					log.Printf("Redis Error: %v", err)
				}
			}

			keyboard := tgbotapi.NewInlineKeyboardMarkup(
				tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonURL("📢 Подписаться", channelURL)),
				tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData("✅ Я подписался", "check_sub")),
			)

			msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Добро пожаловать! Чтобы бесплатно использовать VPN, подпишитесь на наш канал.")
			msg.ReplyMarkup = keyboard
			bot.Send(msg)
		}

		if update.CallbackQuery != nil {
			bot.Request(tgbotapi.NewCallback(update.CallbackQuery.ID, ""))

			if update.CallbackQuery.Data == "check_sub" {
				userID := update.CallbackQuery.From.ID
				chatID := update.CallbackQuery.Message.Chat.ID

				isSubscribed, err := isUserSubscribed(userID)
				if err != nil {
					bot.Send(tgbotapi.NewMessage(chatID, "Ошибка сервера при проверке."))
					return
				}

				if isSubscribed {
					editMsg := tgbotapi.NewEditMessageText(chatID, update.CallbackQuery.Message.MessageID, "🎉 Спасибо! Вы подписаны. Теперь вы можете использовать VPN.")
					bot.Send(editMsg)
				} else {
					alert := tgbotapi.NewCallbackWithAlert(update.CallbackQuery.ID, "Вы еще не подписались!")
					bot.Request(alert)
				}
			}
		}
	})

	log.Fatal(r.Run(":8080"))
}

func isUserSubscribed(userID int64) (bool, error) {
	var memberConfig tgbotapi.GetChatMemberConfig

	if numericChatID, err := strconv.ParseInt(channelID, 10, 64); err == nil {
		memberConfig = tgbotapi.GetChatMemberConfig{
			ChatConfigWithUser: tgbotapi.ChatConfigWithUser{ChatID: numericChatID, UserID: userID},
		}
	} else {
		memberConfig = tgbotapi.GetChatMemberConfig{
			ChatConfigWithUser: tgbotapi.ChatConfigWithUser{SuperGroupUsername: channelID, UserID: userID},
		}
	}

	chatMember, err := bot.GetChatMember(memberConfig)
	if err != nil {
		return false, err
	}

	isSub := chatMember.Status == "member" || chatMember.Status == "administrator" || chatMember.Status == "creator"
	return isSub, nil
}
