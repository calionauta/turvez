package secret

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
//  1. If DOTENV_FILE is set, load it; else if CLIENT_ID is set, load
//     env/<CLIENT_ID>.env when it exists (per-client file; a missing
//     file warns on stderr and continues — secrets may come from the
//     host environment or SECRETS_AGE_FILE instead).
//  2. If SECRETS_AGE_FILE is set, decrypt with AGE_IDENTITY_FILE (or
//     ~/.secrets/consultancy.age.key fallback) and merge.
//
// Env vars already set always win over file contents.
func InitFromEnv() error {
	if dotenv := os.Getenv("DOTENV_FILE"); dotenv != "" {
		if err := LoadDotEnvFile(dotenv); err != nil {
			return err
		}
	} else if client := os.Getenv("CLIENT_ID"); client != "" {
		if strings.ContainsAny(client, `/.\`) || client != filepath.Base(client) {
			return fmt.Errorf("secret: invalid CLIENT_ID %q", client)
		}
		p := filepath.Join("env", client+".env")
		if _, err := os.Stat(p); err == nil {
			if err := LoadDotEnvFile(p); err != nil {
				return err
			}
		} else {
			fmt.Fprintf(os.Stderr, "secret: CLIENT_ID=%s but %s not found; continuing with host env\n", client, p)
		}
	}
	enc := os.Getenv("SECRETS_AGE_FILE")
	if enc == "" {
		return nil
	}
	identity := os.Getenv("AGE_IDENTITY_FILE")
	if identity == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("secret: AGE_IDENTITY_FILE unset and home dir unknown: %w", err)
		}
		identity = home + "/.secrets/consultancy.age.key"
	}
	return LoadAgeEnv(enc, identity)
}
