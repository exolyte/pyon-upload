package main

import (
	"fmt"
	"net/http"

	"github.com/spf13/viper"
)

var configLocation string = "/etc/pyon-files"

func getConfig() {
	viper.SetConfigName("conf")
	viper.SetConfigType("toml")
	viper.AddConfigPath(configLocation)

	viper.BindEnv("AWSId", "PYON_AWS_ACCESS_ID")
	viper.BindEnv("AWSKey", "PYON_AWS_ACCESS_KEY")

	err := viper.ReadInConfig()
	if err != nil {
		panic(fmt.Errorf("fatal error config file: %w", err))
	}
	if !viper.IsSet("port") {
		panic(fmt.Errorf("port was not set"))
	}
	if !viper.IsSet("AWSKey") || !viper.IsSet("AWSId") {
		panic(fmt.Errorf("PYON_AWS_ACCESS_ID or PYON_AWS_ACCESS_KEY environment variables were not set"))
	}
}

func main() {
	getConfig()

	http.ListenAndServe(fmt.Sprintf(":%d", viper.GetInt("port")), nil)
}
