package aliyun

import (
	"github.com/alibaba/kubeskoop/pkg/skoop/context"
	"github.com/spf13/pflag"
)

type ProviderConfig struct {
	AccessKeyID     string
	AccessKeySecret string
	SecurityToken   string
}

var Config = &ProviderConfig{}

func init() {
	context.RegisterConfigBinder("Aliyun provider", Config)
}

func (pc *ProviderConfig) BindFlags(fs *pflag.FlagSet) {
	fs.StringVarP(&pc.AccessKeyID, "aliyun-access-key-id", "", "", "Aliyun access key.")
	fs.StringVarP(&pc.AccessKeySecret, "aliyun-access-key-secret", "", "", "Aliyun access secret.")
	fs.StringVarP(&pc.SecurityToken, "aliyun-security-token", "", "", "Aliyun security token (optional).")
}

func (pc *ProviderConfig) Validate() error {
	return nil
}
