package objectstorage

import "errors"

var (
	ErrObjectNotFound = errors.New("object not found")
	ErrUploadFailed   = errors.New("upload failed")
	ErrDeleteFailed   = errors.New("delete failed")
	ErrInvalidKey     = errors.New("invalid object key")
)
