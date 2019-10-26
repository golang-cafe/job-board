package main

import (
	"fmt"
	"os"
)

type Config struct {
	DatabaseURL string
	AppID       string
	AppKey      string
	Username    string
	Password    string
	Subreddit   string
}

func LoadConfig() (Config, error) {
	redditAppID := os.Getenv("REDDIT_APP_ID")
	if redditAppID == "" {
		return Config{}, fmt.Errorf("REDDIT_APP_ID cannot be empty")
	}
	redditAppKey := os.Getenv("REDDIT_APP_KEY")
	if redditAppKey == "" {
		return Config{}, fmt.Errorf("REDDIT_APP_KEY cannot be empty")
	}
	redditUsername := os.Getenv("REDDIT_USERNAME")
	if redditUsername == "" {
		return Config{}, fmt.Errorf("REDDIT_USERNAME cannot be empty")
	}
	redditPassword := os.Getenv("REDDIT_PASSWORD")
	if redditPassword == "" {
		return Config{}, fmt.Errorf("REDDIT_PASSWORD cannot be empty")
	}
	databaseURL := os.Getenv("HEROKU_POSTGRESQL_PINK_URL")
	if databaseURL == "" {
		return Config{}, fmt.Errorf("HEROKU_POSTGRESQL_PINK_URL cannot be empty")
	}

	return Config{
		DatabaseURL: databaseURL,
		AppKey:      redditAppKey,
		AppID:       redditAppID,
		Username:    redditUsername,
		Password:    redditPassword,
	}, nil
}
