package controller

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

// ========== SendSmsCode ==========

type SendSmsCodeRequest struct {
	Phone string `json:"phone"`
}

func SendSmsCode(c *gin.Context) {
	var req SendSmsCodeRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	phone := strings.TrimSpace(req.Phone)
	if phone == "" {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}

	// 检查用户是否存在
	var user model.User
	if err := model.DB.Where("username = ?", phone).First(&user).Error; err != nil {
		common.ApiErrorI18n(c, i18n.MsgUserNotExists)
		return
	}
	if user.Status != common.UserStatusEnabled {
		common.ApiErrorI18n(c, i18n.MsgAuthUserBanned)
		return
	}

	// 加载 SMS 配置
	setting.LoadSmsSetting()
	if setting.SmsApiUrl == "" {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "短信服务未配置",
		})
		return
	}

	// 失败熔断检查
	if common.RedisEnabled {
		ctx := c.Request.Context()
		failKey := "smsFail:" + phone
		failVal, err := common.RDB.Get(ctx, failKey).Result()
		failCount := 0
		if err == nil {
			fmt.Sscanf(failVal, "%d", &failCount)
		}
		if failCount >= 3 {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "发送失败次数过多，请稍后再试",
			})
			return
		}
	}

	// 生成验证码
	code := common.GenerateNumericCode(6)
	common.RegisterVerificationCodeWithKey(phone, code, common.SmsLoginPurpose)

	// 发送短信
	content := fmt.Sprintf("您的验证码：%s，%d分钟内有效。", code, common.VerificationValidMinutes)
	if err := service.SendSms(phone, content); err != nil {
		// 记录失败
		if common.RedisEnabled {
			ctx := c.Request.Context()
			failKey := "smsFail:" + phone
			newCount, _ := common.RDB.Incr(ctx, failKey).Result()
			if newCount == 1 {
				common.RDB.Expire(ctx, failKey, 5*time.Minute)
			}
		}
		common.SysLog(fmt.Sprintf("SMS send failed to %s: %v", phone, err))
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "短信发送失败，请稍后重试",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

// ========== QuerySmsBalance ==========

// QuerySmsBalance 查询短信余额（余额、已发送条数、消费金额）
func QuerySmsBalance(c *gin.Context) {
	result, err := service.QuerySmsBalance()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}

// ========== QuerySmsReport ==========

// QuerySmsReport 查询短信发送状态报告
func QuerySmsReport(c *gin.Context) {
	items, err := service.QuerySmsReport()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    items,
	})
}

// ========== SmsLogin ==========

type SmsLoginRequest struct {
	Phone string `json:"phone"`
	Code  string `json:"code"`
}

func SmsLogin(c *gin.Context) {
	var req SmsLoginRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	phone := strings.TrimSpace(req.Phone)
	code := strings.TrimSpace(req.Code)
	if phone == "" || code == "" {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}

	// 验证校验码
	if !common.VerifyCodeWithKey(phone, code, common.SmsLoginPurpose) {
		common.ApiErrorI18n(c, i18n.MsgUserVerificationCodeError)
		return
	}
	// 验证成功后立即失效验证码，防止重放
	common.DeleteKey(phone, common.SmsLoginPurpose)

	// 查询用户
	var user model.User
	err := model.DB.Where("username = ?", phone).First(&user).Error
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgUserNotExists)
		return
	}

	if user.Status != common.UserStatusEnabled {
		common.ApiErrorI18n(c, i18n.MsgAuthUserBanned)
		return
	}

	// 建立 session（内联 setupLogin 逻辑以支持 need_set_password）
	model.UpdateUserLastLoginAt(user.Id)
	session := sessions.Default(c)
	session.Set("id", user.Id)
	session.Set("username", user.Username)
	session.Set("role", user.Role)
	session.Set("status", user.Status)
	session.Set("group", user.Group)
	if err := session.Save(); err != nil {
		common.ApiErrorI18n(c, i18n.MsgUserSessionSaveFailed)
		return
	}
	recordLoginAudit(&user, c)

	// 判断是否需要设置密码
	needSetPassword := user.Password == ""

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": map[string]any{
			"need_set_password": needSetPassword,
			"id":                user.Id,
			"username":          user.Username,
			"display_name":      user.DisplayName,
			"role":              user.Role,
			"status":            user.Status,
			"group":             user.Group,
		},
	})
}

// ========== QuickRedeem ==========

type QuickRedeemRequest struct {
	Phone string `json:"phone"`
	Key   string `json:"key"`
}

func QuickRedeem(c *gin.Context) {
	var req QuickRedeemRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	phone := strings.TrimSpace(req.Phone)
	key := strings.TrimSpace(req.Key)
	if phone == "" || key == "" {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}

	// Step 1: 先校验兑换码，有效才继续（避免创建无效用户）
	var redemption model.Redemption
	keyCol := "`key`"
	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		keyCol = `"key"`
	}
	if err := model.DB.Where(keyCol+" = ?", key).First(&redemption).Error; err != nil {
		common.ApiErrorI18n(c, "无效的兑换码")
		return
	}
	if redemption.Status != common.RedemptionCodeStatusEnabled {
		common.ApiErrorI18n(c, "该兑换码已被使用")
		return
	}
	if redemption.ExpiredTime != 0 && redemption.ExpiredTime < common.GetTimestamp() {
		common.ApiErrorI18n(c, "该兑换码已过期")
		return
	}

	// Step 2: 从 context 获取管理员 ID（由 AdminAuth 中间件设置）
	creatorId := c.GetInt("id")

	// Step 3: 查询或创建用户（使用 Unscoped 以查找软删除用户）
	var user model.User
	err := model.DB.Unscoped().Where("username = ?", phone).First(&user).Error
	userExisted := err == nil
	if userExisted && user.DeletedAt.Valid {
		// 用户被软删除，仍视为已存在，后续 redeem 仍可使用 user.Id
	} else if !userExisted {
		// 创建新用户
		user = model.User{
			Username:    phone,
			Password:    "", // 空密码，只能通过短信登录
			DisplayName: phone,
			Phone:       phone,
			CreatorId:   creatorId,
			Role:        common.RoleCommonUser,
		}
		if err := user.Insert(0); err != nil {
			common.ApiError(c, err)
			return
		}

		// 生成默认 token（best-effort：失败不影响用户创建和兑换）
		if constant.GenerateDefaultToken {
			tokenKey, err := common.GenerateKey()
			if err != nil {
				common.SysLog("QuickRedeem: failed to generate token key for user " + phone + ": " + err.Error())
			} else {
				token := model.Token{
					UserId:             user.Id,
					Name:               phone + "的初始令牌",
					Key:                tokenKey,
					CreatedTime:        common.GetTimestamp(),
					AccessedTime:       common.GetTimestamp(),
					ExpiredTime:        -1,
					RemainQuota:        500000,
					UnlimitedQuota:     true,
					ModelLimitsEnabled: false,
				}
				if setting.DefaultUseAutoGroup {
					token.Group = "auto"
				}
				if err := token.Insert(); err != nil {
					common.SysLog("QuickRedeem: failed to create default token for user " + phone + ": " + err.Error())
				}
			}
		}
	}

	// Step 4: 执行兑换（带行锁，兜底并发安全）
	quota, err := model.Redeem(key, user.Id)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": map[string]any{
			"user_existed": userExisted,
			"username":     phone,
			"quota":        quota,
		},
	})
}
