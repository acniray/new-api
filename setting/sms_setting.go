package setting

import "github.com/QuantumNous/new-api/common"

var (
	SmsApiUrl       string
	SmsEnterpriseNo string
	SmsAccount      string
	SmsSignKey      string
)

func LoadSmsSetting() {
	common.OptionMapRWMutex.RLock()
	defer common.OptionMapRWMutex.RUnlock()
	SmsApiUrl = common.OptionMap["SmsApiUrl"]
	SmsEnterpriseNo = common.OptionMap["SmsEnterpriseNo"]
	SmsAccount = common.OptionMap["SmsAccount"]
	SmsSignKey = common.OptionMap["SmsSignKey"]
}
