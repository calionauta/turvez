package secret

import (
	"fmt"
	"os"
	"os/exec"
)

// DecryptAgeFile shells out to the `age` binary (https://age-encryption.org)
// to decrypt an age-encrypted dotenv file. Keeping this an external dep
// (instead of a Go library) means ciphertext stays portable across tools.
func DecryptAgeFile(encryptedPath, identityFile string) ([]byte, error) {
	ageBin, err := exec.LookPath("age")
	if err != nil {
		return nil, fmt.Errorf("secret: `age` binary not found in PATH: %w", err)
	}
	cmd := exec.Command(ageBin, "--decrypt", "--identity", identityFile, encryptedPath)
	cmd.Stderr = nil
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("secret: age --decrypt %s: %w", encryptedPath, err)
	}
	return out, nil
}

// LoadAgeEnv decrypts encryptedPath with identityFile, parses the plaintext
// as dotenv, and merges it into the environment without overriding
// existing vars. Plaintext lives only in memory — never written to disk.
func LoadAgeEnv(encryptedPath, identityFile string) error {
	raw, err := DecryptAgeFile(encryptedPath, identityFile)
	if err != nil {
		return err
	}
	return mergeDotEnvBytes(raw)
}

// InitFromEnv implements the startup chain used by cmd/worker:
//
//  1. If DOTENV_FILE is set, load it (local dev convenience).
//  2. If SECRETS_AGE_FILE is set, decrypt with AGE_IDENTITY_FILE (or
//     ~/.secrets/consultancy.age.key fallback) and merge.
//
// Env vars already set always win over file contents.
func InitFromEnv() error {
	if dotenv := os.Getenv("DOTENV_FILE"); dotenv != "" {
		if err := LoadDotEnvFile(dotenv); err != nil {
			return err
		}
	}
	enc := os.Getenv("SECRETS_AGE_FILE")
	if enc == "" {
		return nil
	}
	identity := os.Getenv("AGE_IDENTITY_FILE")
	if identity == "" {
		home, _ := os.UserHomeDir()
		identity = home + "/.secrets/consultancy.age.key"
	}
	return LoadAgeEnv(enc, identity)
}
