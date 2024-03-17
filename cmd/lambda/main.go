package main

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/paralleltree/markov-bot-go/config"
	"github.com/paralleltree/markov-bot-go/handler"
	"github.com/paralleltree/markov-bot-go/morpheme"
	"github.com/paralleltree/markov-bot-go/persistence"
)

func main() {
	rand.Seed(time.Now().UnixNano())
	lambda.Start(requestHandler)
}

type PostEvent struct {
	S3Region     string `json:"s3Region"`
	S3BucketName string `json:"s3BucketName"`
	S3KeyPrefix  string `json:"s3KeyPrefix"`
}

func requestHandler(e PostEvent) error {
	confStore, err := persistence.NewS3Store(e.S3Region, e.S3BucketName, fmt.Sprintf("%s/config.yml", e.S3KeyPrefix))
	if err != nil {
		return err
	}
	conf, err := loadConfig(confStore)
	if err != nil {
		return err
	}

	s3Store, err := persistence.NewS3Store(e.S3Region, e.S3BucketName, fmt.Sprintf("%s/model", e.S3KeyPrefix))
	if err != nil {
		return err
	}
	modelStore := persistence.NewCompressedStore(s3Store)

	analyzer := morpheme.NewMecabAnalyzer("mecab-ipadic-neologd")

	mod, ok, err := modelStore.ModTime()
	if err != nil {
		return fmt.Errorf("get modtime: %w", err)
	}

	if !ok || float64(conf.ExpiresIn) < time.Since(mod).Seconds() {
		if err := handler.BuildChain(conf.FetchClient, analyzer, conf.FetchStatusCount, conf.StateSize, modelStore); err != nil {
			return fmt.Errorf("build chain: %w", err)
		}
	}

	if err := handler.GenerateAndPost(conf.PostClient, modelStore, conf.MinWordsCount, false); err != nil {
		return fmt.Errorf("generate and post: %w", err)
	}

	return nil
}

func loadConfig(store persistence.PersistentStore) (*config.BotConfig, error) {
	data, err := store.Load()
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	conf, err := config.LoadBotConfig(data)
	if err != nil {
		return nil, fmt.Errorf("load bot config: %w", err)
	}

	return conf, nil
}
