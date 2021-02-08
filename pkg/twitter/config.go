package main

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	JobsToPost        int
	DatabaseURL       string
	AccessToken       string
	AccessTokenSecret string
	ClientKey         string
	ClientSecret      string
}

func LoadConfig() (Config, error) {
	accessToken := os.Getenv("TWITTER_ACCESS_TOKEN")
	if accessToken == "" {
		return Config{}, fmt.Errorf("TWITTER_ACCESS_TOKEN cannot be empty")
	}
	accessTokenSecret := os.Getenv("TWITTER_ACCESS_TOKEN_SECRET")
	if accessTokenSecret == "" {
		return Config{}, fmt.Errorf("TWITTER_ACCESS_TOKEN_SECRET cannot be empty")
	}
	clientKey := os.Getenv("TWITTER_CLIENT_KEY")
	if clientKey == "" {
		return Config{}, fmt.Errorf("TWITTER_CLIENT_KEY cannot be empty")
	}
	clientSecret := os.Getenv("TWITTER_CLIENT_SECRET")
	if clientSecret == "" {
		return Config{}, fmt.Errorf("TWITTER_CLIENT_SECRET cannot be empty")
	}
	databaseURL := os.Getenv("HEROKU_POSTGRESQL_PINK_URL")
	if databaseURL == "" {
		return Config{}, fmt.Errorf("HEROKU_POSTGRESQL_PINK_URL cannot be empty")
	}
	jobsToPostStr := os.Getenv("TWITTER_JOBS_TO_POST")
	if jobsToPostStr == "" {
		return Config{}, fmt.Errorf("TWITTER_JOBS_TO_POST cannot be empty")
	}
	jobsToPost, err := strconv.Atoi(jobsToPostStr)
	if err != nil {
		return Config{}, fmt.Errorf("could not convert ascii to string %v", err)
	}

	return Config{
		JobsToPost:        jobsToPost,
		DatabaseURL:       databaseURL,
		AccessToken:       accessToken,
		AccessTokenSecret: accessTokenSecret,
		ClientKey:         clientKey,
		ClientSecret:      clientSecret,
	}, nil
}
