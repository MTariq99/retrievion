package objectstorage

import (
	"context"
	"fmt"
	"strings"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/mtariq99/retrievion/models"
)

type B2Storage struct {
	client   *s3.Client
	bucket   string
	endpoint string
}

func NewB2Storage(cfg *models.Config) (Storage, error) {
	awsCfg, err := awsconfig.LoadDefaultConfig(
		context.Background(),
		awsconfig.WithRegion(cfg.Storage.B2Region),
		awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(
				cfg.Storage.B2AccessKey,
				cfg.Storage.B2SecretKey,
				"",
			),
		),
	)
	if err != nil {
		return nil, err
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.UsePathStyle = true
		o.EndpointResolver = s3.EndpointResolverFromURL(cfg.Storage.B2Endpoint)
	})

	return &B2Storage{
		client:   client,
		bucket:   cfg.Storage.B2Bucket,
		endpoint: strings.TrimRight(cfg.Storage.B2Endpoint, "/"),
	}, nil
}

func (b *B2Storage) GetURL(key string) string {
	return fmt.Sprintf("%s/%s/%s", b.endpoint, b.bucket, strings.TrimLeft(key, "/"))
}
