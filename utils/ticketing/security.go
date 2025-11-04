package main

import "github.com/alexedwards/argon2id"

func hashPassword(password string) (string, error) {
	return argon2id.CreateHash(password, argon2id.DefaultParams)
}

func verifyPassword(encoded, password string) (bool, error) {
	return argon2id.ComparePasswordAndHash(password, encoded)
}
