package utils

import "errors"

var (
	ErrInvalidJSON     = errors.New("invalid JSON body")
	ErrReservedField   = errors.New("fields starting with '_' are reserved")
	ErrInvalidName     = errors.New("name must be alphanumeric with underscores")
	ErrDocumentNotFound = errors.New("document not found")
)
