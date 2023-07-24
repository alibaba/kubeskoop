package context

import (
	"fmt"
	"strings"

	"github.com/spf13/pflag"
	"golang.org/x/exp/slices"
)

type UIConfig struct {
	Format      string
	Output      string
	HTTP        bool
	HTTPAddress string
	HTTPPort    uint
}

var (
	supportedFormat = []string{"d2", "svg", "json"}
)

func (c *UIConfig) BindFlags(fs *pflag.FlagSet) {
	fs.StringVarP(&c.Format, "format", "", "", "Output format of diagnose result, support d2/svg/json. If not set, only print simple path info on console.")
	fs.StringVarP(&c.Output, "output", "", "", "Output file name, default is output.d2/svg/json in current work directory.")
	fs.BoolVarP(&c.HTTP, "http", "", false, "Enable an http server to show diagnose result.")
	fs.StringVarP(&c.HTTPAddress, "http-address", "", "127.0.0.1:8080", "Listen address for http server.")
}

func (c *UIConfig) Validate() error {
	if c.Format != "" && !slices.Contains(supportedFormat, c.Format) {
		return fmt.Errorf("unsupported output format %q, should be %s", c.Format, strings.Join(supportedFormat, ","))
	}
	return nil
}

func (c *Context) UIConfig() *UIConfig {
	uiConfig, _ := c.Ctx.Load(uiConfigKey)
	return uiConfig.(*UIConfig)
}
