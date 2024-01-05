// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build serverlessdebug

package tmpdebug

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go/aws"
)

type putS3Client struct {
	client *s3.Client
}

func BlockingS3Upload(dirName string, bucketName string, client BucketClientInterface) {
	startTime := time.Now()
	s3Client, err := createS3Client(client)
	if err != nil {
		log.Error("Could not create the s3Client")
		return
	}
	items, _ := ioutil.ReadDir(dirName)
	for _, item := range items {
		if !item.IsDir() {
			fullName := fmt.Sprintf("%s/%s", dirName, item.Name())
			uploadToS3(s3Client, fullName, bucketName)
		}
	}
	elapsedTime := time.Since(startTime)
	log.Info("Upload completed in %s", elapsedTime)
}

func createS3Client(client BucketClientInterface) (BucketClientInterface, error) {
	if client != nil {
		return client, nil
	}
	cfg, err := awsconfig.LoadDefaultConfig(
		context.TODO(),
	)
	if err != nil {
		return nil, err
	}
	putS3Client := &putS3Client{
		client: s3.NewFromConfig(cfg),
	}
	return putS3Client, nil
}

func uploadToS3(client BucketClientInterface, fileName string, bucketName string) {
	fmt.Printf("fileName=%s", fileName)
	currentTime := time.Now()
	key := currentTime.Format("2006-01-02-15-04-05-000000")
	log.Infof("Uploading file: %s to bucketName: %s at key: %s", fileName, bucketName, key)
	file, err := os.Open(fileName)
	if err != nil {
		log.Errorf("Couldn't open file to upload")
	} else {
		defer file.Close()
		client.putObject(bucketName, key, file)
		if err != nil {
			log.Errorf("Couldn't upload file: %v\n", err)
		}
		log.Info("File uploaded")
	}
}

func (p *putS3Client) putObject(bucketName string, key string, file *os.File) error {
	_, err := p.client.PutObject(context.TODO(), &s3.PutObjectInput{
		Bucket: aws.String(bucketName),
		Key:    aws.String(key),
		Body:   file,
	})
	return err
}
