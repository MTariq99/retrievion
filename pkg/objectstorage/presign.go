package objectstorage

import (
	"context"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func (b *B2Storage) PresignGetURL(ctx context.Context, key string, expiry time.Duration) (string, error) {
	presignClient := s3.NewPresignClient(b.client)
	out, err := presignClient.PresignGetObject(
		ctx,
		&s3.GetObjectInput{
			Bucket: &b.bucket,
			Key:    &key,
		},
		func(opts *s3.PresignOptions) {
			opts.Expires = expiry
		},
	)
	if err != nil {
		return "", err
	}

	return out.URL, nil
}
