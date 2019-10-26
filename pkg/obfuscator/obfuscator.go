package obfuscator

import (
	"crypto/rand"
	"math/big"
	"strconv"
)

func ObfuscateInt(jobID int) (string, error) {
	min, max := 10000000, 99999999
	n, err := rand.Int(rand.Reader, big.NewInt(int64(max-min)))
	if err != nil {
		return "", err
	}
	key := n.Int64() + int64(min)
	right := strconv.Itoa(jobID ^ int(key))
	left := strconv.Itoa(int(key))

	return left + right, nil
}

func RevealInt(ciphertext string) (int, error) {
	keyStr := ciphertext[:8]
	cipherStr := ciphertext[8:]
	key, err := strconv.Atoi(keyStr)
	if err != nil {
		return 0, err
	}
	cipher, err := strconv.Atoi(cipherStr)
	if err != nil {
		return 0, err
	}
	return cipher ^ key, nil
}
