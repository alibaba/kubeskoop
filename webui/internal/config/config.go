package config

import "os"

var (
	Global Config
)

type GrafanaConfig struct {
	Endpoint string `json:"endpoint"`
	Proxy    bool   `json:"proxy"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type ControllerConfig struct {
	Endpoint string `json:"endpoint"`
}

type Config struct {
	Grafana    GrafanaConfig    `json:"grafana"`
	Controller ControllerConfig `json:"controller"`
}

func init() {
	err := initConfig()
	if err != nil {
		panic(err)
	}
}

func initConfig() error {
	Global.Grafana.Endpoint, _ = os.LookupEnv("GRAFANA_ENDPOINT")
	Global.Grafana.Proxy, _ = readBoolFromEnvironment("GRAFANA_PROXY")
	Global.Grafana.Username, _ = os.LookupEnv("GRAFANA_USERNAME")
	Global.Grafana.Password, _ = os.LookupEnv("GRAFANA_PASSWORD")
	Global.Controller.Endpoint, _ = os.LookupEnv("CONTROLLER_ENDPOINT")
	return nil
}

func readBoolFromEnvironment(env string) (bool, bool) {
	e, ok := os.LookupEnv(env)
	return e == "true", ok
}
