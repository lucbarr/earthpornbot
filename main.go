package main

import (
	"fmt"

	"github.com/lucbarr/earthpornbot/api"
	"github.com/spf13/viper"
)

func main() {
	err := setupConfig()
	if err != nil {
		panic(err)
	}
	reddit := api.NewReddit()
	err = reddit.Authenticate()
	if err != nil {
		panic(err)
	}

	err = reddit.FetchSubmissions()
	fmt.Println(err)
}

func setupConfig() error {
	viper.SetConfigName("default")
	viper.AddConfigPath(".")
	viper.SetConfigType("yaml")
	return viper.ReadInConfig()
}
