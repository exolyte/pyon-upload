package main

import (
	"database/sql"
	"fmt"
	"net/http"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/exolyte/pyon-upload/internal/upload"
	_ "github.com/mattn/go-sqlite3"
	"github.com/spf13/viper"
)

var configLocation string = "/etc/pyon-upload/config"

func getConfig() {
	viper.SetConfigName("conf")
	viper.SetConfigType("toml")
	viper.AddConfigPath(configLocation)

	viper.BindEnv("aws.id", "PYON_AWS_ACCESS_ID")
	viper.BindEnv("aws.key", "PYON_AWS_ACCESS_KEY")

	err := viper.ReadInConfig()
	if err != nil {
		panic(fmt.Errorf("fatal error config file: %w", err))
	}
	if !viper.IsSet("server.port") {
		panic(fmt.Errorf("port was not set"))
	}
	if !viper.IsSet("aws.id") || !viper.IsSet("aws.key") {
		panic(fmt.Errorf("PYON_AWS_ACCESS_ID or PYON_AWS_ACCESS_KEY environment variables were not set"))
	}
}

func getAWSStorageClient() *s3.S3 {
	session := session.Must(session.NewSession(&aws.Config{
		Region: aws.String(viper.GetString("aws.region")),
		Credentials: credentials.NewStaticCredentials(
			viper.GetString("aws.id"),
			viper.GetString("aws.key"),
			"")}))
	return s3.New(session)
}

func main() {
	//TODO configure https
	getConfig()
	db, err := sql.Open("sqlite3", viper.GetString("paths.database"))
	if err != nil {
		panic(fmt.Errorf("couldn't open db: %w", err))
	}
	db.SetMaxOpenConns(1)

	upload.Setup(db, getAWSStorageClient())
	http.ListenAndServe(fmt.Sprintf(":%d", viper.GetInt("server.port")), nil)
}
