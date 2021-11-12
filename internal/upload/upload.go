package upload

import (
	"bytes"
	"context"
	"crypto/sha1"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/ioutil"
	"math/rand"
	"mime/multipart"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/spf13/viper"
)

var database *sql.DB
var awsStorageClient *s3.S3

type fileResult struct {
	FileHash string `json:"hash"`
	FileName string `json:"name"`
	Url      string `json:"url"`
	Size     uint   `json:"size"`
}

type successResponse struct {
	Success bool         `json:"success"`
	Files   []fileResult `json:"files"`
}

type failureResponse struct {
	Success     bool   `json:"success"`
	Errorcode   int    `json:"errorcode"`
	Description string `json:"description"`
}

func constructFailureResponse(code int, description string) ([]byte, error) {
	message := &failureResponse{
		Success:     false,
		Errorcode:   code,
		Description: description,
	}
	jsonValue, err := json.Marshal(message)
	if err != nil {
		return nil, errors.New("could not convert message to json")
	}
	return jsonValue, nil
}

func Setup(db *sql.DB, storageClient *s3.S3) {
	database = db
	awsStorageClient = storageClient
	http.HandleFunc("/upload", upload)
}

func getHash(file []byte) string {
	bytes := sha1.Sum(file)
	fileHash := hex.EncodeToString(bytes[:])
	return fileHash
}

func checkExists(fileHash string) (bool, string, error) {
	var fileName string
	err := database.QueryRow("SELECT filename FROM files WHERE hash=?", fileHash).Scan(&fileName)
	if err == sql.ErrNoRows {
		return false, "", nil
	}
	if err != nil {
		return false, "", errors.New("could not query db")
	}
	return true, fileName, nil
}

func getFileExtension(fileName string) string {
	var extension string
	suffixes := viper.GetStringSlice("service.double_dot_extensions")
	for _, suffix := range suffixes {
		if strings.HasSuffix(fileName, suffix) {
			return "." + suffix
		}
	}

	splitName := strings.Split(fileName, ".")
	if len(splitName) == 1 {
		return ""
	}
	extension = splitName[len(splitName)-1]
	return "." + extension
}

func generateName(originalName string) (string, error) {
	extension := getFileExtension(originalName)

	rand.Seed(time.Now().UnixNano())
	var letters = []rune("abcdefghijklmnopqrstuvwxyz")
	var count int
	randomString := make([]rune, viper.GetInt("service.filename_length"))
	for i := 0; i < viper.GetInt("service.generate_name_retries"); i++ {
		for i := range randomString {
			randomString[i] = letters[rand.Intn(len(letters))]
		}
		newName := string(randomString) + extension
		err := database.QueryRow("SELECT COUNT(filename) FROM files WHERE filename=?", newName).Scan(&count)
		if err != nil {
			return "", errors.New("could not query db")
		}
		if count == 0 {
			return newName, nil
		}
	}
	return "", errors.New("could not generate unique name")
}

func storeToS3(file []byte, fileName string, fileSize int64) error {
	ctx := context.Background()
	ctx, cancelFn := context.WithTimeout(ctx, 30*time.Second)
	defer cancelFn()

	mimeType := http.DetectContentType(file)
	_, err := awsStorageClient.PutObjectWithContext(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(viper.GetString("aws.bucket")),
		Key:         aws.String(fileName),
		Body:        bytes.NewReader(file),
		ContentType: &mimeType,
	})
	if err != nil {
		return errors.New("failed to upload to s3")
	}
	return nil
}

func updateDB(fileHash string, originalName string, newName string, fileSize int) error {
	statement, err := database.Prepare("INSERT INTO files(hash, originalname, filename, size, date) VALUES (?,?,?,?,?)")
	if err != nil {
		return errors.New("failed to prepare sqlite insert")
	}
	_, err = statement.Exec(fileHash, originalName, newName, fileSize, time.Now().Unix())
	if err != nil {
		return errors.New("failed to insert into sqlite")
	}
	return nil
}

func handleFile(fileHeader *multipart.FileHeader) (*fileResult, error) {
	infile, err := fileHeader.Open()
	if err != nil {
		return nil, errors.New("failed to open file")
	}
	bytes, err := ioutil.ReadAll(infile)
	if err != nil {
		return nil, errors.New("failed to read file")
	}
	fileHash := getHash(bytes)
	exists, fileName, err := checkExists(fileHash)
	if err != nil {
		return nil, err
	}
	if exists {
		res := fileResult{
			FileHash: fileHash,
			FileName: fileHeader.Filename,
			Url:      viper.GetString("service.URL_prefix") + fileName,
			Size:     uint(fileHeader.Size),
		}
		return &res, nil
	}
	newName, err := generateName(fileHeader.Filename)
	if err != nil {
		return nil, err
	}
	err = storeToS3(bytes, newName, fileHeader.Size)
	if err != nil {
		return nil, err
	}
	err = updateDB(fileHash, fileHeader.Filename, newName, int(fileHeader.Size))
	if err != nil {
		return nil, err
	}

	//Create a placeholder file so nginx can check whether a file exists without using S3
	outfile, err := os.OpenFile(viper.GetString("paths.placeholder_dir")+newName, os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return nil, errors.New("could not create placeholder file")
	}
	outfile.Close()

	res := fileResult{
		FileHash: fileHash,
		FileName: fileHeader.Filename,
		Url:      viper.GetString("service.URL_prefix") + newName,
		Size:     uint(fileHeader.Size),
	}
	return &res, nil
}

func upload(w http.ResponseWriter, req *http.Request) {
	//TODO configure headers (content type, accept encoding, compression,...)
	if req.Method == "POST" {
		req.Body = http.MaxBytesReader(w, req.Body, viper.GetInt64("service.max_upload_size"))
		err := req.ParseMultipartForm(viper.GetInt64("server.max_memory_use"))
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			jsonValue, err := constructFailureResponse(http.StatusBadRequest, "Failed to accept file")
			if err != nil {
				return
			}
			w.Write(jsonValue)
			return
		}
		fileHeaders := req.MultipartForm.File["files[]"]
		response := successResponse{
			Success: true,
			Files:   make([]fileResult, 0),
		}
		for _, fileHeader := range fileHeaders {
			res, err := handleFile(fileHeader)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				jsonValue, err := constructFailureResponse(http.StatusInternalServerError, "Failed to handle file")
				if err != nil {
					return
				}
				w.Write(jsonValue)
				return
			}
			response.Files = append(response.Files, *res)
		}
		w.WriteHeader(http.StatusOK)
		jsonValue, err := json.Marshal(response)
		if err != nil {
			return
		}
		w.Write(jsonValue)
		return
	}
}
