package service

import (
	"crypto/md5"
	"fmt"
	"net/http"
	"net/url"
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
	return fmt.Sprintf("%x", md5.Sum([]byte(raw)))
}

func SendSms(phone string, content string) error {
	setting.LoadSmsSetting()
	ts := timestampMS()
	sign := smsSign(ts)

	form := url.Values{
		"enterprise_no": {setting.SmsEnterpriseNo},
		"account":       {setting.SmsAccount},
		"phones":        {phone},
		"content":       {content},
		"timestamp":     {ts},
		"sign":          {sign},
		"subcode":       {""},
		"sendtime":      {""},
		"countrycode":   {""},
	}

	req, err := http.NewRequest("POST", setting.SmsApiUrl, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	res, err := GetHttpClient().Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	var result struct {
		Result string `json:"result"`
		Desc   string `json:"desc"`
	}
	if err := common.DecodeJson(res.Body, &result); err != nil {
		return err
	}
	if result.Result != "0" {
		return fmt.Errorf("sms send failed: %s", result.Desc)
	}
	return nil
}
