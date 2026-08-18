package storage

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const UploadDir = "./uploads"

// allowedExtensions maps validated MIME types to the canonical extension we store.
// This is the ONLY place extensions are decided — never from user input.
var allowedExtensions = map[string]string{
	"application/pdf": ".pdf",
	"image/jpeg":      ".jpg",
	"image/png":       ".png",
}

func init() {
	// 0750: owner read/write/execute, group read/execute, others nothing
	err := os.MkdirAll(UploadDir, 0750)
	if err != nil {
		fmt.Printf("Error creating upload directory: %v\n", err)
	}
}

// generateID returns a cryptographically secure 32-character hex string.
// Returns an error on failure instead of silently returning a predictable fallback.
func generateID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("crypto/rand failure: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// SaveFile writes r to a randomly-named file whose extension is derived from
// the validated mimeType (NOT from the user-supplied filename).
// This prevents path traversal and extension spoofing.
func SaveFile(r io.Reader, mimeType string) (string, string, error) {
	ext, ok := allowedExtensions[mimeType]
	if !ok {
		return "", "", errors.New("unsupported MIME type")
	}

	uniqueID, err := generateID()
	if err != nil {
		return "", "", fmt.Errorf("failed to generate file ID: %w", err)
	}

	newFilename := uniqueID + ext
	destPath := filepath.Join(UploadDir, newFilename)

	// Safety check: ensure the resolved path is inside UploadDir
	absUpload, _ := filepath.Abs(UploadDir)
	absDest, _ := filepath.Abs(destPath)
	if len(absDest) <= len(absUpload) || absDest[:len(absUpload)] != absUpload {
		return "", "", errors.New("path traversal detected")
	}

	out, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0640)
	if err != nil {
		return "", "", fmt.Errorf("failed to create file: %w", err)
	}
	defer out.Close()

	if _, err = io.Copy(out, r); err != nil {
		// Remove the partially-written file on error
		os.Remove(destPath)
		return "", "", fmt.Errorf("failed to write file: %w", err)
	}

	return newFilename, destPath, nil
}
