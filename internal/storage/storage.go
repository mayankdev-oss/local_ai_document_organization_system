package storage

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const UploadDir = "./uploads"

func init() {
	err := os.MkdirAll(UploadDir, os.ModePerm)
	if err != nil {
		fmt.Printf("Error creating upload directory: %v\n", err)
	}
}

func generateID() string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "fallback_id"
	}
	return hex.EncodeToString(bytes)
}

func SaveFile(r io.Reader, originalFilename string) (string, string, error) {
	ext := filepath.Ext(originalFilename)
	uniqueID := generateID()
	newFilename := fmt.Sprintf("%s%s", uniqueID, ext)
	filepath := filepath.Join(UploadDir, newFilename)

	out, err := os.Create(filepath)
	if err != nil {
		return "", "", err
	}
	defer out.Close()

	_, err = io.Copy(out, r)
	if err != nil {
		return "", "", err
	}

	return newFilename, filepath, nil
}
