package service

import (
	"bytes"
	"crypto/md5"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
)

func timestampMS() string {
	return time.Now().Format("20060102150405000")
}

func smsSign(ts string) string {
	raw := setting.SmsEnterpriseNo + setting.SmsAccount + ts + setting.SmsSignKey
	return fmt.Sprintf("%X", md5.Sum([]byte(raw)))
}

// smsPost 向短信平台发送 POST 请求，path 为相对路径（如 /json/submit）
func smsPost(path string, payload any) (map[string]any, error) {
	setting.LoadSmsSetting()
	baseUrl := strings.TrimRight(setting.SmsApiUrl, "/")
	if baseUrl == "" {
		return nil, fmt.Errorf("短信服务未配置")
	}

	body, err := common.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("sms marshal failed: %w", err)
	}

	req, err := http.NewRequest("POST", baseUrl+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	res, err := GetHttpClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	var result map[string]any
	if err := common.DecodeJson(res.Body, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// smsBasePayload 构造带签名的基础请求参数
func smsBasePayload() map[string]string {
	ts := timestampMS()
	return map[string]string{
		"enterprise_no": setting.SmsEnterpriseNo,
		"account":       setting.SmsAccount,
		"timestamp":     ts,
		"sign":          smsSign(ts),
	}
}

// ========== SendSms ==========

func SendSms(phone string, content string) error {
	setting.LoadSmsSetting()

	payload := smsBasePayload()
	payload["phones"] = phone
	payload["content"] = content
	payload["subcode"] = ""
	payload["sendtime"] = ""
	payload["countrycode"] = ""

	result, err := smsPost("/json/submit", payload)
	if err != nil {
		return err
	}
	if r, ok := result["result"].(string); !ok || r != "0" {
		desc, _ := result["desc"].(string)
		return fmt.Errorf("sms send failed: %s", desc)
	}
	return nil
}

// ========== QuerySmsBalance ==========

// SmsBalanceResult 短信余额查询结果
type SmsBalanceResult struct {
	Balance string `json:"balance"` // 余额（元）
	Sended  string `json:"sended"`  // 已发送条数
	Sale    string `json:"sale"`    // 消费金额（元）
}

// QuerySmsBalance 查询短信余额
func QuerySmsBalance() (*SmsBalanceResult, error) {
	setting.LoadSmsSetting()

	payload := smsBasePayload()
	result, err := smsPost("/json/balance", payload)
	if err != nil {
		return nil, err
	}
	if r, ok := result["result"].(string); !ok || r != "0" {
		desc, _ := result["desc"].(string)
		return nil, fmt.Errorf("query balance failed: %s", desc)
	}

	balance := &SmsBalanceResult{}
	if v, ok := result["balance"].(string); ok {
		balance.Balance = v
	}
	if v, ok := result["sended"].(string); ok {
		balance.Sended = v
	}
	if v, ok := result["sale"].(string); ok {
		balance.Sale = v
	}
	return balance, nil
}

// ========== QuerySmsReport ==========

// SmsReportItem 单条发送状态报告
type SmsReportItem struct {
	Result       string `json:"result"`       // success / faild
	Phone        string `json:"phone"`        // 手机号码
	ReportTime   string `json:"report_time"`  // 回执时间
	CountryCode  string `json:"countrycode"`  // 国家代码
	NetwayCode   string `json:"netway_code"`  // 发送端口
	MsgId        string `json:"msgid"`        // 消息标识
	Seq          string `json:"seq"`          // 消息序列
	Mcc          string `json:"mcc"`          // 移动国家代码
	Mnc          string `json:"mnc"`          // 移动网络代码
	Cost         string `json:"cost"`         // 短信成本（元）
	Status       string `json:"status"`       // 状态码
}

// QuerySmsReport 查询短信发送状态报告
func QuerySmsReport() ([]SmsReportItem, error) {
	setting.LoadSmsSetting()

	payload := smsBasePayload()
	result, err := smsPost("/json/report", payload)
	if err != nil {
		return nil, err
	}
	if r, ok := result["result"].(string); !ok || r != "0" {
		desc, _ := result["desc"].(string)
		return nil, fmt.Errorf("query report failed: %s", desc)
	}

	var items []SmsReportItem
	if reportRaw, ok := result["report"]; ok {
		reportBytes, err := common.Marshal(reportRaw)
		if err != nil {
			return nil, err
		}
		if err := common.Unmarshal(reportBytes, &items); err != nil {
			return nil, err
		}
	}
	return items, nil
}
