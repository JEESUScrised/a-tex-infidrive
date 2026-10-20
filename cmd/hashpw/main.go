// Command hashpw печатает bcrypt-хеш пароля для ADMIN_PASSWORD_HASH.
package main

import (
	"fmt"
	"os"

	"ytdrive/internal/auth"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: hashpw <password>")
		os.Exit(2)
	}
	hash, err := auth.HashPassword(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(hash)
}
