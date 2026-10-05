package objectstorage

import (
	"context"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func (b *B2Storage) Upload(ctx context.Context, key string, body io.Reader, size int64, contentType string) (string, error) {
	_, err := b.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        &b.bucket,
		Key:           &key,
		Body:          body,
		ContentLength: &size,
		ContentType:   &contentType,
	})
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrUploadFailed, err)
	}

	// Return the object key, not a URL.
	return key, nil
}

func (b *B2Storage) Delete(ctx context.Context, key string) error {
	_, err := b.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: &b.bucket,
		Key:    &key,
	})
	if err != nil {
		return fmt.Errorf("%w: %v", ErrDeleteFailed, err)
	}

	return nil
}

func (b *B2Storage) Exists(ctx context.Context, key string) (bool, error) {
	_, err := b.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: &b.bucket,
		Key:    &key,
	})
	if err != nil {
		return false, nil
	}

	return true, nil
}

func (b *B2Storage) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	out, err := b.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: &b.bucket,
		Key:    &key,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", "failed to download", err)
	}

	return out.Body, nil
}
