package utils

import (
	"crypto/md5"
	"crypto/sha1"
	"hash"
	"io"
	"os"
)

// calculate file's MD5
func CalcMd5(path string) ([]byte, error) {
	file, err := os.Open(path)

	if err != nil {
		return nil, err
	}
	return CalcHashByReader(file, md5.New())
}

// calculate file's SHA-1
func CalcSha1(path string) ([]byte, error) {
	file, err := os.Open(path)

	if err != nil {
		return nil, err
	}

	return CalcHashByReader(file, sha1.New())
}

// calculate file's HASH value with specified HASH Algorithm
func CalcHashByReader(reader io.Reader, hash hash.Hash) ([]byte, error) {

	_, _err := io.Copy(hash, reader)
	if _err != nil {
		return nil, _err
	}

	sum := hash.Sum(nil)

	return sum, nil
}
