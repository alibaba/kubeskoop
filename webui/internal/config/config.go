package config

import (
	"github.com/samber/lo"
	"log"
	"os"
)

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

type AuthConfig struct {
	Username string `json:"username"`
	Password string `json:"password"`
	JWTKey   string `json:"jwt_key"`
}

type Config struct {
	Grafana    GrafanaConfig    `json:"grafana"`
	Controller ControllerConfig `json:"controller"`
	Auth       AuthConfig       `json:"auth"`
	StaticDir  string           `json:"web"`
}

func init() {
	err := initConfig()
	if err != nil {
		panic(err)
	}
}

func initConfig() error {
	var ok bool
	Global.Grafana.Endpoint, _ = os.LookupEnv("GRAFANA_ENDPOINT")
	Global.Grafana.Proxy, _ = readBoolFromEnvironment("GRAFANA_PROXY")
	Global.Grafana.Username, _ = os.LookupEnv("GRAFANA_USERNAME")
	Global.Grafana.Password, _ = os.LookupEnv("GRAFANA_PASSWORD")
	Global.Controller.Endpoint, _ = os.LookupEnv("CONTROLLER_ENDPOINT")
	Global.StaticDir, ok = os.LookupEnv("STATIC_DIR")
	if !ok || Global.StaticDir == "" {
		Global.StaticDir = "/var/www"
	}
	Global.Auth.Username, _ = os.LookupEnv("AUTH_USERNAME")
	Global.Auth.Password, _ = os.LookupEnv("AUTH_PASSWORD")
	Global.Auth.JWTKey, ok = os.LookupEnv("AUTH_JWT_KEY")
	if !ok || Global.Auth.JWTKey == "" {
		log.Println("AUTH_JWT_KEY is not set, using random value.")
		Global.Auth.JWTKey = lo.RandomString(32, lo.AlphanumericCharset)
	}
	return nil
}

func readBoolFromEnvironment(env string) (bool, bool) {
	e, ok := os.LookupEnv(env)
	return e == "true", ok
}
