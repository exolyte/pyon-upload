package main

import (
	"database/sql"
	"fmt"
	"net/http"
	"os"

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
	requiredKeys := []string{"service.URL_prefix", "service.max_upload_size", "service.double_dot_extensions",
		"service.filename_length", "service.generate_name_retries", "server.port", "server.max_memory_use",
		"paths.database_path", "paths.database_filename", "paths.placeholder_dir", "aws.region", "aws.bucket",
		"server.ssl_certificate", "server.ssl_key"}
	for _, key := range requiredKeys {
		if !viper.IsSet(key) {
			panic(fmt.Errorf("required key %s is not set in the conf file", key))
		}
	}
	if !viper.IsSet("aws.id") || !viper.IsSet("aws.key") {
		panic(fmt.Errorf("PYON_AWS_ACCESS_ID or PYON_AWS_ACCESS_KEY environment variables were not set"))
	}
	_, err = os.Stat(viper.GetString("paths.database_path"))
	if err != nil {
		panic(fmt.Errorf("the database directory does not exist or is not accessible"))
	}
	_, err = os.Stat(viper.GetString("paths.placeholder_dir"))
	if err != nil {
		panic(fmt.Errorf("the file placeholder directory does not exist or is not accessible"))
	}
	viper.Set("paths.database", viper.GetString("paths.database_dir")+viper.GetString("paths.database_filename"))
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

func createDatabase(path string) error {
	_, err := os.Create(path)
	if err != nil {
		return err
	}
	db, err := sql.Open("sqlite3", viper.GetString("paths.database"))
	if err != nil {
		return err
	}
	_, err = db.Exec(`CREATE TABLE "files" (
		"hash"	TEXT NOT NULL UNIQUE,
		"originalName"	TEXT NOT NULL,
		"fileName"	TEXT NOT NULL UNIQUE,
		"size"	NUMERIC NOT NULL,
		"date"	NUMERIC NOT NULL
		)`)
	if err != nil {
		return err
	}
	_, err = db.Exec(`CREATE INDEX "hashidx" ON "files" ("hash")`)
	if err != nil {
		return err
	}
	_, err = db.Exec(`CREATE INDEX "nameidx" ON "files" ("fileName")`)
	if err != nil {
		return err
	}
	db.Close()
	return nil
}

func main() {
	getConfig()
	_, err := os.Stat(viper.GetString("paths.database"))
	if err != nil {
		err = createDatabase(viper.GetString("paths.database"))
		if err != nil {
			panic(fmt.Errorf("couldn't create db: %w", err))
		}
	}
	db, err := sql.Open("sqlite3", viper.GetString("paths.database"))
	if err != nil {
		panic(fmt.Errorf("couldn't open db: %w", err))
	}
	db.SetMaxOpenConns(1)

	upload.Setup(db, getAWSStorageClient())
	err = http.ListenAndServeTLS(":"+viper.GetString("server.port"),
		viper.GetString("server.ssl_certificate"),
		viper.GetString("server.ssl_key"),
		nil)
	if err != nil {
		panic(fmt.Errorf("couldn't start server"))
	}
}
