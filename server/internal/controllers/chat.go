package controllers

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/OneSignal/onesignal-go-api"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
	"github.com/the-clothing-loop/website/server/internal/app"
	"github.com/the-clothing-loop/website/server/internal/app/auth"
	"github.com/the-clothing-loop/website/server/internal/models"
	"github.com/the-clothing-loop/website/server/internal/views"
	ginext "github.com/the-clothing-loop/website/server/pkg/gin_ext"
	"github.com/the-clothing-loop/website/server/sharedtypes"
)

func ChatGetType(c *gin.Context) {
	db := getDB(c)
	var body sharedtypes.ChatGetTypeRequest
	if err := c.BindQuery(&body); err != nil {
		c.String(http.StatusBadRequest, err.Error())
		return
	}
	ok, _, chain := auth.Authenticate(c, db, auth.AuthState2UserOfChain, body.ChainUID)
	if !ok {
		return
	}

	chatTypeUrl, err := chain.GetChatType(db)
	if err != nil {
		ginext.AbortWithErrorInBody(c, http.StatusInternalServerError, err, "Unable to find chat type")
		return
	}

	c.JSON(http.StatusOK, chatTypeUrl)
}

func ChatPatchType(c *gin.Context) {
	db := getDB(c)
	var body sharedtypes.ChatPatchTypeRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.String(http.StatusBadRequest, err.Error())
		return
	}
	ok, _, chain := auth.Authenticate(c, db, auth.AuthState3AdminChainUser, body.ChainUID)
	if !ok {
		return
	}

	err := chain.SaveChatType(db, sharedtypes.ChatGetTypeResponse{
		ChatType:          body.ChatType,
		ChatUrl:           body.ChatUrl,
		ChatInAppDisabled: body.ChatInAppDisabled,
	})
	if err != nil {
		ginext.AbortWithErrorInBody(c, http.StatusInternalServerError, err, "Unable to find chat type")
		return
	}

	c.Status(http.StatusOK)
}

func ChatChannelList(c *gin.Context) {
	db := getDB(c)
	var body sharedtypes.ChatChannelListQuery
	if err := c.ShouldBindQuery(&body); err != nil {
		c.String(http.StatusBadRequest, err.Error())
		return
	}
	ok, _, chain := auth.Authenticate(c, db, auth.AuthState2UserOfChain, body.ChainUID)
	if !ok {
		return
	}

	chatChannelList := []sharedtypes.ChatChannel{}
	err := db.Raw(`SELECT * FROM chat_channels WHERE chain_id = ?`, chain.ID).Scan(&chatChannelList).Error
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	c.JSON(http.StatusOK, sharedtypes.ChatChannelListResponse{List: chatChannelList})
}

func ChatChannelCreate(c *gin.Context) {
	db := getDB(c)
	var body sharedtypes.ChatChannel
	if err := c.ShouldBindJSON(&body); err != nil {
		c.String(http.StatusBadRequest, err.Error())
		return
	}
	ok, _, chain := auth.Authenticate(c, db, auth.AuthState3AdminChainUser, body.ChainUID)
	if !ok {
		return
	}

	body.ChainID = chain.ID
	body.CreatedAt = time.Now().UnixMilli()
	body.ID = 0
	body.ChatMessages = nil
	err := db.Save(&body).Error
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}
	c.Status(http.StatusOK)
}

func ChatChannelEdit(c *gin.Context) {
	db := getDB(c)
	var body sharedtypes.ChatChannelEditRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.String(http.StatusBadRequest, err.Error())
		return
	}
	ok, _, chain := auth.Authenticate(c, db, auth.AuthState3AdminChainUser, body.ChainUID)
	if !ok {
		return
	}

	err := db.Exec(`UPDATE chat_channels SET name = ?, color = ? WHERE id = ? AND chain_id = ?`, body.Name, body.Color, body.ID, chain.ID).Error
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}
	c.Status(http.StatusOK)
}

func ChatChannelMessageList(c *gin.Context) {
	db := getDB(c)
	var body sharedtypes.ChatChannelMessageListQuery
	if err := c.ShouldBindQuery(&body); err != nil {
		c.String(http.StatusBadRequest, err.Error())
		return
	}
	ok, _, chain := auth.Authenticate(c, db, auth.AuthState2UserOfChain, body.ChainUID)
	if !ok {
		return
	}

	chatChannelMessageList := []sharedtypes.ChatMessage{}
	err := db.Debug().Raw(`
SELECT msg.* FROM chat_messages msg
LEFT JOIN chat_channels channel ON channel.id = msg.chat_channel_id AND channel.id = ? AND channel.chain_id = ?
WHERE msg.created_at < ?
LIMIT ?, 20 
`, body.ChatChannelID, chain.ID, body.StartFrom, body.Page*20).Scan(&chatChannelMessageList).Error
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	c.JSON(http.StatusOK, sharedtypes.ChatChannelMessageListResponse{Messages: chatChannelMessageList})
}

func ChatChannelMessageCreate(c *gin.Context) {
	db := getDB(c)
	var body sharedtypes.ChatMessageCreateRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.String(http.StatusBadRequest, err.Error())
		return
	}
	ok, authUser, chain := auth.Authenticate(c, db, auth.AuthState2UserOfChain, body.ChainUID)
	if !ok {
		return
	}

	count := int64(-1)
	db.Raw(`SELECT COUNT(*) FROM chat_channels WHERE id = ? AND chain_id = ? LIMIT 1`, body.ChatChannelID, chain.ID).Count(&count)
	if count <= 0 {
		c.String(http.StatusBadRequest, fmt.Sprintf("chat room %d is not part of this Loop", body.ChatChannelID))
		return
	}

	chatMessage := sharedtypes.ChatMessage{
		Message:       body.Message,
		SendByUID:     authUser.UID,
		ChatChannelID: body.ChatChannelID,
		CreatedAt:     time.Now().UnixMilli(),
	}
	err := db.Save(&chatMessage).Error
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	// send message to one signal
	userUIDs, err := models.UserGetAllApprovedUserUIDsByChain(db, chain.ID)
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}
	userUIDs = lo.Filter(userUIDs, func(uid string, _ int) bool {
		return uid != authUser.UID
	})
	notificationMessage := lo.Ellipsis(body.Message, 10)
	err = app.OneSignalCreateNotification(db, userUIDs, *views.Notifications[views.NotificationEnumTitleChatMessage], onesignal.StringMap{
		En: &notificationMessage,
	})
	if err != nil {
		slog.Error("Unable to send notification", "err", err)
	}

	c.Status(http.StatusOK)
}
