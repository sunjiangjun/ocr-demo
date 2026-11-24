package cmd

import (
	"crypto/subtle"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var password string

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Authenticate the operator",
	RunE: func(cmd *cobra.Command, args []string) error {
		expected := os.Getenv("OCR_DEMO_PASSWORD")
		if !passwordMatches(password, expected) {
			return fmt.Errorf("authentication failed")
		}
		fmt.Println("authenticated")
		return nil
	},
}

func passwordMatches(got, want string) bool {
	if want == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) != 1
}

func init() {
	loginCmd.Flags().StringVar(&password, "password", "", "operator password")
	rootCmd.AddCommand(loginCmd)
}
