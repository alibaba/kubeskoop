package context

import (
	"github.com/spf13/pflag"
)

type MiscConfig struct {
	Version bool
}

func (m *MiscConfig) BindFlags(fs *pflag.FlagSet) {
	fs.BoolVarP(&m.Version, "version", "", false, "Show version.")
}

func (m *MiscConfig) Validate() error {
	return nil
}

func (c *Context) MiscConfig() *MiscConfig {
	miscConfig, _ := c.Ctx.Load(miscConfigKey)
	return miscConfig.(*MiscConfig)
}
